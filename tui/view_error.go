package tui

import (
	"os"
	"path/filepath"
	"strings"
)

func (m Model) viewError() string {
	var b strings.Builder

	b.WriteString(pink400.Bold(true).Render("Something went wrong"))
	b.WriteString("\n\n")

	if m.lastErr != nil {
		b.WriteString(red500.Render(m.lastErr.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(gray400.Render("Things that usually fix it:"))
	b.WriteString("\n")
	b.WriteString(gray500.Render("  • Press ") + yellow300.Bold(true).Render("r") + gray500.Render(" to retry — startup failures are often transient."))
	b.WriteString("\n")
	b.WriteString(gray500.Render("  • Clear the browser profile: ") + gray300.Render("rm -rf "+chromeDataDir()))
	b.WriteString("\n")
	b.WriteString(gray500.Render("  • Check the log: ") + gray300.Render(logPath()))
	b.WriteString("\n\n")

	b.WriteString(gray600.Render("r: retry    q: quit"))
	b.WriteString("\n")

	block := b.String()
	return "\n    " + strings.ReplaceAll(block, "\n", "\n    ")
}

// These mirror the paths main.go builds. They're recomputed rather than
// threaded through the model because they're only needed to print advice.
func chromeDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.local/share/reels"
	}
	return filepath.Join(home, ".local", "share", "reels")
}

func logPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "~/.local/state/reels/reels.log"
	}
	return filepath.Join(home, ".local", "state", "reels", "reels.log")
}
