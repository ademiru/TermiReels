package tui

import (
	"strings"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func shareTestFriends(n int) []backend.User {
	friends := make([]backend.User, n)
	for i := range friends {
		friends[i] = backend.User{
			Name:    string(rune('a'+i)) + "_friend",
			ImgSrc:  "http://example/x.jpg",
			ImgPath: "/nonexistent/x.jpg",
		}
	}
	return friends
}

// The row a friend is drawn on and the row a click resolves to are computed
// separately; they have to agree.
func TestShareFriendAtRowMatchesRenderedRows(t *testing.T) {
	sp := NewSharePanel()
	sp.Open()
	sp.SetFriends(shareTestFriends(6))

	const height = 14
	view := sp.View(40, height, "")
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")

	// Line 0 is the header; friends start at line 1, one every
	// sharePfpCellHeight rows with the name on the middle row.
	for i := range sp.visibleCount {
		nameRow := 1 + i*sharePfpCellHeight + sharePfpCellHeight/2
		if nameRow >= len(lines) {
			break
		}
		if !strings.Contains(lines[nameRow], sp.friends[i].Name) {
			t.Fatalf("friend %d not on rendered line %d: %q", i, nameRow, lines[nameRow])
		}
		// FriendAtRow takes an offset from the first friend row, which is
		// rendered line 1.
		for row := range sharePfpCellHeight {
			offset := i*sharePfpCellHeight + row
			if got := sp.FriendAtRow(offset); got != i {
				t.Errorf("offset %d (friend %d, row %d of its block): got %d", offset, i, row, got)
			}
		}
	}

	if got := sp.FriendAtRow(-1); got != -1 {
		t.Errorf("above the list: got %d, want -1", got)
	}
	if got := sp.FriendAtRow(sp.visibleCount * sharePfpCellHeight); got != -1 {
		t.Errorf("past the visible list: got %d, want -1", got)
	}
}

// Clicking a friend must pick them, the same as moving the cursor there and
// pressing the select key.
func TestClickSelectsShareFriend(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubShareBackend{}
	m.width, m.height = 100, 40
	m.currentReel = &backend.ReelInfo{}
	player.VideoHeightChars = 10
	player.VideoWidthChars = 30

	m.share.Open()
	m.share.SetFriends(shareTestFriends(5))
	m.share.View(40, m.panelLines(), "") // establishes visibleCount

	// The second friend's block starts one block below the first.
	y := m.panelBaseRow() + sharePfpCellHeight

	press := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      m.videoCol + 10,
		Y:      y,
	}
	updated, _ := m.updateMouse(press)
	sp := updated.(Model).share

	if sp.CursorIndex() != 1 {
		t.Errorf("cursor: got %d, want 1", sp.CursorIndex())
	}
	if sp.SelectedCount() != 1 {
		t.Errorf("selected: got %d, want 1", sp.SelectedCount())
	}

	// Clicking the same friend again clears the selection.
	updated, _ = updated.(Model).updateMouse(press)
	if n := updated.(Model).share.SelectedCount(); n != 0 {
		t.Errorf("after second click: %d selected, want 0", n)
	}
}

// A selection has to be visible without relying on colour alone.
func TestShareRowShowsAnExplicitCheckbox(t *testing.T) {
	off := shareRowLabel("someone", false, false)
	on := shareRowLabel("someone", true, false)

	if !strings.Contains(off, "○") {
		t.Errorf("unselected row has no empty checkbox: %q", off)
	}
	if !strings.Contains(on, "●") {
		t.Errorf("selected row has no filled checkbox: %q", on)
	}
	if cursor := shareRowLabel("someone", false, true); !strings.Contains(cursor, "▌") {
		t.Errorf("cursor row has no bar: %q", cursor)
	}
}

type stubShareBackend struct {
	backend.Backend
}

func (s *stubShareBackend) IsSyncing() bool         { return false }
func (s *stubShareBackend) ToggleShareFriend(i int) {}

