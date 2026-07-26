package tui

import (
	"strings"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/charmbracelet/lipgloss"
)

// Fixed mode must shrink the reel while a panel is open so the panel has room,
// and must not touch the stored reel_width doing it.
func TestReelPixelSizeShrinksForPanelInFixedMode(t *testing.T) {
	m := testModel()
	backend.Config = backend.Settings{
		ReelFit:          false,
		RetinaScale:      1,
		ReelWidth:        270,
		ReelHeight:       480,
		ReelSizeStep:     30,
		PanelShrinkSteps: 4,
	}

	w, h := m.reelPixelSize()
	if w != 270 || h != 480 {
		t.Errorf("closed: got %dx%d, want 270x480", w, h)
	}

	m.help.Open()
	sw, sh := m.reelPixelSize()
	if sw >= w {
		t.Errorf("panel open: width %d did not shrink below %d", sw, w)
	}
	if want := 270 - 30*4; sw != want {
		t.Errorf("panel open: got width %d, want %d", sw, want)
	}
	if want := sw * reelAspectH / reelAspectW; sh != want {
		t.Errorf("panel open: got height %d, want %d (9:16 of %d)", sh, want, sw)
	}

	if backend.GetSettings().ReelWidth != 270 {
		t.Errorf("opening a panel changed the stored reel_width to %d", backend.GetSettings().ReelWidth)
	}
}

// The narrow fallback must reserve its declared panel area even if the richer
// metadata layout happens to reserve one more row while closed.
func TestChromeRowsReservesBottomPanel(t *testing.T) {
	m := testModel()
	m.width = sidePanelMinTerminalWidth - 1 // narrow layout: panel opens below the reel
	m.comments.Open("pk")
	if got, want := m.chromeRows(), chromeRowsAbove+chromeRowsBelowPanel; got != want {
		t.Errorf("panel chrome=%d, want %d", got, want)
	}
}

func TestCommentsUseResponsiveSidePanel(t *testing.T) {
	m := testModel()
	m.comments.Open("pk")

	m.width = sidePanelMinTerminalWidth - 1
	if m.commentsOnSide() {
		t.Fatal("comments moved beside the reel in a narrow terminal")
	}
	if m.panelBaseCol() != m.videoCol {
		t.Errorf("narrow panel col=%d, want reel col=%d", m.panelBaseCol(), m.videoCol)
	}

	m.width = 120
	if !m.commentsOnSide() {
		t.Fatal("comments did not move beside the reel in a wide terminal")
	}
	if m.panelBaseCol() <= m.videoCol+player.VideoWidthChars {
		t.Errorf("side panel col=%d overlaps reel ending at %d",
			m.panelBaseCol(), m.videoCol+player.VideoWidthChars)
	}
	if m.panelBaseRow() != m.videoRow {
		t.Errorf("side panel row=%d, want video row=%d", m.panelBaseRow(), m.videoRow)
	}
}

func TestStatusLineLivesAtBottom(t *testing.T) {
	m := testModel()
	m.width, m.height = 100, 40
	if got := m.statusLineRow(); got != 37 {
		t.Errorf("status row=%d, want 37", got)
	}
}

func TestBrowsingFrameActivelyClearsEveryRowAndDrawsOneFooter(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubBackend{}
	m.width, m.height = 90, 36
	m.currentReel = &backend.ReelInfo{}

	view := m.viewBrowsing()
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("frame has %d rows, want exactly %d", len(lines), m.height)
	}
	for i, line := range lines {
		if line == "" {
			t.Errorf("row %d is empty and may leave stale terminal content", i)
		}
	}
	if got := strings.Count(view, "🤍"); got != 1 {
		t.Errorf("frame contains %d footer hearts, want exactly one", got)
	}
}

func TestVideoIsTopAnchored(t *testing.T) {
	m := testModel()
	m.width, m.height = 100, 50
	m.updateVideoPosition()
	if want := chromeRowsAbove + 1; m.videoRow != want {
		t.Errorf("video row=%d, want top anchor %d", m.videoRow, want)
	}
}

