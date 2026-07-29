package tui

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/ademiru/TermiReels/tui/colors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) viewBrowsing() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	videoWidthChars := player.VideoWidthChars - 1
	videoHeightChars := player.VideoHeightChars
	videoX := max(m.videoCol-1, 0)
	videoY := max(m.videoRow-1, 0)

	// The terminal view is a sparse canvas. The reel itself is drawn by the
	// Kitty renderer, while text blocks are placed around it. Building by
	// coordinates lets a side panel and the reel share rows without fragile
	// newline arithmetic.
	screen := make([]string, m.height)
	place := func(row, col int, block string) {
		if block == "" || row >= len(screen) {
			return
		}
		for i, line := range strings.Split(strings.TrimSuffix(block, "\n"), "\n") {
			y := row + i
			if y < 0 || y >= len(screen) {
				continue
			}
			screen[y] = strings.Repeat(" ", max(col, 0)) + line
		}
	}

	// HUD remains above the reel, but controls now live in a stable footer.
	place(0, 0, m.viewHUD(videoWidthChars, videoY, strings.Repeat(" ", videoX)))
	if profile, ok := m.backend.(backend.ProfileBackend); ok && profile.IsProfileMode() {
		header := buildProfileHeaderLayout(profile.CreatorProfile(), videoWidthChars)
		place(max(videoY-1, 0), videoX, header.render())
	}
	var statusContent string
	for _, seg := range m.statusSegments() {
		statusContent += seg.text
	}
	if m.status == statusLoading || m.comments.loading || m.backend.IsSyncing() || m.profileOpening {
		statusContent += "  " + m.spinner.View()
	}
	statusX := m.statusLineStart()

	if m.currentReel != nil {
		nameWidth := max(videoWidthChars-pfpGutter, 1)
		var profileState *backend.CreatorProfileState
		if follow, ok := m.backend.(backend.CreatorFollowBackend); ok && !m.backend.IsChatMode() {
			followUsername := m.currentReel.Username
			total := m.currentReel.Total
			if profile, profileOK := m.backend.(backend.ProfileBackend); profileOK && profile.IsProfileMode() {
				state := profile.CreatorProfile()
				followUsername = state.Username
				total = state.Total
			}
			following, known := follow.CreatorFollowState(followUsername)
			state := backend.CreatorProfileState{
				Username: followUsername, Following: following, Known: known, Total: total,
			}
			profileState = &state
		}
		usernameBudget := nameWidth
		if profileState != nil {
			usernameBudget = max(
				nameWidth-lipgloss.Width(profileInlineFollowLabel(*profileState))-1,
				1,
			)
		}
		username := "@" + m.currentReel.Username
		if m.currentReel.IsVerified {
			username = truncateByWidth(username, max(usernameBudget-2, 1))
		} else if lipgloss.Width(username) > usernameBudget {
			username = truncateByWidth(username, max(usernameBudget-1, 1)) + "…"
		}
		userLine := pink400.Bold(true).Render(username)
		if m.currentReel.IsVerified {
			userLine += " " + blue500.Render("✓")
		}
		if profileState != nil {
			userLine += " " + renderProfileInlineFollow(*profileState)
			if !m.compactViewport() {
				counter := fmt.Sprintf(" %02d / %02d ", m.currentReel.Index, max(profileState.Total, m.currentReel.Total))
				counterWidth := lipgloss.Width(counter)
				if lipgloss.Width(userLine)+counterWidth+2 <= nameWidth {
					userLine += "  " + lipgloss.NewStyle().
						Foreground(colors.Purple100Color).
						Background(colors.Purple900Color).
						Bold(true).
						Render(counter)
				}
			}
		}
		infoX := videoX + pfpGutter
		infoY := videoY + videoHeightChars
		place(infoY, infoX, userLine)

		if m.currentReel.Music != nil && !m.compactViewport() {
			explicit := ""
			if m.currentReel.Music.IsExplicit {
				explicit = " [E]"
			}
			title := cleanMetadataText(m.currentReel.Music.Title)
			artist := cleanMetadataText(m.currentReel.Music.Artist)
			musicText := strings.Trim(strings.Join([]string{title, artist}, " · "), " ·")
			if musicText == "" {
				musicText = "Original audio"
			}
			musicText += explicit
			maxMusicWidth := max(videoWidthChars-pfpGutter-2, 1)
			musicWindow := marquee(musicText, maxMusicWidth, m.marqueeOffset)
			musicStyle := lipgloss.NewStyle().Italic(true).Bold(true)
			musicLine := gradientText("♪ ", brandRamp, musicStyle) +
				gradientText(musicWindow, brandRamp, musicStyle)
			place(infoY+1, infoX, musicLine)
		}

		panelRow := m.panelBaseRow() - 1
		panelCol := m.panelBaseCol() - 1
		panelWidth := m.panelWidth()
		panelLines := m.panelLines()

		if m.comments.IsOpen() {
			if m.commentsOnSide() {
				content := m.comments.View(m.panelContentWidth(), m.panelContentLines(), "")
				card := lipgloss.NewStyle().
					Width(m.panelContentWidth()).
					Height(m.panelContentLines()).
					Border(lipgloss.RoundedBorder()).
					BorderForeground(colors.Purple400Color).
					Padding(0).
					Render(content)
				place(panelRow, panelCol, card)
			} else {
				place(panelRow, panelCol, m.comments.View(panelWidth, panelLines, ""))
			}
		} else if m.share.IsOpen() {
			place(panelRow, panelCol, m.share.View(panelWidth, panelLines, ""))
		} else if m.help.IsOpen() {
			place(panelRow, panelCol, m.help.View(panelWidth, panelLines, ""))
		} else if m.chats.IsOpen() {
			place(panelRow, panelCol, m.chats.View(panelWidth, panelLines, ""))
		} else if m.react.IsOpen() {
			place(panelRow, panelCol, m.react.View(panelWidth, panelLines, ""))
		} else if !m.compactViewport() {
			// Caption prose and hashtags are separate visual layers. Treating
			// one styled multiline string as a block allowed ANSI state and
			// wrapping to push hashtag rows far to the right.
			prose, tags := splitCaption(m.currentReel.Caption)
			detailWidth := max(videoWidthChars-pfpGutter, 1)
			detailY := infoY + 2
			available := max(m.statusLineRow()-detailY-1, 0)

			if prose != "" && available > 0 {
				// A fixed-width marquee reveals the complete caption without
				// ellipses and, unlike changing wrapped lines, cannot make the
				// surrounding layout jump while it moves.
				line := marquee(prose, max(detailWidth-2, 1), m.marqueeOffset)
				accent := purple500.Render("│ ")
				place(detailY, infoX, accent+renderWithMentions(line, gray200))
				detailY++
				available--
			}

			tagLines := limitedWrappedLines(tags, detailWidth, min(2, max(available, 0)))
			for i, line := range tagLines {
				place(detailY+i, infoX, renderTagLine(line))
			}

		}
	} else {
		place(videoY+videoHeightChars, videoX, m.spinner.View())
	}

	// Footer wins if a user-selected fixed reel is taller than the available
	// content area. Controls must remain reachable even in that degraded
	// layout.
	place(m.statusLineRow(), statusX, gray300.Render(statusContent))
	if m.statusLabelRow() >= 0 {
		place(m.statusLabelRow(), statusX, m.statusLabels())
	}
	place(m.volumeSliderRow(), m.volumeSliderStart(), m.volumeSlider())

	// Empty rows can be optimized away by the renderer. Put one harmless cell
	// on every row so stale content from the previous frame — especially a
	// footer whose row changed — is actively overwritten.
	for i := range screen {
		if screen[i] == "" {
			screen[i] = " "
		}
	}
	return strings.Join(screen, "\n")
}

