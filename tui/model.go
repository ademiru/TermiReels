package tui

import (
	"context"
	"io"
	"slices"
	"time"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/ademiru/TermiReels/player/shm"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Messages
type (
	backendReadyMsg   struct{}
	backendErrorMsg   struct{ err error }
	loginRequiredMsg  struct{}
	loginSuccessMsg   struct{}
	reelLoadedMsg     struct{ info *backend.ReelInfo }
	reelErrorMsg      struct{ err error }
	feedMoreFailedMsg struct{}
	backendEventMsg   backend.Event
	playbackEventMsg  player.PlaybackEvent
	videoErrorMsg     struct{ err error }
	videoReadyMsg     struct {
		index           int
		pfp             *player.Img
		contextFloating []floatingItem // reel-context pfps from the download (repost/like/sent)
		chatFloating    []floatingItem // chat-mode sender + reactor pfps
	}
	selfReactedMsg         struct{ index int }
	musicTickMsg           struct{}
	shareResetMsg          struct{}
	shareSentMsg           struct{}
	shareClosedMsg         struct{}
	shareFailedMsg         struct{}
	versionCheckMsg        struct{ latest string }
	loadingMsgsMsg         struct{ messages []string }
	loadingMsgTickMsg      struct{}
	loadingScrollTickMsg   struct{}
	loadingFadeTickMsg     struct{}
	configCheckMsg         struct{}
	loginRestartedMsg      struct{ err error }
	repostSpinMsg          struct{ gen int }
	profileEnteredMsg      struct{ info *backend.ReelInfo }
	profileExitedMsg       struct{ info *backend.ReelInfo }
	profileActionFailedMsg struct {
		action string
		err    error
		info   *backend.ReelInfo
	}
	profileFollowedMsg struct {
		following bool
		err       error
	}

	// commentLikeFailedMsg rolls back an optimistic comment like that the page
	// refused, so the UI never claims something the site didn't do
	commentLikeFailedMsg struct {
		pk      string
		restore bool
	}
)

// floatingItem is a pfp that floats in the reel's bottom-right quadrant with a
// badge overlaid, similar in theory to instagram's floating items.
//
// The sender/repost/like context pfps carry a fixed icon badge
// Chat-mode reactors carry their reaction emoji.
type floatingItem struct {
	pfp   *player.Img // reactor/sender pfp
	badge *player.Img // RepostIcon/HeartIcon/SentIcon, or EmojiBadge(reaction)
}

// State represents the app state
type state int

const (
	stateLoading state = iota
	stateLogin
	stateBrowsing
	stateError
)

// status represents the current player/loading status shown in the UI
type status int

const (
	statusNone       status = iota
	statusLoading           // reel or video is loading
	statusPaused            // playback is paused
	statusReelError         // error fetching reel metadata
	statusVideoError        // error loading video
)

// Model is the Bubble Tea model
type Model struct {
	state       state
	backend     backend.Backend
	player      *player.AVPlayer
	currentReel *backend.ReelInfo

	width   int
	height  int
	spinner spinner.Model
	status  status

	// Video pixel dimensions
	videoWidthPx  int
	videoHeightPx int

	// Video position in terminal cells (1-indexed). TUI is source of truth;
	// updated via updateVideoPosition and forwarded to the player.
	videoRow int
	videoCol int

	showNavbar bool

	// Comments panel encapsulates all comments UI state
	comments *CommentsPanel

	// Share panel encapsulates the share/DM friend selection UI
	share *SharePanel

	// Help panel displays all keybinds
	help *HelpPanel

	// Chats panel picks a DM chat whose reels to browse
	chats *ChatsPanel

	// React panel picks a reaction to send to the current chat-mode reel
	react *ReactPanel
	// dmReelsReady gates opening the chats panel until the background DM
	// collection + reel prefetch has finished (EventDMReelsReady)
	dmReelsReady bool

	flags Config

	loginSuccess bool
	// loginRestarting is set while the browser is being relaunched in headed
	// mode, so the login screen can say so and ignore repeat presses
	loginRestarting bool

	// marqueeOffset drives the horizontal scroll shared by the music line and
	// an over-long caption, in display columns
	marqueeOffset int

	// share button switches to a different emoji for 1s when clicked
	shareConfirmed bool
	shareSending   bool
	// shareCount is how many friends the in-flight share is going to, captured
	// before the panel clears itself so the confirmation can name them
	shareCount int

	hud HUD

	reelPFP *player.Img
	// reelFloating holds the reel-context pfps from the download; floating is
	// what's rendered: reelFloating plus the chat-mode sender/reactor pfps,
	// which get rebuilt whenever the user reacts.
	reelFloating []floatingItem
	floating     []floatingItem

	// hoveredStatus is the status-line control the pointer is currently over,
	// highlighted so the row reads as clickable
	hoveredStatus statusAction

	// scrubbing is set while the pointer drags the reel's progress bar
	scrubbing bool
	// volumeDragging keeps the footer volume slider active while the pointer
	// moves, independently from video progress scrubbing.
	volumeDragging bool

	// lastWheelStep is when the last wheel notch was acted on, used to collapse
	// the burst of events a high-resolution scroll produces
	lastWheelStep time.Time

	// prefetchCancel stops downloads that belong to a reel the user has
	// already navigated away from.
	prefetchCancel context.CancelFunc

	// profileBusy prevents rapid follow/back/navigation input from scheduling
	// conflicting browser navigations before the previous one completes.
	profileBusy bool

	// repostSpin counts down the frames left in the repost icon's animation;
	// repostGen discards ticks from a superseded animation
	repostSpin int
	repostGen  int

	// configDir is watched so edits to reels.conf apply without a restart
	configDir string

	version         string
	updateAvailable string
	lastErr         error

	loadingMessages  []string
	loadingMsgIndex  int
	loadingMsgScroll int
	loadingFadeStep  int // 0=visible, 1-6=fading out, 7-12=fading in
}

type Config struct {
	HeadedMode bool
	LoginMode  bool
}

// NewModel creates a new TUI model
func NewModel(userDataDir, logDir, cacheDir, configDir string, output io.Writer, version string, flags Config) Model {
	backend.LoadSettings(configDir)
	backend.InitLogger(logDir)
	settings := backend.GetSettings()

	playerHeight := settings.ReelHeight * settings.RetinaScale
	playerWidth := settings.ReelWidth * settings.RetinaScale
	player.ComputeVideoCharacterDimensions(playerWidth, playerHeight)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = yellow500

	p := player.NewAVPlayer()
	p.SetSize(playerWidth, playerHeight)
	p.SetVolume(settings.Volume)
	p.SetUseShm(shm.ShmSupported())
	p.SetRetinaScale(settings.RetinaScale)

	b := backend.NewChromeBackend(userDataDir, cacheDir, configDir)

	return Model{
		state:         stateLoading,
		backend:       b,
		player:        p,
		spinner:       s,
		status:        statusLoading,
		videoWidthPx:  playerWidth,
		videoHeightPx: playerHeight,
		comments:      NewCommentsPanel(),
		share:         NewSharePanel(),
		help:          NewHelpPanel(),
		chats:         NewChatsPanel(),
		react:         NewReactPanel(),
		flags:         flags,
		showNavbar:    settings.ShowNavbar,
		configDir:     configDir,
		version:       version,
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.startBackend,
		m.checkVersion,
		m.fetchLoadingMessages,
		m.watchConfig(),
		m.listenForPlaybackEvents,
	)
}

// watchConfig schedules the next poll of reels.conf for external edits.
func (m Model) watchConfig() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return configCheckMsg{}
	})
}

