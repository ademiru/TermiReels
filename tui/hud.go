package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ademiru/TermiReels/tui/colors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// HUD message types
type (
	volumeHoldMsg         struct{ gen int }
	volumeFadeTickMsg     struct{}
	dmNotifyHoldMsg       struct{}
	dmNotifyFadeTickMsg   struct{}
	chatBannerHoldMsg     struct{ gen int }
	chatBannerFadeTickMsg struct{}
	toastHoldMsg          struct{ gen int }
	toastFadeTickMsg      struct{}
)

// hudItem identifies which overlay is currently displayed.
// Higher values = higher priority.
type hudItem int

const (
	hudNone hudItem = iota
	hudChatBanner
	hudVolume
	hudDMNotify
	// hudToast outranks the rest: it always acknowledges something the user
	// just did, so it should never be suppressed by an older overlay.
	hudToast
)

// HUD holds state for heads-up display overlays (volume indicator, notifications).
type HUD struct {
	active hudItem

	// volume: 0=hidden, 1=visible (holding), 2-7=fading out
	volumeFadeStep int
	volumeGen      int

	// DM notification: 0=hidden, 1=visible (holding), 2-7=fading out
	dmNotifyFadeStep int
	dmNotifyCount    int

	// chat banner: 0=hidden, 1=visible (holding), 2-7=fading out
	chatBannerFadeStep int
	chatBannerGen      int
	chatBannerTitle    string
	chatBannerKeys     []string

	// toast: 0=hidden, 1=visible (holding), 2-7=fading out
	toastFadeStep int
	toastGen      int
	toastText     string
}

// ShowToast flashes a short message above the reel. Used for acknowledgements
// that have no dedicated overlay: config reloads, seek positions, copied links.
func (h *HUD) ShowToast(text string) tea.Cmd {
	h.active = hudToast
	h.toastFadeStep = 1
	h.toastText = text
	h.toastGen++
	return h.toastHoldTick()
}

// ShowVolume triggers the volume indicator
func (h *HUD) ShowVolume() tea.Cmd {
	if h.active > hudVolume {
		return nil
	}
	h.active = hudVolume
	h.volumeFadeStep = 1
	h.volumeGen++
	return h.volumeHoldTick()
}

// ShowDMNotify triggers the DM reels notification
func (h *HUD) ShowDMNotify(count int) tea.Cmd {
	if h.active == hudVolume {
		h.volumeFadeStep = 0
	}
	h.active = hudDMNotify
	h.dmNotifyFadeStep = 1
	h.dmNotifyCount = count
	return h.dmNotifyHoldTick()
}

// ShowChatBanner triggers the ephemeral chat-mode banner
func (h *HUD) ShowChatBanner(title string, keysReactOpen []string) tea.Cmd {
	if h.active == hudVolume {
		h.volumeFadeStep = 0
	}
	if h.active == hudDMNotify {
		h.dmNotifyFadeStep = 0
	}
	h.active = hudChatBanner
	h.chatBannerFadeStep = 1
	h.chatBannerTitle = title
	h.chatBannerKeys = keysReactOpen
	h.chatBannerGen++
	return h.chatBannerHoldTick()
}

// HideChatBanner dismisses the banner immediately. Called on chat-mode
// exit, where the react hint would be stale.
func (h *HUD) HideChatBanner() {
	h.chatBannerFadeStep = 0
	h.chatBannerGen++
	if h.active == hudChatBanner {
		h.active = hudNone
	}
}