// splitCaption separates prose from hashtags while retaining their original
// order inside each layer. Instagram captions commonly end in a dense tag
// cloud; giving it its own rows makes the actual sentence readable.
func splitCaption(caption string) (prose, tags string) {
	fields := strings.Fields(strings.ReplaceAll(caption, "\n", " "))
	var proseFields, tagFields []string
	for _, field := range fields {
		if strings.HasPrefix(field, "#") && len([]rune(field)) > 1 {
			tagFields = append(tagFields, field)
		} else {
			proseFields = append(proseFields, field)
		}
	}
	return strings.Join(proseFields, " "), strings.Join(tagFields, " ")
}

// limitedWrappedLines wraps text to a strict line budget and marks truncation
// on the final line rather than silently dropping the rest.
func limitedWrappedLines(text string, width, limit int) []string {
	if text == "" || width < 1 || limit < 1 {
		return nil
	}
	lines := wrapByWidth(text, width)
	if len(lines) <= limit {
		return lines
	}
	lines = lines[:limit]
	last := strings.TrimSpace(lines[limit-1])
	lines[limit-1] = truncateByWidth(last, max(width-1, 1)) + "…"
	return lines
}

func renderTagLine(line string) string {
	var b strings.Builder
	for i, field := range strings.Fields(line) {
		if i > 0 {
			b.WriteString(" ")
		}
		if strings.HasPrefix(field, "#") {
			b.WriteString(pink400.Render(field))
		} else {
			b.WriteString(gray400.Render(field))
		}
	}
	return b.String()
}

