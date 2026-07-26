package tui

import (
	"strings"
	"sync"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// testModel builds a Model that can be rendered without a live backend.
//
// The reel's cell dimensions live in package-level vars in player, so tests
// pin them here rather than inheriting whatever the previous test left behind.
func testModel() Model {
	backend.Config = backend.Settings{}
	player.VideoWidthChars = 30
	player.VideoHeightChars = 20
	return Model{
		backend:  &stubBackend{},
		player:   player.NewAVPlayer(),
		comments: NewCommentsPanel(),
		share:    NewSharePanel(),
		help:     NewHelpPanel(),
		chats:    NewChatsPanel(),
		react:    NewReactPanel(),
		videoRow: 10,
		videoCol: 20,
	}
}

// Clicking each icon must resolve to that icon's action. The offsets are
// derived from the same segment list the renderer uses, so this catches a
// segment being added without the hit-test seeing it.
func TestStatusActionAtHitsEverySegment(t *testing.T) {
	m := testModel()

	col := 0
	for _, seg := range m.statusSegments() {
		w := lipgloss.Width(seg.text)
		if seg.action != statusActionNone && w > 0 {
			if got := m.statusActionAt(col); got != seg.action {
				t.Errorf("offset %d (segment %q): got action %d, want %d", col, seg.text, got, seg.action)
			}
		}
		col += w
	}

	if got := m.statusActionAt(col + 5); got != statusActionNone {
		t.Errorf("past end of line: got action %d, want none", got)
	}
	if got := m.statusActionAt(-1); got != statusActionNone {
		t.Errorf("negative offset: got action %d, want none", got)
	}
}

// The hit-test and renderer must agree on the fixed footer coordinates.
func TestStatusRowMatchesRenderedLayout(t *testing.T) {
	m := testModel()
	m.width, m.height = 120, 40

	rendered := m.viewBrowsing()
	lines := strings.Split(rendered, "\n")

	statusY := m.statusLineRow()
	if statusY < 0 || statusY >= len(lines) {
		t.Fatalf("status row %d outside rendered output of %d lines", statusY, len(lines))
	}

	// The status line is the one carrying the controls. Its first control's
	// pill starts at the centred footer origin.
	line := lines[statusY]
	statusX := m.statusLineStart()
	if !strings.Contains(line, "🤍") {
		t.Fatalf("row %d is not the status line: %q", statusY, line)
	}
	if got := lipgloss.Width(line[:strings.Index(line, "🤍")]) - statusPillPad; got != statusX {
		t.Errorf("status line indent: got %d, want %d\nline: %q", got, statusX, line)
	}

	// The first segment is the like icon; a click at its first column must
	// resolve to a like.
	if got := m.statusActionAt2D(statusX, statusY); got != statusActionLike {
		t.Errorf("click at start of status line: got action %d, want like", got)
	}
	if got := m.statusActionAt2D(statusX, statusY-1); got != statusActionNone {
		t.Errorf("click above status line: got action %d, want none", got)
	}
	if got := m.statusActionAt2D(statusX-1, statusY); got != statusActionNone {
		t.Errorf("click left of status line: got action %d, want none", got)
	}
}

// stubBackend satisfies backend.Backend by embedding it. Only the methods a
// test actually exercises are implemented; anything else panics on a nil
// interface, which is the signal that the test needs to declare more.
type stubBackend struct {
	backend.Backend

	mu    sync.Mutex
	likes int
	vol   float64
}

func (s *stubBackend) IsSyncing() bool { return false }

func (s *stubBackend) ToggleLike() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.likes++
	return true, nil
}

func (s *stubBackend) SetVolume(vol float64) error {
	s.mu.Lock()
	s.vol = vol
	s.mu.Unlock()
	return nil
}