// viewHUD renders the heads-up display overlay area above the video.
// topPad is the total number of lines available above the status line.
func (m Model) viewHUD(videoWidthChars, topPad int, padding string) string {
	if m.profileOpening || m.profileClosing {
		return m.viewProfileTransitionHUD(videoWidthChars, topPad, padding)
	}
	if m.followTarget != "" {
		return m.viewFollowTransitionHUD(videoWidthChars, topPad, padding)
	}
	if m.hud.active == hudNone {
		return strings.Repeat("\n", max(topPad-1, 0))
	}
	if topPad < 3 {
		text := ""
		step := 1
		switch m.hud.active {
		case hudToast:
			text, step = m.hud.toastText, m.hud.toastFadeStep
		case hudDMNotify:
			text, step = fmt.Sprintf("%d new reels from friends", m.hud.dmNotifyCount), m.hud.dmNotifyFadeStep
		case hudVolume:
			text, step = fmt.Sprintf("volume %d%%", int(m.player.Volume()*100+0.5)), m.hud.volumeFadeStep
		case hudChatBanner:
			text = fmt.Sprintf("From: %s · %s react", m.hud.chatBannerTitle, displayKeys(m.hud.chatBannerKeys))
			step = m.hud.chatBannerFadeStep
		}
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(hudFadeColor(step))).
			Background(colors.Gray900Color).
			Bold(true).
			Padding(0, 1)
		badge := style.Render(text)
		left := max((videoWidthChars-1-lipgloss.Width(badge))/2, 0)
		return padding + strings.Repeat(" ", left) + badge
	}

	var b strings.Builder
	b.WriteString(strings.Repeat("\n", max(topPad-3, 0)))

	switch m.hud.active {
	case hudToast:
		fadeColor := lipgloss.Color(hudFadeColor(m.hud.toastFadeStep))
		style := lipgloss.NewStyle().Foreground(fadeColor)
		b.WriteString(padding + centerInWidth(m.hud.toastText, videoWidthChars-1, style) + "\n\n")

	case hudDMNotify:
		fadeColor := lipgloss.Color(hudFadeColor(m.hud.dmNotifyFadeStep))
		style := lipgloss.NewStyle().Foreground(fadeColor)
		text := fmt.Sprintf("%d new reels from friends", m.hud.dmNotifyCount)
		b.WriteString(padding + centerInWidth(text, videoWidthChars-1, style) + "\n\n")

	case hudVolume:
		vol := m.player.Volume()
		barWidth := videoWidthChars - 1
		filled := int(vol*float64(barWidth) + 0.5)
		fadeColor := lipgloss.Color(hudFadeColor(m.hud.volumeFadeStep))
		filledStyle := lipgloss.NewStyle().Foreground(fadeColor)
		emptyStyle := lipgloss.NewStyle().Foreground(fadeColor).Faint(true)
		volBar := filledStyle.Render(strings.Repeat("█", filled)) + emptyStyle.Render(strings.Repeat("░", barWidth-filled))
		b.WriteString(padding + volBar + "\n\n")

	case hudChatBanner:
		fadeColor := lipgloss.Color(hudFadeColor(m.hud.chatBannerFadeStep))
		style := lipgloss.NewStyle().Foreground(fadeColor)
		reactKeys := displayKeys(m.hud.chatBannerKeys)
		text := fmt.Sprintf("From: %s | press %s to react", m.hud.chatBannerTitle, reactKeys)
		b.WriteString(padding + centerInWidth(text, videoWidthChars-1, style) + "\n\n")
	}

	return b.String()
}

func (m Model) viewFollowTransitionHUD(videoWidthChars, topPad int, padding string) string {
	target := strings.TrimPrefix(strings.TrimSpace(m.followTarget), "@")
	title := "UPDATING FOLLOW STATUS"
	if target != "" {
		title = "UPDATING @" + target
	}
	accent := lipgloss.NewStyle().
		Foreground(colors.Blue300Color).
		Bold(true)
	muted := lipgloss.NewStyle().
		Foreground(colors.Purple100Color).
		Faint(true)
	badge := accent.Render("◆") + " " + accent.Render(title) + " " + m.spinner.View()
	if topPad < 3 {
		return padding + centerInWidth(title+"  "+m.spinner.View(), videoWidthChars-1, accent)
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("\n", max(topPad-3, 0)))
	b.WriteString(padding + centerInWidth(badge, videoWidthChars-1, lipgloss.NewStyle()) + "\n")
	b.WriteString(padding + centerInWidth(
		"waiting for Instagram to confirm", videoWidthChars-1, muted,
	) + "\n")
	return b.String()
}

// viewProfileTransitionHUD is intentionally persistent: creator resolution can
// take several seconds, so a fading toast would make the app look frozen.
func (m Model) viewProfileTransitionHUD(videoWidthChars, topPad int, padding string) string {
	title := "RETURNING TO MAIN FEED"
	detail := "restoring your exact position"
	if m.profileOpening {
		target := strings.TrimPrefix(strings.TrimSpace(m.profileTarget), "@")
		title = "OPENING CREATOR REELS"
		if target != "" {
			title = "OPENING @" + target
		}
		detail = "verifying and preparing the reel queue"
	}

	accent := lipgloss.NewStyle().
		Foreground(colors.Pink400Color).
		Bold(true)
	muted := lipgloss.NewStyle().
		Foreground(colors.Purple100Color).
		Faint(true)
	badge := accent.Render("◆") + " " + accent.Render(title) + " " + m.spinner.View()

	if topPad < 3 {
		line := centerInWidth(title+"  "+m.spinner.View(), videoWidthChars-1, accent)
		return padding + line
	}

	var b strings.Builder
	b.WriteString(strings.Repeat("\n", max(topPad-3, 0)))
	b.WriteString(padding + centerInWidth(badge, videoWidthChars-1, lipgloss.NewStyle()) + "\n")
	b.WriteString(padding + centerInWidth(detail, videoWidthChars-1, muted) + "\n")
	return b.String()
}