// clampToHeight drops any lines past the bottom of the terminal.
//
// A view taller than the screen makes the terminal scroll, which shifts every
// row up and quietly invalidates the row arithmetic the mouse hit-test relies
// on. chromeRows sizes the reel to avoid this in fit mode; the clamp is what
// guarantees it for a hand-set reel_width too.
func clampToHeight(view string, height int) string {
	if height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) <= height {
		return view
	}
	return strings.Join(lines[:height], "\n")
}

// displayKeys formats a keybind slice for the navbar
// ["[", "-"] -> "[, -"
func displayKeys(keys []string) string {
	display := make([]string, len(keys))
	for i, k := range keys {
		if v, ok := backend.KeyToConf[k]; ok {
			display[i] = v
		} else {
			display[i] = k
		}
	}
	return strings.Join(display, ", ")
}

// navHint renders one navbar entry with the key picked out from its label, so
// the bindings are scannable instead of a wall of one grey colour.
func navHint(keys []string, label string) string {
	return yellow300.Render(displayKeys(keys)) + gray600.Render(": "+label)
}

// formatLikeCount formats like count with K/M suffixes
func formatLikeCount(count int) string {
	if count >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(count)/1000000)
	}
	if count >= 1000 {
		return fmt.Sprintf("%.1fK", float64(count)/1000)
	}
	return fmt.Sprintf("%d", count)
}

// Browsing state update & helpers