func TestCompactViewportPrioritizesReel(t *testing.T) {
	m := testModel()
	m.height = compactViewportRows - 1
	if !m.compactViewport() {
		t.Fatal("short terminal did not enter compact viewport")
	}
	if got := m.chromeRows(); got != compactChromeRows {
		t.Errorf("compact chrome=%d, want %d", got, compactChromeRows)
	}

	m.height = compactViewportRows
	if m.compactViewport() {
		t.Fatal("desktop-height terminal stayed in compact viewport")
	}
	if got := m.chromeRows(); got <= compactChromeRows {
		t.Errorf("full layout reserved only %d rows; want more than compact %d", got, compactChromeRows)
	}
}

func TestCompactViewportHidesSecondaryFooterLabels(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubBackend{}
	m.width, m.height = 80, 24
	m.currentReel = &backend.ReelInfo{}
	view := m.viewBrowsing()
	if strings.Contains(view, "COMMENTS") || strings.Contains(view, "REPOST") {
		t.Errorf("compact viewport still rendered secondary label row:\n%s", view)
	}
	if !strings.Contains(view, "VOL") {
		t.Error("compact viewport lost direct volume control")
	}
}

func TestCommentsRenderToRightOfReel(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubBackend{}
	m.width, m.height = 120, 40
	m.videoRow, m.videoCol = 5, 10
	player.VideoWidthChars, player.VideoHeightChars = 30, 20
	backend.Config = backend.DefaultSettings()
	m.currentReel = &backend.ReelInfo{}
	m.currentReel.Username = "creator"
	m.comments.Open("reel")
	m.comments.SetComments("reel", []backend.Comment{
		{PK: "comment", Username: "commenter", Text: "hello"},
	})

	lines := strings.Split(m.viewBrowsing(), "\n")
	headerRow := m.panelContentBaseRow() - 1
	if headerRow >= len(lines) || !strings.Contains(lines[headerRow], "Comments") {
		t.Fatalf("side-panel header missing from row %d", headerRow)
	}
	headerCol := lipgloss.Width(lines[headerRow][:strings.Index(lines[headerRow], "Comments")])
	if want := m.panelContentBaseCol() - 1; headerCol != want {
		t.Errorf("comments header starts at col %d, want %d", headerCol, want)
	}
}

// The browsing view must never be taller than the terminal.
//
// One line too many makes the terminal scroll, which shifts every row up and
// leaves the mouse hit-test pointing a row below what's drawn — the reason
// clicking a status icon activated the row beneath it.
func TestViewNeverOverflowsTerminal(t *testing.T) {
	for _, rows := range []int{20, 24, 30, 40, 50, 60, 80} {
		for _, navbar := range []bool{true, false} {
			for _, panel := range []bool{true, false} {
				m := testModel()
				m.state = stateBrowsing
				m.backend = &stubBackend{}
				m.width, m.height = 100, rows
				m.showNavbar = navbar
				m.currentReel = &backend.ReelInfo{}
				m.currentReel.Username = "someone"
				m.currentReel.Caption = strings.Repeat("a long caption that wraps over many lines ", 8)
				if panel {
					m.help.Open()
				}

				// Size and centre the reel the way fit mode does.
				h := max(rows-m.chromeRows(), 1)
				player.VideoHeightChars = h
				player.VideoWidthChars = 30
				m.videoRow = (rows-h)/2 + 1
				if panel {
					m.videoRow = 5
				}
				m.videoCol = 20

				n := len(strings.Split(m.viewBrowsing(), "\n"))
				if n > rows {
					t.Errorf("rows=%d navbar=%v panel=%v: view has %d lines", rows, navbar, panel, n)
				}
			}
		}
	}
}

func TestFormatTimecode(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0:00"},
		{5, "0:05"},
		{59.4, "0:59"},
		{60, "1:00"},
		{125, "2:05"},
		{-3, "0:00"},
	}
	for _, tc := range cases {
		if got := formatTimecode(tc.in); got != tc.want {
			t.Errorf("formatTimecode(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
