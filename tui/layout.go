package tui

import (
	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/player"
	"github.com/charmbracelet/lipgloss"
)

// Reels are portrait video.
const (
	reelAspectW = 9
	reelAspectH = 16
)

// The creator's avatar is drawn over the two rows directly below the reel —
// the username and music lines. reelPfpCells is its height in cells, and
// pfpGutter the columns of indent every line under the reel needs so the
// avatar never lands on text. A square avatar two cells tall is roughly four
// columns wide, so five leaves a column of air.
const (
	reelPfpCells = 2
	pfpGutter    = 5
	// A 64-column terminal can still fit a useful 26-column comments rail if
	// fit mode gives the remaining space to the reel. The previous 90-column
	// cutoff classified ordinary split terminals as "narrow", so users saw
	// the old below-reel panel even after opting into the side layout.
	sidePanelMinTerminalWidth = 64
	sidePanelMinWidth         = 26
	sidePanelMaxWidth         = 44
	sidePanelGap              = 2
)

// Rows viewBrowsing draws around the reel. Used to work out how much height
// the reel itself can have in fit mode.
const (
	// A small fixed HUD band. The reel is top-anchored below it.
	chromeRowsAbove = 2
	// Below the reel with the navbar shown: username, music, caption, a blank
	// row, and three navbar lines.
	chromeRowsBelowNavbar = 10
	// Below the reel with the navbar hidden: username, music, and room for the
	// caption wrapped over several lines.
	chromeRowsBelowCaption = 10
	// Below the reel with a panel open: username, music, and the panel itself.
	chromeRowsBelowPanel = 14
	compactViewportRows  = 32
	compactChromeRows    = 6
)

func (m Model) compactViewport() bool {
	return m.height > 0 && m.height < compactViewportRows
}

// chromeRows is how many terminal rows the UI around the reel needs.
//
// The reel is top-anchored, so the top and bottom reservations are each paid
// once. The previous centring formula doubled the larger side, leaving a large
// dead band above the video and shrinking it unnecessarily.
func (m Model) chromeRows() int {
	if m.compactViewport() {
		if m.panelOpen() && !m.commentsOnSide() {
			// A below-reel panel still needs usable content, but take fewer
			// rows than the desktop layout so the reel remains recognizable.
			return 10
		}
		return compactChromeRows
	}
	if m.commentsOnSide() {
		return chromeRowsAbove + chromeRowsBelowNavbar
	}
	if m.panelOpen() {
		return chromeRowsAbove + chromeRowsBelowPanel
	}
	below := chromeRowsBelowCaption
	if m.showNavbar {
		below = chromeRowsBelowNavbar
	}
	return chromeRowsAbove + below
}

// reelPixelSize is the reel's target size in device pixels for the current
// terminal, fit mode, and panel state.
//
// In fit mode the size is derived from the terminal on every call, so it is
// never written to the config. In fixed mode it comes from reel_width /
// reel_height, shrunk while a panel is open to make room for it.
func (m Model) reelPixelSize() (widthPx, heightPx int) {
	s := backend.GetSettings()
	scale := max(s.RetinaScale, 1)

	if s.ReelFit {
		reservedCols := 2
		if m.commentsOnSide() {
			reservedCols += m.sidePanelWidth() + sidePanelGap
		}
		if w, h, ok := player.FitVideoToTerminalArea(m.chromeRows(), reservedCols, reelAspectW, reelAspectH); ok {
			return w, h
		}
		// Terminal reports no pixel size; fall through to the fixed size.
	}

	width, height := s.ReelWidth, s.ReelHeight
	if m.panelOpen() && !m.commentsOnSide() {
		shrunk := width - s.ReelSizeStep*s.PanelShrinkSteps
		if shrunk >= s.ReelSizeStep {
			width = shrunk
			height = shrunk * reelAspectH / reelAspectW
		}
	}
	return width * scale, height * scale
}

// commentsOnSide chooses the desktop layout. Narrow terminals keep the
// vertical fallback so comments never crush the reel into an unusable strip.
func (m Model) commentsOnSide() bool {
	if !m.comments.IsOpen() || m.width < sidePanelMinTerminalWidth {
		return false
	}
	if backend.GetSettings().ReelFit {
		return true
	}
	return player.VideoWidthChars+sidePanelGap+sidePanelMinWidth <= m.width-2
}

func (m Model) sidePanelWidth() int {
	return min(max(m.width/3, sidePanelMinWidth), sidePanelMaxWidth)
}