func (m Model) updateBrowsing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {

	config := backend.GetSettings()
	key := msg.String()
	follow, followAvailable := m.backend.(backend.CreatorFollowBackend)
	profile, profileAvailable := m.backend.(backend.ProfileBackend)

	switch {
	case profileAvailable && (profile.IsProfileMode() || m.profileOpening) &&
		slices.Contains(config.KeysProfileBack, key):
		return m, m.exitCreatorProfile(profile)

	case followAvailable && !m.profileBusy && !m.profileOpening && !m.backend.IsChatMode() &&
		slices.Contains(config.KeysProfileFollow, key):
		if m.currentReel == nil || m.backend.IsSyncing() {
			return m, nil
		}
		username := m.currentReel.Username
		if profileAvailable && profile.IsProfileMode() {
			username = profile.CreatorProfile().Username
		}
		m.profileBusy = true
		return m, m.toggleCreatorFollowFor(follow, username)

	case m.flags.CreatorProvider && profileAvailable && !profile.IsProfileMode() &&
		!m.profileBusy && !m.profileOpening && !m.backend.IsChatMode() &&
		slices.Contains(config.KeysProfileOpen, key):
		if m.currentReel == nil || m.backend.IsSyncing() {
			return m, nil
		}
		m.profileOpening = true
		return m, tea.Batch(
			m.enterCreatorProfile(profile, m.currentReel.Username),
			m.hud.ShowToast("verifying creator reels"),
		)

	// Chats panel select takes priority over other keys
	case m.chats.IsOpen() && slices.Contains(config.KeysSelect, key):
		chat := m.chats.CursorChat()
		if chat == nil {
			return m, nil
		}
		threadKey, title := chat.ThreadKey, chat.Title
		m.chats.Close()
		m.closePanelLayout()
		m.player.Stop()
		m.status = statusLoading
		m.comments.Clear()
		if err := m.backend.EnterChatMode(threadKey); err != nil {
			m.status = statusReelError
			return m, nil
		}
		m.player.SetBorder(colors.Blue300Color)
		return m, tea.Batch(m.loadCurrentReel, m.hud.ShowChatBanner(title, config.KeysReactOpen))

	// React select sends the highlighted reaction to the current reel
	case m.react.IsOpen() && slices.Contains(config.KeysSelect, key):
		emoji := m.react.CursorEmoji()
		if emoji == "" {
			return m, nil
		}
		m.react.Close()
		m.closePanelLayout()
		if m.currentReel == nil {
			return m, nil
		}
		return m, m.reactToCurrent(emoji, m.currentReel.Index)

	// Share select takes priority over other keys when share panel is open
	case m.share.IsOpen() && slices.Contains(config.KeysSelect, key):
		if m.shareSending {
			return m, nil
		}
		m.share.ToggleSelected()
		go m.backend.ToggleShareFriend(m.share.CursorIndex())
		return m, nil

	// Comments select loads (or collapses) the replies of the comment under the cursor
	case m.comments.IsOpen() && slices.Contains(config.KeysSelect, key):
		c, ok := m.comments.CursorComment()
		if !ok || c.ParentCommentID != "" || c.ChildCommentCount == 0 {
			return m, nil // not a top-level comment with replies
		}
		if m.comments.RepliesLoaded(c.PK) {
			go m.backend.CollapseChildComments(c.PK)
		} else if !m.comments.loading {
			m.comments.SetLoading(true)
			go m.backend.FetchChildComments(c.PK)
		}
		return m, nil
	case slices.Contains(config.KeysNext, key):
		if m.scrollPanel(1) {
			return m, nil
		}
		if cmd := m.navigateToReel(1); cmd != nil {
			return m, cmd
		}

	case slices.Contains(config.KeysPrevious, key):
		if m.scrollPanel(-1) {
			return m, nil
		}
		if cmd := m.navigateToReel(-1); cmd != nil {
			return m, cmd
		}

	case slices.Contains(config.KeysMute, key):
		m.toggleMute()

	case slices.Contains(config.KeysPause, key):
		m.togglePause()

	case slices.Contains(config.KeysLike, key):
		m.toggleLike()

	case slices.Contains(config.KeysRepost, key):
		return m, m.toggleRepost()

	case slices.Contains(config.KeysSave, key):
		m.toggleSave()

	case m.comments.IsOpen() && slices.Contains(config.KeysCommentsClose, key):
		m.closeComments()

	case !m.comments.IsOpen() && slices.Contains(config.KeysCommentsOpen, key):
		m.openComments()

	case m.share.IsOpen() && slices.Contains(config.KeysShareClose, key):
		if cmd := m.closeShare(); cmd != nil {
			return m, cmd
		}

	case !m.share.IsOpen() && slices.Contains(config.KeysShareOpen, key):
		m.openShare()

	case m.help.IsOpen() && slices.Contains(config.KeysHelpClose, key):
		m.help.Close()
		m.closePanelLayout()

	case !m.help.IsOpen() && slices.Contains(config.KeysHelpOpen, key):
		if !m.panelOpen() {
			m.help.Open()
			m.relayout()
			m.player.RedrawVideo()
		}

	case m.react.IsOpen() && slices.Contains(config.KeysReactClose, key):
		m.react.Close()
		m.closePanelLayout()

	case !m.react.IsOpen() && slices.Contains(config.KeysReactOpen, key):
		if m.backend.IsChatMode() && !m.panelOpen() && !m.backend.IsSyncing() {
			m.react.Open()
			m.relayout()
			m.player.RedrawVideo()
		}
	case m.chats.IsOpen() && slices.Contains(config.KeysChatsClose, key):
		// if selecting a friend's dm to visit (chat panel), close key will close
		// that panel first. else, fall through to the next case
		m.chats.Close()
		m.closePanelLayout()

	case !m.panelOpen() && slices.Contains(config.KeysChatsClose, key) && m.backend.IsChatMode():
		// in chat mode with no panel open, close key exits back to the feed.
		// the react panel must be closed with its own close key first.
		go m.backend.ExitChatMode()
		m.player.SetBorder(nil)
		return m, nil

	case !m.chats.IsOpen() && slices.Contains(config.KeysChatsOpen, key):
		// Gate until the background DM collection + prefetch has finished.
		if !m.panelOpen() && m.dmReelsReady {
			chats := m.backend.GetDMChats()
			m.chats.Open(chats)
			m.relayout()
			m.player.RedrawVideo()
		}

	case slices.Contains(config.KeysNavbar, key):
		showNavbar := m.backend.ToggleNavbar()
		m.showNavbar = showNavbar

	case slices.Contains(config.KeysReelSizeInc, key):
		m.nudgeReelSize(config.ReelSizeStep)
		m.player.RedrawVideo()
		m.updateCommentGifs()

	case slices.Contains(config.KeysReelSizeDec, key):
		m.nudgeReelSize(-config.ReelSizeStep)
		m.player.RedrawVideo()
		m.updateCommentGifs()

	case slices.Contains(config.KeysVolUp, key):
		vol := min(m.player.Volume()+0.1, 1.0)
		m.player.SetVolume(vol)
		go m.backend.SetVolume(vol)
		return m, m.hud.ShowVolume()

	case slices.Contains(config.KeysVolDown, key):
		vol := max(m.player.Volume()-0.1, 0.0)
		m.player.SetVolume(vol)
		go m.backend.SetVolume(vol)
		return m, m.hud.ShowVolume()

	case slices.Contains(config.KeysCopyLink, key):
		if m.currentReel != nil && m.currentReel.Code != "" {
			copyToClipboard("https://www.instagram.com/reel/" + m.currentReel.Code)
			m.shareConfirmed = true
			return m, m.queueShareReset()
		}

	case slices.Contains(config.KeysSeekBackward, key):
		return m, m.seek(-5)

	case slices.Contains(config.KeysSeekForward, key):
		return m, m.seek(5)
	}

	return m, nil
}

// Reel actions
//
// These are the bodies the keybind switch above used to inline. They live on
// their own so the mouse handler can invoke the same action — with the same
// guards — instead of re-deriving when a toggle is legal.

