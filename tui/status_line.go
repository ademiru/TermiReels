package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ademiru/TermiReels/tui/colors"
	"github.com/charmbracelet/lipgloss"
)

// statusAction is what activating a status-line segment does. Segments that
// are only decoration (the gaps between icons) carry statusActionNone.
type statusAction int

const (
	statusActionNone statusAction = iota
	statusActionLike
	statusActionComments
	statusActionRepost
	statusActionSave
	statusActionShare
	statusActionPause
	statusActionMute
)

// statusSegment is one run of the status line. The renderer concatenates
// text; the mouse handler walks the same list to map a column back to an
// action, so the two can't drift apart.
type statusSegment struct {
	text   string
	action statusAction
}

var statusActionLabels = map[statusAction]string{
	statusActionLike:     "LIKE",
	statusActionComments: "COMMENTS",
	statusActionRepost:   "REPOST",
	statusActionSave:     "SAVE",
	statusActionShare:    "SHARE",
	statusActionPause:    "PLAY",
	statusActionMute:     "SOUND",
}

const statusGap = "  "
const statusCompactGap = " "

// Each control gets its own hue so the row reads as a set of distinct buttons
// rather than one grey string. Inactive controls stay muted; acting on one
// lights it up.
var (
	statusIdleColor    = colors.Gray500Color
	statusLikeColor    = colors.Red500Color
	statusCommentColor = colors.Blue400Color
	statusRepostColor  = colors.Purple300Color
	statusSaveColor    = colors.Yellow400Color
	statusShareColor   = colors.Pink300Color
	statusPauseColor   = colors.Gray300Color
	statusMuteColor    = colors.Orange400Color
)

// Reposting turns the icon for a moment, so the action registers as having
// happened rather than only as a colour that changed.
//
// The frames are all single-rune emoji that measure two columns, matching the
// resting icon: a frame of a different width would shift every control to its
// right for the length of the animation.
const (
	repostSpinFrames   = 8
	repostSpinInterval = 80 * time.Millisecond
)

var repostSpinCycle = []string{"🔃", "🔄"}

// statusIconCells is the column width every status icon occupies.
//
// Mixing single- and double-width glyphs on this row is what breaks clicking:
// symbols like ⇄, ↗, ⚐ and ▶ have ambiguous East Asian width, so a terminal is
// free to draw them one or two cells wide while the layout assumes the other.
// Every column after such a glyph then sits somewhere the hit-test doesn't
// expect. Sticking to icons that measure exactly two columns, and padding
// anything narrower up to two, keeps the drawn row and the hit-test aligned.
const statusIconCells = 2

// statusPillPad is the space on each side of a control's label, which is what
// gives the filled state a pill shape rather than a colour-flooded glyph.
const statusPillPad = 1

// Counts reserve a fixed number of cells so moving between reels cannot shift
// every control beside them. This also keeps hover hitboxes stationary.
const statusCountCells = 5

func fixedStatusCount(count string, cells int) string {
	count = truncateByWidth(count, cells)
	return count + strings.Repeat(" ", max(cells-lipgloss.Width(count), 0))
}

// padIcon widens an icon to statusIconCells so a state change (play to pause,
// muted to unmuted) can never shift the controls beside it.
func padIcon(icon string) string {
	if w := lipgloss.Width(icon); w < statusIconCells {
		return icon + strings.Repeat(" ", statusIconCells-w)
	}
	return icon
}

