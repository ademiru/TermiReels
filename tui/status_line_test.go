package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// statusLineWidth is the drawn width of the whole row.
func statusLineWidth(m Model) int {
	w := 0
	for _, seg := range m.statusSegments() {
		w += lipgloss.Width(seg.text)
	}
	return w
}

// Toggling any control must not move the ones beside it. When an icon changes
// width with its state the whole row shifts, and every click after it lands on
// the wrong control — which is exactly how the ambiguous-width glyphs behaved.
func TestStatusLineWidthIsStableAcrossStates(t *testing.T) {
	base := testModel()
	base.currentReel = &backend.ReelInfo{}
	want := statusLineWidth(base)

	cases := []struct {
		name  string
		apply func(m *Model)
	}{
		{"liked", func(m *Model) { m.currentReel.Liked = true }},
		{"reposted", func(m *Model) { m.currentReel.Reposted = true }},
		{"saved", func(m *Model) { m.currentReel.Saved = true }},
		{"reshareable", func(m *Model) { m.currentReel.CanViewerReshare = true }},
		{"share confirmed", func(m *Model) {
			m.currentReel.CanViewerReshare = true
			m.shareConfirmed = true
		}},
		{"paused", func(m *Model) { m.player.Pause() }},
		{"muted", func(m *Model) { m.player.Mute() }},
		{"hovered", func(m *Model) { m.hoveredStatus = statusActionLike }},
	}

	for _, tc := range cases {
		m := testModel()
		m.currentReel = &backend.ReelInfo{}
		tc.apply(&m)
		if got := statusLineWidth(m); got != want {
			t.Errorf("%s: status line is %d columns, want %d", tc.name, got, want)
		}
	}
}

// Every icon must measure exactly statusIconCells.
//
// lipgloss is the authority here, not runewidth: it is what statusActionAt
// measures with, and its width table handles variation-selector emoji
// (❤️ = U+2764 U+FE0F) the way a modern terminal does. runewidth still reports
// those as one column. What must be avoided is the other class of glyph —
// bare symbols like ⇄, ↗ and ⚐ with ambiguous East Asian width, where the
// terminal itself is free to disagree with any library.
func TestStatusIconsHaveUnambiguousWidth(t *testing.T) {
	m := testModel()
	m.currentReel = &backend.ReelInfo{}
	m.currentReel.CanViewerReshare = true

	// Sweep the states so every icon variant gets rendered at least once.
	variants := []Model{m}

	liked := m
	liked.currentReel = &backend.ReelInfo{}
	liked.currentReel.Liked = true
	liked.currentReel.Reposted = true
	liked.currentReel.Saved = true
	liked.currentReel.CanViewerReshare = true
	variants = append(variants, liked)

	paused := testModel()
	paused.currentReel = &backend.ReelInfo{}
	paused.player.Pause()
	paused.player.Mute()
	variants = append(variants, paused)

	for _, v := range variants {
		for _, seg := range v.statusSegments() {
			if seg.action == statusActionNone {
				continue
			}
			// The label is pill-padded, so the icon starts statusPillPad
			// columns in.
			body := seg.text[statusPillPad:]
			icon := truncateByWidth(body, statusIconCells)
			if lg := lipgloss.Width(icon); lg != statusIconCells {
				t.Errorf("icon %q is %d columns, want %d", icon, lg, statusIconCells)
			}
		}
	}
}

func TestStatusClickTargetIsFooterOnly(t *testing.T) {
	m := testModel()
	m.width, m.height = 100, 40
	statusY := m.statusLineRow()
	statusX := m.statusLineStart()

	if got := m.statusActionAt2D(statusX, statusY); got != statusActionLike {
		t.Errorf("on the status row: got %d, want like", got)
	}
	if got := m.statusActionAt2D(statusX, statusY-1); got != statusActionNone {
		t.Errorf("above the status row: got %d, want none", got)
	}
}

// The avatar is drawn over the leftmost pfpGutter columns of the two rows
// under the reel. No text on any of those rows may start inside that band, or
// the avatar sits on top of it.
func TestTextBelowReelClearsTheAvatarGutter(t *testing.T) {
	m := testModel()
	m.state = stateBrowsing
	m.backend = &stubBackend{}
	m.width, m.height = 100, 40
	m.showNavbar = true
	m.currentReel = &backend.ReelInfo{}
	m.currentReel.Username = "someone"
	m.currentReel.Caption = "a caption long enough to be worth scrolling sideways for #tag"

	lines := strings.Split(m.viewBrowsing(), "\n")
	firstBelow := m.videoRow + player.VideoHeightChars - 1
	wantIndent := max(m.videoCol-1, 0) + pfpGutter

	for i := firstBelow; i < min(len(lines), m.statusLineRow()); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent < wantIndent {
			t.Errorf("line %d starts at column %d, inside the avatar gutter (want >= %d)\n  %q",
				i, indent, wantIndent, line)
		}
	}
}