// sentToast is the confirmation shown after a reel goes out over DM.
func sentToast(friends int) string {
	switch {
	case friends <= 0:
		return "SENT"
	case friends == 1:
		return "SENT TO 1 FRIEND"
	default:
		return fmt.Sprintf("SENT TO %d FRIENDS", friends)
	}
}

func (m Model) enterCreatorProfile(profile backend.ProfileBackend, username string) tea.Cmd {
	return func() tea.Msg {
		info, err := profile.EnterCreatorProfile(username)
		if err != nil {
			current, _ := m.backend.GetCurrent()
			return profileActionFailedMsg{action: "opening profile", err: err, info: current}
		}
		return profileEnteredMsg{
			info: info, generation: profile.CreatorProfile().Generation,
		}
	}
}

func (m Model) exitCreatorProfile(profile backend.ProfileBackend) tea.Cmd {
	return func() tea.Msg {
		info, err := profile.ExitCreatorProfile()
		if err != nil {
			current, _ := m.backend.GetCurrent()
			return profileActionFailedMsg{action: "returning to feed", err: err, info: current}
		}
		return profileExitedMsg{info: info}
	}
}

func (m Model) toggleCreatorFollow(profile backend.ProfileBackend) tea.Cmd {
	return func() tea.Msg {
		following, err := profile.ToggleCreatorFollow()
		return profileFollowedMsg{following: following, err: err}
	}
}

func (m Model) toggleCreatorFollowFor(follow backend.CreatorFollowBackend, username string) tea.Cmd {
	return func() tea.Msg {
		following, err := follow.ToggleCreatorFollowFor(username)
		return profileFollowedMsg{following: following, err: err}
	}
}

// seek jumps playback by delta seconds and flashes where it landed. Without
// this the only feedback is the progress bar burned into the frame, which is
// too subtle to read while scrubbing.
func (m *Model) seek(delta float64) tea.Cmd {
	pos, dur, ok := m.player.Skip(delta)
	if !ok {
		return nil
	}
	return m.hud.ShowToast(formatTimecode(pos) + " / " + formatTimecode(dur))
}

// formatTimecode renders seconds as m:ss.
func formatTimecode(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int(seconds + 0.5)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func (m *Model) toggleMute() {
	if m.currentReel != nil {
		m.player.Mute()
	}
}

func (m *Model) togglePause() {
	m.player.Pause()
	if m.player.IsPaused() {
		m.status = statusPaused
	} else {
		m.status = statusNone
	}
}

func (m *Model) toggleLike() {
	if m.panelOpen() || m.currentReel == nil || m.backend.IsSyncing() {
		return
	}
	m.currentReel.Liked = !m.currentReel.Liked
	go m.backend.ToggleLike()
}

// toggleRepost reposts or unreposts, and returns the command that turns the
// icon. The spin is the whole feedback now that the count is gone.
func (m *Model) toggleRepost() tea.Cmd {
	if m.panelOpen() || m.currentReel == nil || m.backend.IsSyncing() {
		return nil
	}
	m.currentReel.Reposted = !m.currentReel.Reposted
	go m.backend.ToggleRepost()

	m.repostSpin = repostSpinFrames
	m.repostGen++

	// The spinning icon alone was too easy to miss mid-video, so the action is
	// also announced above the reel.
	toast := "repost removed"
	if m.currentReel.Reposted {
		toast = "REPOSTED"
	}
	return tea.Batch(m.repostSpinTick(), m.hud.ShowToast(toast))
}

func (m Model) repostSpinTick() tea.Cmd {
	gen := m.repostGen
	return tea.Tick(repostSpinInterval, func(t time.Time) tea.Msg {
		return repostSpinMsg{gen: gen}
	})
}

func (m *Model) toggleSave() {
	if m.panelOpen() || m.currentReel == nil || m.backend.IsSyncing() {
		return
	}
	m.currentReel.Saved = !m.currentReel.Saved
	go m.backend.ToggleSave()
}

func (m *Model) openComments() {
	if m.backend.IsSyncing() || m.currentReel == nil || m.currentReel.CommentsDisabled || m.panelOpen() {
		return
	}
	m.comments.Open(m.currentReel.PK)
	m.relayout()

	if m.currentReel.Comments != nil {
		m.comments.SetComments(m.currentReel.PK, m.currentReel.Comments)
		m.updateCommentGifs()
	}

	go m.backend.OpenComments()
	m.player.RedrawVideo()
}

func (m *Model) closeComments() {
	if m.backend.IsSyncing() {
		return
	}
	m.comments.Close()
	m.closePanelLayout()
	go m.backend.CloseComments()
}

func (m *Model) openShare() {
	if m.backend.IsSyncing() || m.currentReel == nil || !m.currentReel.CanViewerReshare || m.panelOpen() {
		return
	}
	m.share.Open()
	m.relayout()
	go m.backend.OpenSharePanel()
	m.player.RedrawVideo()
}

// closeShare sends to the selected friends and closes the panel. Returns nil
// if a send is already in flight.
func (m *Model) closeShare() tea.Cmd {
	if m.shareSending {
		return nil
	}
	m.shareSending = true
	// The panel is cleared as it closes, so the count has to be taken now for
	// the confirmation to be able to name it.
	m.shareCount = m.share.SelectedCount()
	return m.sendShare()
}

func (m *Model) startPlayback(index int) tea.Cmd {
	info, infoErr := m.backend.GetReel(index)
	return func() tea.Msg {
		if infoErr != nil {
			return videoErrorMsg{infoErr}
		}
		var videoPath, pfpPath string
		var floatingFiles []backend.FloatingPfpFile
		var err error
		if pinned, ok := m.backend.(backend.PinnedDownloader); ok {
			videoPath, pfpPath, floatingFiles, err = pinned.DownloadReelContext(
				context.Background(), index, info.PK,
			)
		} else {
			videoPath, pfpPath, floatingFiles, err = m.backend.Download(index)
		}
		if err != nil {
			return videoErrorMsg{err}
		}
		var pfp *player.Img
		if pfpPath != "" {
			if loaded, err := player.LoadPFP(pfpPath); err == nil {
				loaded.ResizeToCells(reelPfpCells)
				pfp = loaded
			}
		}

		// friend heart/like/repost floating pfp
		floating := make([]floatingItem, 0, len(floatingFiles))
		for _, f := range floatingFiles {
			if f.Path == "" {
				continue
			}
			loaded, err := player.LoadPFP(f.Path)
			if err != nil {
				continue
			}
			loaded.ResizeToCells(3)
			floating = append(floating, floatingItem{pfp: loaded, badge: iconForType(f.Type)})
		}

		// chat mode sender + reactions
		chat := m.chatFloating(index)

		if err := m.player.Play(videoPath); err != nil {
			return videoErrorMsg{err}
		}

		return videoReadyMsg{index: index, pfp: pfp, contextFloating: floating, chatFloating: chat}
	}
}

func (m Model) prefetch(ctx context.Context, index int) {
	downloader, cancellable := m.backend.(backend.ContextDownloader)
	download := func(i int) {
		if cancellable {
			_, _, _, _ = downloader.DownloadContext(ctx, i)
			return
		}
		_, _, _, _ = m.backend.Download(i)
	}

	toDownload1 := index + 1
	toDownload2 := index + 2

	if toDownload1 <= m.backend.GetTotal() {
		download(toDownload1)
	}
	if toDownload2 <= m.backend.GetTotal() {
		download(toDownload2)
	}
}

func (m Model) musicTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg {
		return musicTickMsg{}
	})
}