// statusSegments builds the status line: like, comments, reposts, bookmark,
// share, play/pause and mute, in that order.
func (m Model) statusSegments() []statusSegment {
	compact := m.width > 0 && m.width < 76
	countCells := statusCountCells
	pillPad := statusPillPad
	gapText := statusGap
	if compact {
		countCells = 4
		pillPad = 0
		gapText = statusCompactGap
	}

	// button renders every control with the same interaction language:
	// neutral at rest, a quiet dark hover surface, and a bright underlined
	// foreground when engaged. Keeping active controls background-free avoids
	// opaque terminal colour blocks and preserves coloured emoji.
	button := func(action statusAction, icon, count string, accent lipgloss.Color, on bool) statusSegment {
		label := padIcon(icon)
		if count != "" {
			label += " " + fixedStatusCount(count, countCells)
		}
		pad := strings.Repeat(" ", pillPad)
		label = pad + label + pad

		enabled := m.currentReel != nil
		if enabled && action == statusActionComments {
			enabled = !m.currentReel.CommentsDisabled
		}
		if enabled && action == statusActionShare {
			enabled = m.currentReel.CanViewerReshare
		}

		var style lipgloss.Style
		switch {
		case !enabled:
			style = lipgloss.NewStyle().Foreground(colors.Gray700Color).Faint(true)
		case on:
			style = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true).
				Underline(true)
		case m.hoveredStatus == action:
			// Hovering previews the accent the control takes when engaged.
			style = lipgloss.NewStyle().Foreground(accent).Background(colors.Gray800Color).Bold(true)
		default:
			style = lipgloss.NewStyle().Foreground(statusIdleColor)
		}
		return statusSegment{text: style.Render(label), action: action}
	}

	heartIcon, liked := "🤍", false
	saved := false
	var likeCount, commentCount string

	if m.currentReel != nil {
		if m.currentReel.Liked {
			heartIcon, liked = "❤️", true
		}
		saved = m.currentReel.Saved
		likeCount = formatLikeCount(m.currentReel.LikeCount)
		commentCount = formatLikeCount(m.currentReel.CommentCount)
	}

	// The repost count carries no information — you either have reposted a
	// reel or you haven't, so it only ever read 0 or 1. The icon says it, the
	// pill fills, and it turns while the change registers.
	repostIcon := "🔁"
	reposted := m.currentReel != nil && m.currentReel.Reposted
	if m.repostSpin > 0 {
		repostIcon = repostSpinCycle[(repostSpinFrames-m.repostSpin)%len(repostSpinCycle)]
	}

	playPauseIcon, paused := "▶️", false
	if m.player.IsPaused() {
		playPauseIcon, paused = "⏸️", true
	}

	muteIcon, muted := "🔊", false
	if m.player.IsMuted() {
		muteIcon, muted = "🔇", true
	}

	// Reels that can't be reshared get a dimmed icon rather than a hole in
	// the row, so the other controls don't shift position between reels.
	shareIcon, shareColor := "✈️", statusIdleColor
	shareOn := false
	if m.currentReel != nil && m.currentReel.CanViewerReshare {
		shareColor = statusShareColor
		if m.shareConfirmed {
			shareIcon, shareColor, shareOn = "✅", colors.Yellow300Color, true
		}
	}

	gap := statusSegment{text: gapText}
	divider := statusSegment{text: gray800.Render(" │ ")}
	if compact {
		divider.text = gray800.Render("│")
	}
	return []statusSegment{
		button(statusActionLike, heartIcon, likeCount, statusLikeColor, liked),
		gap,
		button(statusActionComments, "💬", commentCount, statusCommentColor, false),
		gap,
		button(statusActionRepost, repostIcon, "", statusRepostColor, reposted || m.repostSpin > 0),
		gap,
		button(statusActionShare, shareIcon, "", shareColor, shareOn),
		divider,
		button(statusActionSave, "🔖", "", statusSaveColor, saved),
		divider,
		button(statusActionPause, playPauseIcon, "", statusPauseColor, paused),
		gap,
		button(statusActionMute, muteIcon, "", statusMuteColor, muted),
	}
}

// statusActionAt maps a column offset from the start of the status line to the
// action drawn there, or statusActionNone if the column is a gap or past the
// end. Gaps are attributed to the segment on their left so that clicking just
// beside an icon still hits it.
func (m Model) statusActionAt(offset int) statusAction {
	if offset < 0 {
		return statusActionNone
	}
	col := 0
	last := statusActionNone
	for _, seg := range m.statusSegments() {
		w := lipgloss.Width(seg.text)
		if seg.action != statusActionNone {
			last = seg.action
		}
		if offset < col+w {
			if seg.action != statusActionNone {
				return seg.action
			}
			return last
		}
		col += w
	}
	return statusActionNone
}

// statusLabels mirrors the exact segment widths of the icon row, so every
// small caption sits directly under its control without changing hitboxes.
func (m Model) statusLabels() string {
	var b strings.Builder
	for _, seg := range m.statusSegments() {
		w := lipgloss.Width(seg.text)
		label := statusActionLabels[seg.action]
		if label == "" {
			b.WriteString(strings.Repeat(" ", w))
			continue
		}
		label = truncateByWidth(label, w)
		left := max((w-lipgloss.Width(label))/2, 0)
		right := max(w-left-lipgloss.Width(label), 0)
		style := gray800
		if m.hoveredStatus == seg.action {
			style = gray500.Bold(true)
		}
		b.WriteString(strings.Repeat(" ", left) + style.Render(label) + strings.Repeat(" ", right))
	}
	return b.String()
}

const volumeTrackWidth = 20

// volumeSlider renders a stable-width direct-manipulation control. The track
// geometry is also used by the mouse handler.
func (m Model) volumeSlider() string {
	vol := min(max(m.player.Volume(), 0), 1)
	filled := int(vol*volumeTrackWidth + 0.5)
	filled = min(max(filled, 0), volumeTrackWidth)

	var track strings.Builder
	for i := 0; i <= volumeTrackWidth; i++ {
		switch {
		case i == filled:
			track.WriteString(pink300.Render("●"))
		case i < filled:
			track.WriteString(purple400.Render("━"))
		default:
			track.WriteString(gray800.Render("━"))
		}
	}
	icon := "VOL "
	if m.player.IsMuted() {
		icon = "MUTE"
	}
	return gray600.Render(icon+" ") + track.String() +
		gray500.Render(fmt.Sprintf(" %3d%%", int(vol*100+0.5)))
}

func (m Model) volumeSliderStart() int {
	return max((m.width-lipgloss.Width(m.volumeSlider()))/2, 0)
}

func (m Model) volumeTrackStart() int {
	return m.volumeSliderStart() + 5 // "VOL  " and "MUTE " are both fixed
}
