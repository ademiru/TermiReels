package tui

import (
	"time"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateMouse handles mouse input while browsing.
//
// The wheel is the main event: it scrolls whichever panel is open, and moves
// between reels when none is. Clicks activate the status-line icons and
// toggle playback on the reel itself.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case tea.MouseActionRelease:
		m.scrubbing = false
		m.volumeDragging = false
		return m, nil

	case tea.MouseActionMotion:
		// Once a scrub has started, the pointer is free to leave the bar —
		// that's how every other scrubber behaves, and a one-row-tall track
		// would be unusable otherwise.
		if m.scrubbing {
			return m.scrubTo(msg.X)
		}
		if m.volumeDragging {
			return m.setVolumeFromX(msg.X)
		}
		return m.updateHover(msg.X, msg.Y)

	case tea.MouseActionPress:
	default:
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelDown:
		if m.throttleWheel() {
			return m, nil
		}
		return m.mouseScroll(1)
	case tea.MouseButtonWheelUp:
		if m.throttleWheel() {
			return m, nil
		}
		return m.mouseScroll(-1)
	case tea.MouseButtonLeft:
		return m.mouseClick(msg.X, msg.Y)
	}
	return m, nil
}

// wheelStepInterval is the shortest gap between two accepted wheel steps.
//
// Ghostty on Wayland forwards high-resolution scrolling, so one physical notch
// arrives as a burst of events and a single flick jumped several comments at
// once. Dropping the events inside this window makes a notch move one row.
const wheelStepInterval = 70 * time.Millisecond

// throttleWheel reports whether this wheel event should be dropped. The window
// is measured from the last accepted step, so holding a scroll still moves
// steadily instead of stalling.
func (m *Model) throttleWheel() bool {
	now := time.Now()
	if now.Sub(m.lastWheelStep) < wheelStepInterval {
		return true
	}
	m.lastWheelStep = now
	return false
}

// updateHover tracks which status control the pointer is over. All-motion
// tracking means this runs for every cell the pointer crosses, so it returns
// the model untouched unless the highlight actually needs to move.
func (m Model) updateHover(x, y int) (tea.Model, tea.Cmd) {
	hovered := m.statusActionAt2D(x, y)
	if hovered == m.hoveredStatus {
		return m, nil
	}
	m.hoveredStatus = hovered
	return m, nil
}

// mouseScroll routes a wheel notch to the open panel, or to reel navigation.
func (m Model) mouseScroll(direction int) (tea.Model, tea.Cmd) {
	if m.scrollPanel(direction) {
		return m, nil
	}
	if cmd := m.navigateToReel(direction); cmd != nil {
		return m, cmd
	}
	return m, nil
}

// mouseClick dispatches a left click at 0-indexed cell (x, y).
func (m Model) mouseClick(x, y int) (tea.Model, tea.Cmd) {
	profile, profileAvailable := m.backend.(backend.ProfileBackend)
	if profileAvailable && profile.IsProfileMode() && !m.profileBusy && !m.profileClosing {
		state := profile.CreatorProfile()
		if m.profileHeaderActionAt(x, y, state) == "back" {
			m.profileBusy = true
			m.profileClosing = true
			m.profileTarget = ""
			m.profileRequest++
			m.stopPlaybackForTransition()
			m.comments.Clear()
			return m, m.exitCreatorProfile(profile, m.profileRequest)
		}
	}
	if follow, ok := m.backend.(backend.CreatorFollowBackend); ok &&
		m.currentReel != nil && !m.backend.IsChatMode() && !m.profileBusy &&
		!m.profileOpening && !m.profileClosing {
		username := m.currentReel.Username
		following, known := follow.CreatorFollowState(username)
		state := backend.CreatorProfileState{Following: following, Known: known}
		if m.profileInlineActionAt(x, y, state) == "follow" {
			m.profileBusy = true
			m.followTarget = username
			m.followRequest++
			return m, m.toggleCreatorFollowFor(follow, username, m.followRequest)
		}
	}
	if m.flags.CreatorProvider && profileAvailable && !profile.IsProfileMode() &&
		m.currentReel != nil && !m.backend.IsChatMode() && !m.profileBusy &&
		!m.profileOpening && !m.profileClosing &&
		m.pointOnCreator(x, y) {
		m.profileOpening = true
		m.profileTarget = m.currentReel.Username
		m.profileRequest++
		return m, tea.Batch(
			m.enterCreatorProfile(profile, m.currentReel.Username, m.profileRequest),
			m.hud.ShowToast("preparing creator feed"),
		)
	}
	if m.pointOnVolumeSlider(x, y) {
		m.volumeDragging = true
		return m.setVolumeFromX(x)
	}
	if action := m.statusActionAt2D(x, y); action != statusActionNone {
		return m.applyStatusAction(action)
	}
	if m.share.IsOpen() {
		return m.clickSharePanel(x, y)
	}
	if m.comments.IsOpen() {
		if handled, model, cmd := m.clickCommentsPanel(x, y); handled {
			return model, cmd
		}
	}
	if m.pointOnProgressBar(x, y) {
		m.scrubbing = true
		return m.scrubTo(x)
	}
	if m.pointInVideo(x, y) {
		m.togglePause()
		return m, nil
	}
	return m, nil
}