func (m Model) queueShareReset() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return shareResetMsg{}
	})
}

// reactToCurrent sends the reaction toggle, then reports back so the
// reactor's own pfp can be added, updated, or removed live.
func (m Model) reactToCurrent(emoji string, index int) tea.Cmd {
	return func() tea.Msg {
		m.backend.ReactToCurrent(emoji)
		return selfReactedMsg{index: index}
	}
}

func (m Model) sendShare() tea.Cmd {
	return func() tea.Msg {
		sent, err := m.backend.SendShare()
		if err != nil {
			return shareFailedMsg{}
		}
		if !sent {
			return shareClosedMsg{}
		}
		return shareSentMsg{}
	}
}

// panelOpen returns true if any overlay panel (comments, share, help, chats, react) is open.
func (m Model) panelOpen() bool {
	return m.comments.IsOpen() || m.share.IsOpen() || m.help.IsOpen() || m.chats.IsOpen() || m.react.IsOpen()
}

// scrollPanel dispatches scroll/cursor movement to the active panel.
// Returns true if a panel consumed the input.
func (m *Model) scrollPanel(direction int) bool {
	if m.help.IsOpen() {
		m.help.Scroll(direction)
		return true
	}
	if m.share.IsOpen() {
		if m.shareSending {
			return true
		}
		m.share.MoveCursor(direction)
		m.updateImages()
		return true
	}
	if m.chats.IsOpen() {
		m.chats.MoveCursor(direction)
		return true
	}
	if m.react.IsOpen() {
		m.react.MoveCursor(direction)
		return true
	}
	if m.comments.IsOpen() {
		m.comments.MoveCursor(direction)
		m.updateCommentGifs()
		if direction > 0 && m.currentReel != nil && m.comments.ShouldFetchMore() &&
			!m.comments.loading && len(m.currentReel.Comments) < m.currentReel.CommentCount {
			m.comments.SetLoading(true)
			go m.backend.FetchMoreComments()
		}
		return true
	}
	return false
}