// applySettings installs settings that were changed outside the app (by an
// edit to reels.conf) onto the running player and layout. Keybinds need no
// work here: every handler reads them from GetSettings on each press.
func (m *Model) applySettings(s backend.Settings) {
	m.showNavbar = s.ShowNavbar
	m.player.SetVolume(s.Volume)
	m.player.SetRetinaScale(s.RetinaScale)

	m.relayout()
	m.player.RedrawVideo()
}

func (m Model) startBackend() tea.Msg {
	if err := m.backend.Start(!(m.flags.HeadedMode || m.flags.LoginMode)); err != nil {
		return backendErrorMsg{err}
	}

	needsLogin, err := m.backend.NeedsLogin()
	if err != nil {
		return backendErrorMsg{err}
	}

	if needsLogin {
		return loginRequiredMsg{}
	}

	// if we don't need login, that means success
	if m.flags.LoginMode {
		return loginSuccessMsg{}
	}

	if err := m.backend.NavigateToReels(); err != nil {
		return backendErrorMsg{err}
	}

	return backendReadyMsg{}
}

func (m Model) listenForEvents() tea.Msg {
	event, ok := <-m.backend.Events()
	if !ok {
		return nil
	}
	return backendEventMsg(event)
}

func (m Model) listenForPlaybackEvents() tea.Msg {
	return playbackEventMsg(<-m.player.Events())
}