func (m Model) profileInlineActionAt(x, y int, state backend.CreatorProfileState) string {
	if m.currentReel == nil {
		return ""
	}
	row := m.videoRow - 1 + player.VideoHeightChars
	nameWidth := max(player.VideoWidthChars-1-pfpGutter, 1)
	usernameBudget := max(nameWidth-lipgloss.Width(profileInlineFollowLabel(state))-1, 1)
	username := "@" + m.currentReel.Username
	if m.currentReel.IsVerified {
		username = truncateByWidth(username, max(usernameBudget-2, 1))
	} else if lipgloss.Width(username) > usernameBudget {
		username = truncateByWidth(username, max(usernameBudget-1, 1)) + "…"
	}
	start := m.videoCol - 1 + pfpGutter + lipgloss.Width(username)
	if m.currentReel.IsVerified {
		start += 2
	}
	start++ // gap before the inline button
	end := start + lipgloss.Width(profileInlineFollowLabel(state))
	if y == row && x >= start && x < end {
		return "follow"
	}
	return ""
}

func (m Model) profileHeaderActionAt(x, y int, state backend.CreatorProfileState) string {
	row := max(m.videoRow-2, 0)
	start := max(m.videoCol-1, 0)
	if y != row || x < start {
		return ""
	}
	header := buildProfileHeaderLayout(state, player.VideoWidthChars-1)
	return header.actionAt(x - start)
}

func (m Model) pointOnCreator(x, y int) bool {
	if m.currentReel == nil {
		return false
	}
	row := m.videoRow - 1 + player.VideoHeightChars
	start := m.videoCol - 1
	end := start + pfpGutter + lipgloss.Width("@"+m.currentReel.Username) + 3
	return y == row && x >= start && x < end
}

func (m Model) pointOnVolumeSlider(x, y int) bool {
	start := m.volumeTrackStart()
	return y == m.volumeSliderRow() && x >= start && x <= start+volumeTrackWidth
}

func (m Model) setVolumeFromX(x int) (tea.Model, tea.Cmd) {
	fraction := float64(x-m.volumeTrackStart()) / float64(volumeTrackWidth)
	fraction = min(max(fraction, 0), 1)
	if m.player.IsMuted() && fraction > 0 {
		m.player.Mute()
	}
	m.player.SetVolume(fraction)
	go m.backend.SetVolume(fraction)
	return m, m.hud.ShowVolume()
}

// clickCommentsPanel handles a click on a comment's like control. Returns
// handled=false when the click wasn't on one, so the caller can carry on.
func (m Model) clickCommentsPanel(x, y int) (bool, tea.Model, tea.Cmd) {
	panelX := max(m.panelContentBaseCol()-1, 0)
	idx := m.comments.HeartAt(y-m.panelContentBaseRow(), x-panelX)
	if idx < 0 {
		return false, m, nil
	}

	comment, ok := m.comments.CommentAt(idx)
	if !ok {
		return true, m, nil
	}

	// Answer immediately, then reconcile with what the page actually did.
	liked := !comment.HasLikedComment
	m.comments.SetCommentLiked(idx, liked)
	return true, m, m.toggleCommentLike(comment, liked)
}