// Every engaged control uses the same bright, background-free visual language.
func TestEngagedControlsShareVisualLanguage(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	segmentFor := func(m Model, action statusAction) string {
		for _, seg := range m.statusSegments() {
			if seg.action == action {
				return seg.text
			}
		}
		t.Fatalf("no segment for action %d", action)
		return ""
	}

	// "48;2;" is the truecolor background introducer.
	const bgIntroducer = "48;2;"

	idle := testModel()
	idle.currentReel = &backend.ReelInfo{}
	if got := segmentFor(idle, statusActionRepost); strings.Contains(got, bgIntroducer) {
		t.Errorf("an idle control is filled: %q", got)
	}

	on := testModel()
	on.currentReel = &backend.ReelInfo{}
	on.currentReel.Reposted = true
	if got := segmentFor(on, statusActionRepost); strings.Contains(got, bgIntroducer) ||
		!strings.Contains(got, "38;2;") {
		t.Errorf("a reposted control does not use the shared accent language: %q", got)
	}

	// The in-flight state uses the same treatment, so it cannot resize or flash.
	spinning := testModel()
	spinning.currentReel = &backend.ReelInfo{}
	spinning.repostSpin = repostSpinFrames
	if got := segmentFor(spinning, statusActionRepost); strings.Contains(got, bgIntroducer) ||
		!strings.Contains(got, "38;2;") {
		t.Errorf("a spinning control does not use the shared accent language: %q", got)
	}
}

func TestLikedHeartStaysBrightWithoutDarkBackground(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := testModel()
	m.currentReel = &backend.ReelInfo{}
	m.currentReel.Liked = true
	m.currentReel.LikeCount = 42

	var like string
	for _, seg := range m.statusSegments() {
		if seg.action == statusActionLike {
			like = seg.text
			break
		}
	}
	if strings.Contains(like, "48;2;") {
		t.Errorf("liked heart still has an opaque background: %q", like)
	}
	if !strings.Contains(like, "38;2;") || !strings.Contains(like, "❤") {
		t.Errorf("liked heart lost its bright foreground treatment: %q", like)
	}
}

func TestFooterWidthDoesNotChangeWithCounts(t *testing.T) {
	m := testModel()
	m.width = 100
	m.currentReel = &backend.ReelInfo{}

	width := func() int {
		total := 0
		for _, seg := range m.statusSegments() {
			total += lipgloss.Width(seg.text)
		}
		return total
	}

	m.currentReel.LikeCount = 1
	m.currentReel.CommentCount = 2
	smallWidth := width()
	smallStart := m.statusLineStart()

	m.currentReel.LikeCount = 9876543
	m.currentReel.CommentCount = 123456
	if got := width(); got != smallWidth {
		t.Errorf("footer width changed from %d to %d when counts grew", smallWidth, got)
	}
	if got := m.statusLineStart(); got != smallStart {
		t.Errorf("footer start moved from %d to %d when counts grew", smallStart, got)
	}
}

func TestCompactFooterFitsNarrowTerminal(t *testing.T) {
	m := testModel()
	m.width = 64
	m.currentReel = &backend.ReelInfo{}
	m.currentReel.LikeCount = 9876543
	m.currentReel.CommentCount = 123456
	if got := statusLineWidth(m); got > m.width-2 {
		t.Errorf("compact footer is %d columns in a %d-column terminal", got, m.width)
	}
}

func TestFooterLabelsAlignWithIconRow(t *testing.T) {
	for _, width := range []int{64, 100} {
		m := testModel()
		m.width = width
		m.currentReel = &backend.ReelInfo{}
		if icons, labels := statusLineWidth(m), lipgloss.Width(m.statusLabels()); icons != labels {
			t.Errorf("width=%d: icon row=%d, label row=%d", width, icons, labels)
		}
	}
}

// The send button sweeps the brand ramp, so it reads as one object rather than
// a row of separately coloured letters.
func TestSendButtonIsAGradient(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	sp := NewSharePanel()
	sp.Open()
	sp.SetFriends(shareTestFriends(2))
	sp.ToggleSelected()

	// Each character carries its own escape sequence, so the label is not a
	// contiguous substring here. TestSendButtonSpanMatchesRenderedHeader
	// checks the text itself, with styling stripped.
	header := sp.header()

	// Assert the shape rather than exact bytes: lipgloss round-trips colours
	// through floating point, so a channel can come out one off from what
	// gradientRamp computed.
	backgrounds := truecolorBackgrounds(header)
	if len(backgrounds) < 4 {
		t.Fatalf("send button emitted %d background colours, expected a sweep", len(backgrounds))
	}

	distinct := map[string]bool{}
	for _, bg := range backgrounds {
		distinct[bg] = true
	}
	if len(distinct) < len(backgrounds) {
		t.Errorf("send button repeats colours instead of sweeping: %v", backgrounds)
	}
}

var truecolorBgPattern = regexp.MustCompile(`48;2;(\d+);(\d+);(\d+)`)

// truecolorBackgrounds pulls the 24-bit background colours out of a rendered
// string, in the order they appear.
func truecolorBackgrounds(s string) []string {
	matches := truecolorBgPattern.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, fmt.Sprintf("%s,%s,%s", m[1], m[2], m[3]))
	}
	return out
}