func (m Model) loadCurrentReel() tea.Msg {
	info, err := m.backend.GetCurrent()
	if err != nil {
		return reelErrorMsg{err}
	}
	return reelLoadedMsg{info}
}

// restartHeaded closes the headless browser and reopens it visibly, so the
// user can log in without quitting and re-running with --login.
func (m Model) restartHeaded() tea.Msg {
	m.backend.Stop()
	if err := m.backend.Start(false); err != nil {
		return loginRestartedMsg{err: err}
	}
	return loginRestartedMsg{}
}

func (m Model) checkLoginStatus() tea.Msg {
	// Poll every 2 seconds to check if user has logged in via the browser
	time.Sleep(2 * time.Second)
	needsLogin, err := m.backend.NeedsLogin()
	if err != nil {
		// Browser might be navigating, keep polling
		return loginRequiredMsg{}
	}
	if !needsLogin {
		return loginSuccessMsg{}
	}
	return loginRequiredMsg{}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if slices.Contains(backend.GetSettings().KeysQuit, key) {
			// The panel-open shrink is derived at render time and never
			// persisted, so there is nothing to restore before quitting.
			if m.prefetchCancel != nil {
				m.prefetchCancel()
			}
			m.player.Close()
			if m.backend != nil {
				m.backend.Stop()
			}
			return m, tea.Quit
		}

		if m.state == stateBrowsing {
			return m.updateBrowsing(msg)
		}

		// The login screen used to end at "restart with --login". Enter
		// relaunches the browser in place instead.
		if m.state == stateLogin && !m.flags.LoginMode && !m.loginRestarting && key == "enter" {
			m.loginRestarting = true
			return m, m.restartHeaded
		}

		// A failed start is often transient (Instagram redirect, cold Chrome
		// profile), so offer a retry rather than only a quit.
		if m.state == stateError && key == "r" {
			m.state = stateLoading
			m.status = statusLoading
			m.lastErr = nil
			return m, m.startBackend
		}

	case tea.MouseMsg:
		if m.state == stateBrowsing {
			return m.updateMouse(msg)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Re-derive the reel size for the new terminal, then re-center.
		// In fit mode this is what makes the reel grow with the window.
		m.relayout()
		if m.reelPFP != nil {
			m.reelPFP.ResizeToCells(reelPfpCells)
		}
		for _, item := range m.floating {
			if item.pfp != nil {
				item.pfp.ResizeToCells(3)
			}
		}
		if m.share.IsOpen() {
			m.share.ResizePfps()
		} else if m.comments.IsOpen() {
			m.comments.ResizeGifs()
			m.updateCommentGifs()
		}
		m.updateImages()
		m.player.RedrawVideo()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case versionCheckMsg:
		m.updateAvailable = msg.latest
		return m, nil

	case configCheckMsg:
		if !backend.ConfigFileChanged(m.configDir) {
			return m, m.watchConfig()
		}
		s, changed := backend.ReloadSettings(m.configDir)
		if !changed {
			return m, m.watchConfig()
		}
		m.applySettings(s)
		if m.state != stateBrowsing {
			return m, m.watchConfig()
		}
		return m, tea.Batch(m.hud.ShowToast("config reloaded"), m.watchConfig())

	case loadingMsgsMsg:
		if len(msg.messages) > 0 {
			m.loadingMessages = msg.messages
			m.loadingMsgIndex = 0
			m.loadingMsgScroll = 0
			m.loadingFadeStep = 7
			return m, tea.Batch(m.loadingFadeTick(), m.loadingScrollTick())
		}
		return m, nil

	case loadingMsgTickMsg:
		if m.state != stateLoading || len(m.loadingMessages) == 0 || m.updateAvailable != "" {
			return m, nil
		}
		// Start fade-out instead of immediately swapping
		m.loadingFadeStep = 1
		return m, m.loadingFadeTick()

	case loadingFadeTickMsg:
		if m.state != stateLoading || len(m.loadingMessages) == 0 {
			return m, nil
		}
		m.loadingFadeStep++
		// Midpoint: swap to next message
		if m.loadingFadeStep == 7 {
			m.loadingMsgIndex = (m.loadingMsgIndex + 1) % len(m.loadingMessages)
			m.loadingMsgScroll = 0
		}
		// Fade complete
		if m.loadingFadeStep >= 13 {

			m.loadingFadeStep = 0
			return m, m.loadingMsgTick()
		}
		return m, m.loadingFadeTick()

	case loadingScrollTickMsg:
		if m.state != stateLoading || len(m.loadingMessages) == 0 {
			return m, nil
		}
		m.loadingMsgScroll++
		return m, m.loadingScrollTick()

	case backendReadyMsg:
		m.state = stateBrowsing
		m.status = statusLoading
		return m, tea.Batch(
			m.loadCurrentReel,
			m.listenForEvents,
			m.musicTick(),
		)

	case loginRequiredMsg:
		m.state = stateLogin
		if m.flags.LoginMode {
			// In login mode, poll for login completion
			return m, m.checkLoginStatus
		}
		// In normal mode, just show message to restart with --login
		return m, nil

	case loginSuccessMsg:
		m.state = stateLogin
		m.loginSuccess = true
		m.loginRestarting = false
		return m, nil

	case repostSpinMsg:
		if msg.gen != m.repostGen || m.repostSpin <= 0 {
			return m, nil
		}
		m.repostSpin--
		if m.repostSpin > 0 {
			return m, m.repostSpinTick()
		}
		return m, nil

	case commentLikeFailedMsg:
		m.comments.SetCommentLikedByPK(msg.pk, msg.restore)
		return m, m.hud.ShowToast("couldn't like that comment")

	case loginRestartedMsg:
		m.loginRestarting = false
		if msg.err != nil {
			m.lastErr = msg.err
			m.state = stateError
			return m, nil
		}
		// The browser is now visible; poll until the user has logged in, the
		// same way --login does.
		m.flags.LoginMode = true
		return m, m.checkLoginStatus

	case backendErrorMsg:
		m.lastErr = msg.err
		m.state = stateError
		return m, nil

	case backendEventMsg:
		switch msg.Type {
		case backend.EventCommentsCaptured:
			m.comments.SetLoading(false)
			// Refresh currentReel to get the newly persisted comments
			if m.currentReel != nil {
				if info, err := m.backend.GetReel(m.currentReel.Index); err == nil {
					m.currentReel = info
					m.comments.SetComments(info.PK, info.Comments)
					m.updateCommentGifs()
				}
			}
		case backend.EventShareFriendsLoaded:
			if m.share.IsOpen() {
				m.share.SetFriends(m.backend.GetShareFriends())
				m.updateImages()
			}
		case backend.EventDMReelsReady:
			m.dmReelsReady = true
			if msg.Count > 0 {
				return m, tea.Batch(m.hud.ShowDMNotify(msg.Count), m.listenForEvents)
			}
		case backend.EventChatModeExited:
			m.player.Stop()
			m.status = statusLoading
			m.comments.Clear()
			m.hud.HideChatBanner()
			return m, tea.Batch(m.loadCurrentReel, m.listenForEvents)
		}
		return m, m.listenForEvents

	case playbackEventMsg:
		event := player.PlaybackEvent(msg)
		switch event.Type {
		case player.PlaybackRestarting:
			return m, tea.Batch(
				m.hud.ShowToast("recovering playback"),
				m.listenForPlaybackEvents,
			)
		case player.PlaybackFailed:
			m.status = statusVideoError
			m.lastErr = event.Err
			return m, tea.Batch(
				m.hud.ShowToast("playback stopped safely"),
				m.listenForPlaybackEvents,
			)
		default:
			return m, m.listenForPlaybackEvents
		}

	case reelLoadedMsg:
		m.profileBusy = false
		m.currentReel = msg.info
		m.status = statusNone
		m.marqueeOffset = 0
		return m, m.startPlayback(msg.info.Index)

	case profileEnteredMsg:
		m.profileBusy = false
		m.currentReel = msg.info
		m.status = statusNone
		m.comments.Clear()
		m.marqueeOffset = 0
		return m, tea.Batch(m.startPlayback(msg.info.Index), m.hud.ShowToast("creator reels opened"))

	case profileExitedMsg:
		m.profileBusy = false
		m.currentReel = msg.info
		m.status = statusNone
		m.comments.Clear()
		m.marqueeOffset = 0
		return m, tea.Batch(m.startPlayback(msg.info.Index), m.hud.ShowToast("back to main feed"))

	case profileActionFailedMsg:
		m.profileBusy = false
		m.status = statusNone
		if msg.info != nil {
			m.currentReel = msg.info
			return m, tea.Batch(m.startPlayback(msg.info.Index), m.hud.ShowToast(msg.action+" failed"))
		}
		return m, m.hud.ShowToast(msg.action + " failed")

	case profileFollowedMsg:
		m.profileBusy = false
		if msg.err != nil {
			return m, m.hud.ShowToast("follow action failed")
		}
		if msg.following {
			return m, m.hud.ShowToast("following creator")
		}
		return m, m.hud.ShowToast("unfollowed creator")

	case musicTickMsg:
		// Advances for any reel, not just ones with music: a long caption
		// scrolls off the same counter.
		if m.currentReel != nil {
			m.marqueeOffset++
		}
		return m, m.musicTick()

	case volumeHoldMsg, volumeFadeTickMsg, dmNotifyHoldMsg, dmNotifyFadeTickMsg,
		chatBannerHoldMsg, chatBannerFadeTickMsg, toastHoldMsg, toastFadeTickMsg:
		if handled, updated, cmd := m.updateHUD(msg); handled {
			return updated, cmd
		}

	case shareResetMsg:
		m.shareConfirmed = false
		return m, nil

	case shareFailedMsg:
		m.shareSending = false
		return m, m.hud.ShowToast("couldn't send that")

	case shareClosedMsg:
		if m.share.IsOpen() {
			m.share.Close()
			m.closePanelLayout()
		}
		m.shareSending = false
		return m, nil

	case shareSentMsg:
		if m.share.IsOpen() {
			m.share.Close()
			m.closePanelLayout()
		}
		m.shareSending = false
		m.shareConfirmed = true
		// A tick of the share icon is easy to miss, so say it outright.
		return m, tea.Batch(m.queueShareReset(), m.hud.ShowToast(sentToast(m.shareCount)))

	case reelErrorMsg:
		m.profileBusy = false
		m.status = statusReelError
		return m, nil

	case feedMoreFailedMsg:
		// A lazy page can fail transiently without meaning the infinite feed
		// is exhausted. Restore browsing so the next scroll retries discovery.
		m.status = statusNone
		return m, m.hud.ShowToast("loading more reels failed — scroll to retry")

	case videoReadyMsg:
		m.status = statusNone
		m.reelPFP = msg.pfp
		m.reelFloating = msg.contextFloating
		m.floating = append(slices.Clone(msg.contextFloating), msg.chatFloating...)
		m.updateVideoPosition()
		m.updateImages()
		if profile, ok := m.backend.(backend.ProfileBackend); ok && profile.IsProfileMode() {
			if m.prefetchCancel != nil {
				m.prefetchCancel()
				m.prefetchCancel = nil
			}
			return m, nil
		}
		if m.prefetchCancel != nil {
			m.prefetchCancel()
		}
		prefetchCtx, cancel := context.WithCancel(context.Background())
		m.prefetchCancel = cancel
		go m.prefetch(prefetchCtx, msg.index)
		return m, nil

	case selfReactedMsg:
		if m.currentReel != nil && m.currentReel.Index == msg.index {
			m.floating = append(slices.Clone(m.reelFloating), m.chatFloating(msg.index)...)
			m.updateImages()
		}
		return m, nil

	case videoErrorMsg:
		m.status = statusVideoError
		return m, nil
	}

	return m, nil
}

// View renders the UI
func (m Model) View() string {
	switch m.state {
	case stateLoading:
		return m.viewLoading()
	case stateLogin:
		return m.viewLogin()
	case stateError:
		return m.viewError()
	case stateBrowsing:
		return m.viewBrowsing()
	default:
		return ""
	}
}