// centerInWidth centers text in width columns, truncating with an ellipsis if
// it doesn't fit. Only the text is styled, so the padding stays transparent.
func centerInWidth(text string, width int, style lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	if runewidth.StringWidth(text) > width {
		text = truncateByWidth(text, max(width-3, 0)) + "..."
	}
	leftPad := max((width-runewidth.StringWidth(text))/2, 0)
	return strings.Repeat(" ", leftPad) + style.Render(text)
}

// updateHUD processes HUD-related messages. Returns (handled, model, cmd).
func (m Model) updateHUD(msg tea.Msg) (bool, Model, tea.Cmd) {
	switch msg := msg.(type) {
	case volumeHoldMsg:
		if msg.gen != m.hud.volumeGen {
			return true, m, nil
		}
		if m.hud.volumeFadeStep == 1 {
			m.hud.volumeFadeStep = 2
			return true, m, m.hud.volumeFadeTick()
		}
		return true, m, nil

	case volumeFadeTickMsg:
		if m.hud.volumeFadeStep < 2 {
			return true, m, nil
		}
		m.hud.volumeFadeStep++
		if m.hud.volumeFadeStep > 7 {
			m.hud.volumeFadeStep = 0
			if m.hud.active == hudVolume {
				m.hud.active = hudNone
			}
			return true, m, nil
		}
		return true, m, m.hud.volumeFadeTick()

	case dmNotifyHoldMsg:
		if m.hud.dmNotifyFadeStep == 1 {
			m.hud.dmNotifyFadeStep = 2
			return true, m, m.hud.dmNotifyFadeTick()
		}
		return true, m, nil

	case dmNotifyFadeTickMsg:
		if m.hud.dmNotifyFadeStep < 2 {
			return true, m, nil
		}
		m.hud.dmNotifyFadeStep++
		if m.hud.dmNotifyFadeStep > 7 {
			m.hud.dmNotifyFadeStep = 0
			if m.hud.active == hudDMNotify {
				m.hud.active = hudNone
			}
			return true, m, nil
		}
		return true, m, m.hud.dmNotifyFadeTick()

	case toastHoldMsg:
		if msg.gen != m.hud.toastGen {
			return true, m, nil
		}
		if m.hud.toastFadeStep == 1 {
			m.hud.toastFadeStep = 2
			return true, m, m.hud.toastFadeTick()
		}
		return true, m, nil

	case toastFadeTickMsg:
		if m.hud.toastFadeStep < 2 {
			return true, m, nil
		}
		m.hud.toastFadeStep++
		if m.hud.toastFadeStep > 7 {
			m.hud.toastFadeStep = 0
			if m.hud.active == hudToast {
				m.hud.active = hudNone
			}
			return true, m, nil
		}
		return true, m, m.hud.toastFadeTick()

	case chatBannerHoldMsg:
		if msg.gen != m.hud.chatBannerGen {
			return true, m, nil
		}
		if m.hud.chatBannerFadeStep == 1 {
			m.hud.chatBannerFadeStep = 2
			return true, m, m.hud.chatBannerFadeTick()
		}
		return true, m, nil

	case chatBannerFadeTickMsg:
		if m.hud.chatBannerFadeStep < 2 {
			return true, m, nil
		}
		m.hud.chatBannerFadeStep++
		if m.hud.chatBannerFadeStep > 7 {
			m.hud.chatBannerFadeStep = 0
			if m.hud.active == hudChatBanner {
				m.hud.active = hudNone
			}
			return true, m, nil
		}
		return true, m, m.hud.chatBannerFadeTick()
	}

	return false, m, nil
}

func (h HUD) toastHoldTick() tea.Cmd {
	gen := h.toastGen
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return toastHoldMsg{gen: gen}
	})
}

func (h HUD) toastFadeTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return toastFadeTickMsg{}
	})
}

func (h HUD) volumeHoldTick() tea.Cmd {
	gen := h.volumeGen
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return volumeHoldMsg{gen: gen}
	})
}

func (h HUD) volumeFadeTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return volumeFadeTickMsg{}
	})
}

func (h HUD) dmNotifyHoldTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return dmNotifyHoldMsg{}
	})
}

func (h HUD) dmNotifyFadeTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return dmNotifyFadeTickMsg{}
	})
}

func (h HUD) chatBannerHoldTick() tea.Cmd {
	gen := h.chatBannerGen
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return chatBannerHoldMsg{gen: gen}
	})
}

func (h HUD) chatBannerFadeTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return chatBannerFadeTickMsg{}
	})
}

// hudFadeColor returns the hex color for the fade-out animation.
// Step 1 = full brightness (gray300), steps 2-7 fade to background.
func hudFadeColor(step int) string {
	colors := [8]string{"#C7C7C7", "#C7C7C7", "#A8A8A8", "#808080", "#6B6B6B", "#555555", "#363636", "#262626"}
	if step < 0 || step >= len(colors) {
		return "#262626"
	}
	return colors[step]
}
