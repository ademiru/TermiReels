package tui

import (
	"fmt"
	"strings"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/tui/colors"
	"github.com/charmbracelet/lipgloss"
)

const profileHeaderDivider = " "

type profileHeaderLayout struct {
	back     string
	identity string
	follow   string
}

func buildProfileHeaderLayout(state backend.CreatorProfileState, width int) profileHeaderLayout {
	back := " ‹ FEED "
	follow := ""
	source := ""
	switch {
	case state.Loading:
		source = "  ◌ SYNCING"
	case state.Error != "":
		source = "  ! " + state.Error
	case state.Total > 0:
		source = fmt.Sprintf("  • %d REELS", state.Total)
	}
	if width < 60 {
		switch {
		case state.Loading:
			source = "  ◌"
		case state.Error != "":
			source = "  !"
		case state.Total > 0:
			source = fmt.Sprintf("  • %d", state.Total)
		}
	}
	fixed := lipgloss.Width(back) + lipgloss.Width(profileHeaderDivider) +
		lipgloss.Width("@") + lipgloss.Width(source)
	nameBudget := max(width-fixed, 4)
	name := truncateByWidth(state.Username, nameBudget)
	return profileHeaderLayout{
		back:     back,
		identity: "@" + name + source,
		follow:   follow,
	}
}

func (h profileHeaderLayout) render() string {
	back := lipgloss.NewStyle().
		Foreground(colors.Purple100Color).
		Background(colors.Gray900Color).
		Bold(true).
		Render(h.back)
	identity := gradientText(h.identity, brandRamp, lipgloss.NewStyle().Bold(true))
	result := back +
		gray800.Render(profileHeaderDivider) +
		identity
	if h.follow != "" {
		result += gray800.Render(profileHeaderDivider) + h.follow
	}
	return result
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

func profileInlineFollowLabel(state backend.CreatorProfileState) string {
	if state.Following {
		return " FOLLOWED "
	}
	return " FOLLOW "
}

func renderProfileInlineFollow(state backend.CreatorProfileState) string {
	style := lipgloss.NewStyle().
		Foreground(colors.WhiteColor).
		Background(colors.Purple700Color).
		Bold(true)
	if state.Following {
		style = style.
			Foreground(colors.Purple50Color).
			Background(colors.Purple900Color)
	}
	return style.Render(profileInlineFollowLabel(state))
}

func (h profileHeaderLayout) plain() string {
	if h.follow == "" {
		return strings.Join([]string{h.back, h.identity}, profileHeaderDivider)
	}
	return strings.Join([]string{h.back, h.identity, h.follow}, profileHeaderDivider)
}