// toggleCommentLike drives the like through the browser and reports failures
// instead of leaving the optimistic state standing.
//
// The comment is matched in the page by author and text, and Instagram's
// comment markup is the part of this app that breaks most often, so a miss is
// surfaced rather than swallowed.
func (m Model) toggleCommentLike(comment backend.Comment, liked bool) tea.Cmd {
	return func() tea.Msg {
		if err := m.backend.SetCommentLiked(comment, liked); err != nil {
			return commentLikeFailedMsg{pk: comment.PK, restore: !liked}
		}
		return nil
	}
}

// pointOnProgressBar reports whether the cell is on the reel's bottom row,
// which is where the player burns the progress bar into the frame.
func (m Model) pointOnProgressBar(x, y int) bool {
	bottom := m.videoRow - 2 + player.VideoHeightChars
	left := m.videoCol - 1
	return y == bottom && x >= left && x < left+player.VideoWidthChars
}

// scrubTo seeks to the position the pointer is over on the progress bar and
// flashes the resulting timecode.
func (m Model) scrubTo(x int) (tea.Model, tea.Cmd) {
	left := m.videoCol - 1
	track := player.VideoWidthChars - 1
	if track < 1 {
		return m, nil
	}

	pos, dur, ok := m.player.SeekToFraction(float64(x-left) / float64(track))
	if !ok {
		return m, nil
	}
	m.player.RedrawVideo()
	return m, m.hud.ShowToast(formatTimecode(pos) + " / " + formatTimecode(dur))
}

// clickSharePanel handles a click inside the open share panel: the send button
// on the header line, or a friend on one of the rows below it.
//
// panelBaseRow is the header's 1-indexed row, so the header's 0-indexed row is
// panelBaseRow()-1 and the first friend's is panelBaseRow().
func (m Model) clickSharePanel(x, y int) (tea.Model, tea.Cmd) {
	if m.shareSending {
		return m, nil
	}
	panelX := max(m.panelBaseCol()-1, 0)

	if y == m.panelBaseRow()-1 {
		if m.share.SendButtonAt(x - panelX) {
			if cmd := m.closeShare(); cmd != nil {
				return m, cmd
			}
		}
		return m, nil
	}

	idx := m.share.FriendAtRow(y - m.panelBaseRow())
	if idx < 0 {
		return m, nil
	}
	m.share.SetCursor(idx)
	m.share.ToggleSelected()
	go m.backend.ToggleShareFriend(idx)
	m.updateImages()
	return m, nil
}

// statusActionAt2D resolves a click to a status-line action, or
// statusActionNone if the click wasn't on the status line.
//
// The controls live in the terminal footer and are centred as one group.
func (m Model) statusActionAt2D(x, y int) statusAction {
	statusY := m.statusLineRow()
	statusX := m.statusLineStart()
	if (y != statusY && y != m.statusLabelRow()) || x < statusX {
		return statusActionNone
	}
	return m.statusActionAt(x - statusX)
}

// pointInVideo reports whether the 0-indexed cell is inside the area the reel
// is drawn into.
func (m Model) pointInVideo(x, y int) bool {
	top := m.videoRow - 1
	left := m.videoCol - 1
	return y >= top && y < top+player.VideoHeightChars &&
		x >= left && x < left+player.VideoWidthChars
}

// applyStatusAction runs the action behind a clicked status icon, reusing the
// same guarded helpers the keybinds use.
func (m Model) applyStatusAction(action statusAction) (tea.Model, tea.Cmd) {
	switch action {
	case statusActionLike:
		m.toggleLike()
	case statusActionRepost:
		return m, m.toggleRepost()
	case statusActionSave:
		m.toggleSave()
	case statusActionPause:
		m.togglePause()
	case statusActionMute:
		m.toggleMute()
	case statusActionComments:
		if m.comments.IsOpen() {
			m.closeComments()
		} else {
			m.openComments()
		}
	case statusActionShare:
		if m.share.IsOpen() {
			if cmd := m.closeShare(); cmd != nil {
				return m, cmd
			}
		} else {
			m.openShare()
		}
	}
	return m, nil
}