// The send button's recorded column span must match where it is actually
// drawn, or clicking it does nothing.
func TestSendButtonSpanMatchesRenderedHeader(t *testing.T) {
	sp := NewSharePanel()
	sp.Open()
	sp.SetFriends(shareTestFriends(3))
	sp.MoveCursor(1)
	sp.ToggleSelected()

	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii) // plain text so columns are literal
	defer lipgloss.SetColorProfile(restore)

	header := sp.header()
	idx := strings.Index(header, "  SEND  ")
	if idx < 0 {
		t.Fatalf("header has no send button: %q", header)
	}
	if idx != sp.sendStart {
		t.Errorf("send button drawn at column %d, recorded at %d\n  %q", idx, sp.sendStart, header)
	}

	for off := sp.sendStart; off < sp.sendStart+sp.sendWidth; off++ {
		if !sp.SendButtonAt(off) {
			t.Errorf("column %d is inside the button but not recognised", off)
		}
	}
	if sp.SendButtonAt(sp.sendStart - 1) {
		t.Error("column left of the button was recognised as the button")
	}
	if sp.SendButtonAt(sp.sendStart + sp.sendWidth) {
		t.Error("column right of the button was recognised as the button")
	}
}

// Clicking the send button must fire the send, the same as pressing S.
func TestClickSendButtonSends(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubShareBackend{}
	m.width, m.height = 100, 40
	m.currentReel = &backend.ReelInfo{}

	m.share.Open()
	m.share.SetFriends(shareTestFriends(3))
	m.share.View(40, m.panelLines(), "") // records the button span

	panelX := max(m.videoCol-1, 0)
	press := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      panelX + m.share.sendStart,
		Y:      m.panelBaseRow() - 1,
	}
	updated, cmd := m.updateMouse(press)
	if cmd == nil {
		t.Fatal("clicking send returned no command")
	}
	if !updated.(Model).shareSending {
		t.Error("clicking send did not mark the share as in flight")
	}

	// A click elsewhere on the header does nothing.
	m2 := testModel()
	m2.state = stateBrowsing
	m2.backend = &stubShareBackend{}
	m2.width, m2.height = 100, 40
	m2.currentReel = &backend.ReelInfo{}
	m2.share.Open()
	m2.share.SetFriends(shareTestFriends(3))
	m2.share.View(40, m2.panelLines(), "")

	_, cmd = m2.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: panelX, Y: m2.panelBaseRow() - 1,
	})
	if cmd != nil {
		t.Error("clicking the title fired a send")
	}
}

// Sending has to say so. The share icon ticking for a second is easy to miss,
// which is the whole reason a share felt like it might not have gone.
func TestSendConfirmationNamesTheCount(t *testing.T) {
	cases := []struct {
		friends int
		want    string
	}{
		{0, "SENT"},
		{1, "SENT TO 1 FRIEND"},
		{2, "SENT TO 2 FRIENDS"},
	}
	for _, tc := range cases {
		if got := sentToast(tc.friends); got != tc.want {
			t.Errorf("sentToast(%d) = %q, want %q", tc.friends, got, tc.want)
		}
	}
}

// The count is read off the panel before it closes, or the confirmation would
// always say zero.
func TestShareCountIsCapturedBeforeThePanelCloses(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubShareBackend{}
	m.width, m.height = 100, 40
	m.currentReel = &backend.ReelInfo{}

	m.share.Open()
	m.share.SetFriends(shareTestFriends(3))
	m.share.View(40, m.panelLines(), "")
	m.share.SetCursor(0)
	m.share.ToggleSelected()
	m.share.SetCursor(2)
	m.share.ToggleSelected()

	m.closeShare()
	if m.shareCount != 2 {
		t.Errorf("captured %d friends, want 2", m.shareCount)
	}

	// Closing the panel afterwards must not change what the toast will say.
	m.share.Close()
	if m.shareCount != 2 {
		t.Errorf("count changed to %d after the panel closed", m.shareCount)
	}
}