// panelBaseCol is the panel's 1-indexed terminal column. Comments can live to
// the right of the reel; the other panels remain aligned under it.
func (m Model) panelBaseCol() int {
	if m.commentsOnSide() {
		return m.videoCol + player.VideoWidthChars + sidePanelGap
	}
	return m.videoCol
}

// panelBaseRow is the 1-indexed terminal row of an open panel's header line:
// the reel, then the username and music lines. Overlay images and the mouse
// hit-test both measure from here, so they agree on where a panel's rows are.
func (m Model) panelBaseRow() int {
	if m.commentsOnSide() {
		return m.videoRow
	}
	return m.videoRow + player.VideoHeightChars + 2
}

// panelLines is how many rows a panel has to draw into.
func (m Model) panelLines() int {
	if m.commentsOnSide() {
		return max(min(player.VideoHeightChars, m.height-(m.videoRow-1)-1), 1)
	}
	return max(m.height-m.panelBaseRow(), 1)
}

func (m Model) panelWidth() int {
	if m.commentsOnSide() {
		return min(m.sidePanelWidth(), max(m.width-(m.panelBaseCol()-1), 1))
	}
	return max(player.VideoWidthChars-1, 1)
}

func (m Model) panelContentBaseCol() int {
	if m.commentsOnSide() {
		return m.panelBaseCol() + 1 // rounded border
	}
	return m.panelBaseCol()
}

func (m Model) panelContentBaseRow() int {
	if m.commentsOnSide() {
		return m.panelBaseRow() + 1 // rounded border
	}
	return m.panelBaseRow()
}

func (m Model) panelContentWidth() int {
	if m.commentsOnSide() {
		return max(m.panelWidth()-2, 1)
	}
	return m.panelWidth()
}

func (m Model) panelContentLines() int {
	if m.commentsOnSide() {
		return max(m.panelLines()-2, 1)
	}
	return m.panelLines()
}

// statusLineRow is the zero-indexed footer row shared by rendering and mouse
// hit-testing. One terminal row is deliberately left below it: writing a
// wide/emoji-heavy line into the physical bottom row makes some terminals
// auto-wrap/scroll when their emoji width differs from lipgloss by one cell,
// leaving a duplicate footer behind.
func (m Model) statusLineRow() int {
	if m.compactViewport() {
		return max(m.height-2, 0)
	}
	return max(m.height-3, 0)
}

func (m Model) statusLabelRow() int {
	if m.compactViewport() {
		return -1
	}
	return max(m.height-2, 0)
}

func (m Model) volumeSliderRow() int {
	if m.compactViewport() {
		return max(m.statusLineRow()-1, 0)
	}
	return max(m.statusLineRow()-1, 0)
}

func (m Model) statusLineStart() int {
	width := 0
	for _, seg := range m.statusSegments() {
		width += lipgloss.Width(seg.text)
	}
	return max((m.width-width)/2, 0)
}

// relayout recomputes the reel size and repositions everything anchored to it.
// Call it after anything that changes the available space: a terminal resize,
// a panel opening or closing, or a config reload.
func (m *Model) relayout() {
	w, h := m.reelPixelSize()
	if w < 1 || h < 1 {
		return
	}

	m.videoWidthPx, m.videoHeightPx = w, h
	player.ComputeVideoCharacterDimensions(w, h)
	m.player.SetSize(w, h)
	m.updateVideoPosition()
	m.updateImages()
	m.updateCommentGifs()
}

// nudgeReelSize resizes the reel by delta pixels of width, keeping the aspect
// ratio, and persists the result.
//
// Resizing by hand means the user wants to pick the size themselves, so this
// also leaves fit mode. Otherwise the next terminal resize would immediately
// throw the adjustment away.
func (m *Model) nudgeReelSize(delta int) {
	s := backend.GetSettings()
	scale := max(s.RetinaScale, 1)

	// In fit mode reel_width holds a stale value, so start from what is
	// actually on screen and stop the first nudge from jumping.
	baseW := s.ReelWidth
	if s.ReelFit {
		baseW = m.videoWidthPx / scale
		if m.panelOpen() {
			// Store the unshrunk size, since that is what the config holds.
			baseW += s.ReelSizeStep * s.PanelShrinkSteps
		}
	}

	newW := baseW + delta
	newH := newW * reelAspectH / reelAspectW
	if newW < s.ReelSizeStep || newH < s.ReelSizeStep {
		return
	}

	if s.ReelFit {
		m.backend.SetReelFit(false)
	}
	if err := m.backend.SetReelSize(newW, newH); err != nil {
		return
	}
	m.relayout()
	m.player.RedrawVideo()
}
