package tui

import (
	"strings"

	"github.com/ademiru/TermiReels/backend"
	"github.com/charmbracelet/lipgloss"
)

const profileHeaderDivider = "  │  "

type profileHeaderLayout struct {
	back     string
	identity string
	follow   string
}

func buildProfileHeaderLayout(state backend.CreatorProfileState, width int) profileHeaderLayout {
	back := "🏠 MAIN"
	follow := "➕ FOLLOW"
	if state.Following {
		follow = "✅ FOLLOWING"
	}
	fixed := lipgloss.Width(back) + lipgloss.Width(follow) +
		2*lipgloss.Width(profileHeaderDivider) + lipgloss.Width("👤 @")
	nameBudget := max(width-fixed, 4)
	name := truncateByWidth(state.Username, nameBudget)
	return profileHeaderLayout{
		back:     back,
		identity: "👤 @" + name,
		follow:   follow,
	}
}

func (h profileHeaderLayout) render() string {
	return purple400.Bold(true).Render(h.back) +
		gray700.Render(profileHeaderDivider) +
		pink400.Bold(true).Render(h.identity) +
		gray700.Render(profileHeaderDivider) +
		blue400.Bold(true).Render(h.follow)
}

func (h profileHeaderLayout) actionAt(offset int) string {
	if offset < 0 {
		return ""
	}
	backEnd := lipgloss.Width(h.back)
	if offset < backEnd {
		return "back"
	}
	followStart := backEnd + lipgloss.Width(profileHeaderDivider) +
		lipgloss.Width(h.identity) + lipgloss.Width(profileHeaderDivider)
	if offset >= followStart && offset < followStart+lipgloss.Width(h.follow) {
		return "follow"
	}
	return ""
}

func (h profileHeaderLayout) plain() string {
	return strings.Join([]string{h.back, h.identity, h.follow}, profileHeaderDivider)
}
