package tui

import (
	"strings"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	"github.com/charmbracelet/lipgloss"
)

func TestProfileHeaderFitsAndActionsMatchRenderedGeometry(t *testing.T) {
	for _, following := range []bool{false, true} {
		header := buildProfileHeaderLayout(backend.CreatorProfileState{
			Username:  strings.Repeat("creator", 20),
			Following: following,
		}, 48)
		if got := lipgloss.Width(header.plain()); got > 48 {
			t.Fatalf("header width = %d, want <= 48", got)
		}
		if got := header.actionAt(0); got != "back" {
			t.Fatalf("first cell action = %q, want back", got)
		}
		followStart := lipgloss.Width(header.back) + lipgloss.Width(profileHeaderDivider) +
			lipgloss.Width(header.identity) + lipgloss.Width(profileHeaderDivider)
		if got := header.actionAt(followStart); got != "follow" {
			t.Fatalf("follow cell action = %q, want follow", got)
		}
		if got := header.actionAt(lipgloss.Width(header.plain())); got != "" {
			t.Fatalf("past-header action = %q, want empty", got)
		}
	}
}