// discoverNextReel advances Instagram's lazy feed and waits for the newly
// captured reel. Failure is retryable; it must never turn the current captured
// total into a permanent end-of-feed.
func (m Model) discoverNextReel(afterIndex int) tea.Cmd {
	return func() tea.Msg {
		info, err := m.backend.DiscoverNextReel(afterIndex)
		if err != nil {
			return feedMoreFailedMsg{}
		}
		return reelLoadedMsg{info: info}
	}
}

// navigateToReel moves to a reel at currentIndex+direction. At the captured
// tail of the main feed it asks Instagram for another lazy page instead of
// treating the cache boundary as the end.
func (m *Model) navigateToReel(direction int) tea.Cmd {
	if m.currentReel == nil || m.status == statusLoading || m.profileBusy {
		return nil
	}
	if m.profileOpening {
		if profile, ok := m.backend.(backend.ProfileBackend); ok {
			go func() { _, _ = profile.ExitCreatorProfile() }()
		}
		m.profileOpening = false
	}
	index := m.currentReel.Index + direction
	if m.backend.IsChatMode() && direction > 0 && index > m.backend.GetTotal() {
		m.player.Stop()
		m.status = statusLoading
		m.comments.Clear()
		go m.backend.ExitChatMode()
		m.player.SetBorder(nil)
		return nil
	}
	if index < 1 {
		return nil
	}
	if m.prefetchCancel != nil {
		m.prefetchCancel()
		m.prefetchCancel = nil
	}
	if direction > 0 && index > m.backend.GetTotal() {
		m.player.Stop()
		m.status = statusLoading
		m.comments.Clear()
		return m.discoverNextReel(m.currentReel.Index)
	}
	if index > m.backend.GetTotal() {
		return nil
	}
	m.player.Stop()
	m.status = statusLoading
	m.comments.Clear()
	if profile, ok := m.backend.(backend.ProfileBackend); ok && profile.IsProfileMode() {
		// Resolved/profile-prefetched entries can begin playback immediately;
		// browser DOM alignment continues asynchronously and guards mutations
		// through IsSyncing. Only a cache miss needs the blocking resolver path.
		if info, err := m.backend.GetReel(index); err == nil {
			m.currentReel = info
			go func() { _ = m.backend.SyncTo(index) }()
			return m.startPlayback(index)
		}
		m.profileBusy = true
		return func() tea.Msg {
			if err := m.backend.SyncTo(index); err != nil {
				return reelErrorMsg{err}
			}
			info, err := m.backend.GetCurrent()
			if err != nil {
				return reelErrorMsg{err}
			}
			return reelLoadedMsg{info: info}
		}
	}
	if info, err := m.backend.GetReel(index); err == nil {
		m.currentReel = info
	}
	go m.backend.SyncTo(index)
	return m.startPlayback(index)
}

// closePanelLayout restores the reel size and video position after a panel is closed.
func (m *Model) closePanelLayout() {
	m.player.ClearGifs()
	m.relayout()
	m.player.RedrawVideo()
}

// updateCommentGifs recomputes visible GIF slots and passes them to the player.
func (m Model) updateCommentGifs() {
	if !m.comments.IsOpen() {
		m.player.ClearGifs()
		return
	}

	slots := m.comments.VisibleGifSlots(
		m.panelContentWidth(),
		m.panelContentLines(),
		m.panelContentBaseRow(),
		m.panelContentBaseCol(),
	)
	if len(slots) > 0 {
		m.player.SetVisibleGifs(slots)
	} else {
		m.player.ClearGifs()
	}
}

// updateVideoPosition computes the centered video position and stores it on the model,
// then forwards it to the player.
func (m *Model) updateVideoPosition() {
	_, col := player.ComputeVideoCenterPosition(m.videoWidthPx, m.videoHeightPx)
	row := chromeRowsAbove + 1
	if m.compactViewport() {
		row = 2
	}
	if m.commentsOnSide() {
		groupWidth := player.VideoWidthChars + sidePanelGap + m.panelWidth()
		col = max((m.width-groupWidth)/2+1, 1)
	} else if m.panelOpen() {
		row = 5
	}

	m.videoRow = row
	m.videoCol = col
	// Adjust for non-9:16 videos that don't fill the bounding box.
	rowOff, colOff := m.player.VideoCenterOffset()
	m.player.SetVideoPosition(row+rowOff, col+colOff)
}

func (m *Model) updateImages() {
	var slots []player.ImageSlot

	if m.reelPFP != nil {
		row := max(m.videoRow+player.VideoHeightChars, 1)
		slots = append(slots, player.ImageSlot{Img: m.reelPFP, Row: row, Col: m.videoCol})
		slots = append(slots, m.floatingPfpSlots()...)
	}

	if m.share.IsOpen() {
		slots = append(slots, m.share.VisiblePfpSlots(m.panelWidth(), m.panelLines(), m.panelBaseRow(), m.panelBaseCol())...)
	}

	if len(slots) > 0 {
		m.player.SetVisibleImages(slots)
	} else {
		m.player.ClearImages()
	}
}

