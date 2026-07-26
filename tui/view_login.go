package tui

import (
	"strings"

	"github.com/ademiru/TermiReels/tui/colors"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewLogin() string {
	if m.width == 0 || m.height == 0 {
		return "Login required..."
	}

	badge := func(text string) string {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(colors.Yellow300Color).
			Background(colors.Red700Color).
			Padding(0, 1).
			Render(text)
	}

	var title, instructions, statusLine string
	help := gray600.Render("q: quit")

	switch {
	case m.loginSuccess:
		title = pink400.Bold(true).Render("Login successful!")
		instructions = badge("IMPORTANT") + "\nTell Instagram to " + badge("save your login info") +
			" for next time,\nthen restart the app."

	case m.loginRestarting:
		title = pink400.Bold(true).Render("Opening browser")
		instructions = "Launching a browser window for Instagram..."
		statusLine = m.spinner.View() + " Starting..."

	case m.flags.LoginMode:
		title = pink400.Bold(true).Render("Manual login")
		instructions = "Please log in to Instagram in the browser window."
		statusLine = m.spinner.View() + " Waiting for login..."

	default:
		// Normal mode with no session. Offer to relaunch the browser here
		// rather than sending the user away to re-run with a flag.
		title = pink400.Bold(true).Render("Login required")
		instructions = "reels needs an Instagram session to fetch reels.\n\n" +
			"Press " + yellow300.Bold(true).Render("enter") + " to open a browser window and log in."
		help = gray600.Render("enter: log in    q: quit")
	}

	content := []string{
		title,
		"",
		instructions,
		"",
		statusLine,
		"",
		help,
	}

	block := strings.Join(content, "\n")
	return strings.Repeat(" ", 4) + strings.ReplaceAll(block, "\n", "\n    ")
}
