package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestLoadingScreenFillsViewportWithoutOverflow(t *testing.T) {
	view := renderLoadingScreen(80, 31, "1.5.0", "Preparing your Reels session", gray400, 3)
	lines := strings.Split(view, "\n")
	if len(lines) != 31 {
		t.Fatalf("height = %d, want 31", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width != 80 {
			t.Fatalf("line %d width = %d, want 80", i, width)
		}
	}
	if strings.Contains(strings.ToLower(view), "bark fart") {
		t.Fatal("loading screen contains legacy novelty copy")
	}
}

func TestNyanRunnerAnimatesAtStableSize(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	first := renderNyanRunner(44, 10)
	next := renderNyanRunner(44, 11)
	if first == next {
		t.Fatal("nyan runner did not advance")
	}
	if lipgloss.Height(first) != 3 || lipgloss.Height(next) != 3 {
		t.Fatal("nyan runner height changed between frames")
	}
	for i, frame := range []string{first, next} {
		for row, line := range strings.Split(frame, "\n") {
			if width := lipgloss.Width(line); width != 44 {
				t.Fatalf("frame %d row %d width = %d, want 44", i, row, width)
			}
		}
	}
}

func TestLoadingMessageWaveMovesWithoutLayoutShift(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	first := renderLoadingWave("Preparing your Reels session", gray400, 15, 44)
	next := renderLoadingWave("Preparing your Reels session", gray400, 16, 44)
	if first == next {
		t.Fatal("message shimmer did not advance")
	}
	if lipgloss.Width(first) != 44 || lipgloss.Width(next) != 44 {
		t.Fatal("message shimmer changed its layout width")
	}
}