// The whole point of the status row is that clicking it does something. This
// drives a real MouseMsg through the handler rather than calling the toggle.
func TestClickOnHeartLikesTheReel(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubBackend{}
	m.currentReel = &backend.ReelInfo{}

	statusY := m.statusLineRow()
	statusX := m.statusLineStart()

	press := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      statusX,
		Y:      statusY,
	}
	if _, _ = m.updateMouse(press); !m.currentReel.Liked {
		t.Fatal("clicking the heart did not like the reel")
	}
	if _, _ = m.updateMouse(press); m.currentReel.Liked {
		t.Error("clicking the heart again did not unlike the reel")
	}
}

// Clicking the reel itself is the pause affordance.
func TestClickOnReelTogglesPause(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	player.VideoWidthChars = 30
	player.VideoHeightChars = 20

	press := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      m.videoCol - 1 + 5,
		Y:      m.videoRow - 1 + 5,
	}
	updated, _ := m.updateMouse(press)
	if updated.(Model).status != statusPaused {
		t.Error("clicking the reel did not pause it")
	}
}

// Moving the pointer over a control highlights it, which is what tells the
// user the row is clickable in the first place.
func TestHoverTracksStatusControls(t *testing.T) {
	// Tests have no TTY, so lipgloss would otherwise strip every escape and
	// the styling assertion below would pass vacuously.
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := testModel()
	m.state = stateBrowsing
	m.currentReel = &backend.ReelInfo{Reel: backend.Reel{CanViewerReshare: true}}

	statusY := m.statusLineRow()
	statusX := m.statusLineStart()

	move := func(x, y int) Model {
		updated, _ := m.updateMouse(tea.MouseMsg{Action: tea.MouseActionMotion, X: x, Y: y})
		return updated.(Model)
	}

	idle := m.statusSegments()[0].text

	m = move(statusX, statusY)
	if m.hoveredStatus != statusActionLike {
		t.Errorf("hovering the heart: got %d, want like", m.hoveredStatus)
	}

	// Tracking hover is pointless unless it reaches the rendered line.
	if hovered := m.statusSegments()[0].text; hovered == idle {
		t.Errorf("hovered heart renders identically to idle: %q", hovered)
	}

	m = move(statusX, statusY+4)
	if m.hoveredStatus != statusActionNone {
		t.Errorf("moving off the status row: got %d, want none", m.hoveredStatus)
	}
	if back := m.statusSegments()[0].text; back != idle {
		t.Errorf("heart did not return to idle styling: got %q, want %q", back, idle)
	}
}

func TestPointInVideo(t *testing.T) {
	m := testModel()
	player.VideoWidthChars = 30
	player.VideoHeightChars = 20

	top, left := m.videoRow-1, m.videoCol-1
	cases := []struct {
		name string
		x, y int
		want bool
	}{
		{"top-left corner", left, top, true},
		{"bottom-right corner", left + 29, top + 19, true},
		{"one row above", left, top - 1, false},
		{"one row below", left, top + 20, false},
		{"one column left", left - 1, top, false},
		{"one column right", left + 30, top, false},
	}
	for _, tc := range cases {
		if got := m.pointInVideo(tc.x, tc.y); got != tc.want {
			t.Errorf("%s: pointInVideo(%d, %d) = %v, want %v", tc.name, tc.x, tc.y, got, tc.want)
		}
	}
}

// The scrub zone must be the reel's bottom row — where the player burns the
// progress bar into the frame — and nowhere else.
func TestProgressBarScrubZone(t *testing.T) {
	m := testModel()
	player.VideoWidthChars = 30
	player.VideoHeightChars = 20

	top, left := m.videoRow-1, m.videoCol-1
	bottom := top + player.VideoHeightChars - 1

	if !m.pointOnProgressBar(left, bottom) {
		t.Error("left end of the bottom row is not on the bar")
	}
	if !m.pointOnProgressBar(left+29, bottom) {
		t.Error("right end of the bottom row is not on the bar")
	}
	if m.pointOnProgressBar(left, bottom-1) {
		t.Error("the row above the bottom is on the bar")
	}
	if m.pointOnProgressBar(left, bottom+1) {
		t.Error("the row below the reel is on the bar")
	}
	if m.pointOnProgressBar(left-1, bottom) {
		t.Error("left of the reel is on the bar")
	}
	if m.pointOnProgressBar(left+30, bottom) {
		t.Error("right of the reel is on the bar")
	}
}