// floatingPfpSlots scatters floating pfps (friend reposts/likes, the DM sender,
// and chat-mode reactors) across the bottom-right quarter of the reel, each with
// its badge overlaid. Positions are picked with Mitchell's best-candidate
// sampling so the scatter looks random but spreads out instead of stacking.
// Seeded by the reel's PK so the layout is stable across resizes, panel
// toggles, and re-navigation.
func (m *Model) floatingPfpSlots() []player.ImageSlot {
	if len(m.floating) == 0 || m.currentReel == nil {
		return nil
	}

	const pfpCellH = 2
	const pfpCellW = 4

	quadW := player.VideoWidthChars / 4
	quadH := player.VideoHeightChars / 4
	quadRow := m.videoRow + player.VideoHeightChars - quadH
	quadCol := m.videoCol + player.VideoWidthChars - quadW

	maxRowOff := max(quadH-pfpCellH, 0)
	maxColOff := max(quadW-pfpCellW, 0)

	h := fnv.New64a()
	h.Write([]byte(m.currentReel.PK))
	seed := h.Sum64()
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))

	// For each pfp, draw several random positions and keep the one farthest
	// from the pfps already placed.
	const candidates = 10
	type offset struct{ dr, dc int }
	placed := make([]offset, 0, len(m.floating))

	slots := make([]player.ImageSlot, 0, len(m.floating)*2)
	for _, item := range m.floating {
		if item.pfp == nil {
			continue
		}

		tries := candidates
		if len(placed) == 0 {
			tries = 1 // nothing to spread away from yet
		}
		var best offset
		bestScore := -1
		for range tries {
			var cand offset
			if maxRowOff > 0 {
				cand.dr = rng.IntN(maxRowOff + 1)
			}
			if maxColOff > 0 {
				cand.dc = rng.IntN(maxColOff + 1)
			}
			// Chebyshev distance to the nearest placed pfp, in footprint
			// units cross-multiplied to stay integer: pfpCellW*pfpCellH or
			// more means the footprints don't overlap at all.
			nearest := math.MaxInt
			for _, p := range placed {
				dRow := max(cand.dr-p.dr, p.dr-cand.dr) * pfpCellW
				dCol := max(cand.dc-p.dc, p.dc-cand.dc) * pfpCellH
				nearest = min(nearest, max(dRow, dCol))
			}
			if nearest > bestScore {
				best, bestScore = cand, nearest
			}
		}
		placed = append(placed, best)

		row := quadRow + best.dr
		col := quadCol + best.dc
		slots = append(slots, player.ImageSlot{Img: item.pfp, Row: row, Col: col})

		if item.badge != nil {
			item.badge.ResizeToCells(2)
			slots = append(slots, player.ImageSlot{
				Img: item.badge,
				Row: row + 2,
				Col: col + 3,
			})
		}
	}
	return slots
}

// iconForType resolves a floating-context type to its fixed badge icon. Reactor
// items don't go through here — their badge is an EmojiBadge (see reactionItems).
func iconForType(floatingType string) *player.Img {
	switch floatingType {
	case backend.FloatingTypeReposted:
		return player.RepostIcon()
	case backend.FloatingTypeLiked:
		return player.HeartIcon()
	case backend.FloatingTypeSent:
		return player.SentIcon()
	}
	return nil
}

// chatFloating builds a chat-mode reel's whole floating set at 1-based index:
//  1. the friend who sent the reel, with the Sent badge
//  2. everyone who reacted, each with their emoji badge
//
// Returns nil outside chat mode. Reactors are sorted by name so slot positions stay stable on refresh.
func (m *Model) chatFloating(index int) []floatingItem {
	var items []floatingItem

	if sender, ok := m.backend.ChatSender(index); ok && sender.ImgPath != "" {
		if pfp, err := player.LoadPFP(sender.ImgPath); err == nil {
			pfp.ResizeToCells(3)
			items = append(items, floatingItem{pfp: pfp, badge: player.SentIcon()})
		}
	}

	// Clone before sorting since ChatReactions returns the backend's own slice
	reactors, _ := m.backend.ChatReactions(index)
	reactors = slices.Clone(reactors)
	slices.SortFunc(reactors, func(a, b backend.User) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, r := range reactors {
		if r.ImgPath == "" {
			continue
		}
		pfp, err := player.LoadPFP(r.ImgPath)
		if err != nil {
			continue
		}
		pfp.ResizeToCells(3)
		items = append(items, floatingItem{pfp: pfp, badge: player.EmojiBadge(r.Reaction)})
	}

	return items
}