func TestVolumeSliderSetsExactFractionAndDrags(t *testing.T) {
	m := testModel()
	m.width, m.height = 100, 40
	m.backend = &stubBackend{}
	start := m.volumeTrackStart()

	updated, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      start + volumeTrackWidth/4,
		Y:      m.volumeSliderRow(),
	})
	m = updated.(Model)
	if got := m.player.Volume(); got < 0.24 || got > 0.26 {
		t.Errorf("quarter click set volume to %.2f", got)
	}
	if !m.volumeDragging {
		t.Fatal("pressing volume track did not start dragging")
	}

	updated, _ = m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      start + volumeTrackWidth,
		Y:      m.volumeSliderRow(),
	})
	m = updated.(Model)
	if got := m.player.Volume(); got != 1 {
		t.Errorf("dragging to end set volume to %.2f, want 1", got)
	}

	updated, _ = m.updateMouse(tea.MouseMsg{Action: tea.MouseActionRelease})
	if updated.(Model).volumeDragging {
		t.Error("volume drag survived mouse release")
	}
}

// Pressing on the bar starts a scrub, and the drag continues after the pointer
// leaves the bar. Releasing ends it.
func TestScrubStartsOnPressAndSurvivesLeavingTheBar(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	player.VideoWidthChars = 30
	player.VideoHeightChars = 20

	left := m.videoCol - 1
	bottom := m.videoRow - 2 + player.VideoHeightChars

	updated, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: left + 15, Y: bottom,
	})
	if !updated.(Model).scrubbing {
		t.Fatal("pressing the progress bar did not start a scrub")
	}

	// Dragging well away from the bar keeps scrubbing rather than falling
	// through to hover.
	dragged, _ := updated.(Model).updateMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft,
		X: left + 20, Y: bottom - 8,
	})
	if !dragged.(Model).scrubbing {
		t.Error("scrub stopped when the pointer left the bar")
	}

	released, _ := dragged.(Model).updateMouse(tea.MouseMsg{Action: tea.MouseActionRelease})
	if released.(Model).scrubbing {
		t.Error("releasing did not end the scrub")
	}
}

// Clicking the reel body still pauses; only the bottom row scrubs.
func TestClickAboveTheBarStillPauses(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	player.VideoWidthChars = 30
	player.VideoHeightChars = 20

	updated, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: m.videoCol - 1 + 5, Y: m.videoRow - 1 + 5,
	})
	if updated.(Model).scrubbing {
		t.Error("clicking the middle of the reel started a scrub")
	}
	if updated.(Model).status != statusPaused {
		t.Error("clicking the middle of the reel did not pause")
	}
}

// One physical wheel notch arrives as a burst of events on a high-resolution
// scroll, which used to move the cursor several rows at a time.
func TestWheelBurstMovesOneStep(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.comments.Open("pk")
	m.comments.SetComments("pk", func() []backend.Comment {
		out := make([]backend.Comment, 20)
		for i := range out {
			out[i] = backend.Comment{PK: string(rune('a' + i)), Username: "u", Text: "t"}
		}
		return out
	}())
	m.comments.View(40, 20, "")

	wheel := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}

	// Five events with no delay stand in for one notch.
	for range 5 {
		updated, _ := m.updateMouse(wheel)
		m = updated.(Model)
	}
	if got := m.comments.CursorIndex(); got != 1 {
		t.Errorf("a burst of 5 wheel events moved the cursor to %d, want 1", got)
	}

	// After the window, the next notch is accepted.
	m.lastWheelStep = m.lastWheelStep.Add(-2 * wheelStepInterval)
	updated, _ := m.updateMouse(wheel)
	if got := updated.(Model).comments.CursorIndex(); got != 2 {
		t.Errorf("after the throttle window the cursor is at %d, want 2", got)
	}
}
