package player

import (
	"os"

	"github.com/ademiru/TermiReels/internal/platformenv"
	"golang.org/x/sys/unix"
)

// Video dimensions in terminal characters
var (
	VideoWidthChars  = 1
	VideoHeightChars = 1
)

// ComputeVideoDimensions calculates the video character dimensions from pixel dimensions.
// Call this after loading settings and on terminal resize to update VideoWidthChars and VideoHeightChars.
func ComputeVideoCharacterDimensions(videoWidthPx, videoHeightPx int) {
	cols, rows, termW, termH, err := GetTerminalSize()
	if err != nil || termW == 0 || termH == 0 || cols == 0 || rows == 0 {
		VideoWidthChars = 1
		VideoHeightChars = 1
		return
	}

	cellW := termW / cols
	cellH := termH / rows

	VideoWidthChars = (videoWidthPx + cellW - 1) / cellW
	VideoHeightChars = (videoHeightPx + cellH - 1) / cellH
}

// ComputeVideoCenterPosition computes the 1-indexed (row, col) to center the video in the terminal.
// Uses the actual video pixel dimensions so videos with non-standard aspect ratios are centered correctly.
func ComputeVideoCenterPosition(videoWidthPx, videoHeightPx int) (row, col int) {
	cols, rows, termW, termH, err := GetTerminalSize()
	if err != nil || cols == 0 || rows == 0 || termW == 0 || termH == 0 {
		return 1, 1
	}

	cellW := termW / cols
	cellH := termH / rows

	videoCols := (videoWidthPx + cellW - 1) / cellW
	videoRows := (videoHeightPx + cellH - 1) / cellH

	col = (cols-videoCols)/2 + 1
	row = (rows-videoRows)/2 + 1
	if col < 1 {
		col = 1
	}
	if row < 1 {
		row = 1
	}
	return row, col
}

// FitVideoToTerminal returns the pixel size of the largest aspectW:aspectH
// video that fits the terminal with reservedRows left over for the UI drawn
// around it, plus a one-column margin on each side.
//
// The size comes out in real device pixels, because the cell size is derived
// from the pixel dimensions the terminal reports. That makes the decode
// resolution match the display exactly on HiDPI screens, which is what
// retina_scale approximates when the reel size is fixed.
//
// ok is false when the terminal doesn't report pixel dimensions; callers
// should fall back to the configured size.
func FitVideoToTerminal(reservedRows, aspectW, aspectH int) (widthPx, heightPx int, ok bool) {
	return FitVideoToTerminalArea(reservedRows, 2, aspectW, aspectH)
}

// FitVideoToTerminalArea is FitVideoToTerminal with an explicit horizontal
// reservation. Side panels use it to leave terminal columns beside the reel
// instead of shrinking the video only after it has already been centred.
func FitVideoToTerminalArea(reservedRows, reservedCols, aspectW, aspectH int) (widthPx, heightPx int, ok bool) {
	cols, rows, termW, termH, err := GetTerminalSize()
	if err != nil || cols == 0 || rows == 0 || termW == 0 || termH == 0 {
		return 0, 0, false
	}

	cellW := termW / cols
	cellH := termH / rows
	if cellW == 0 || cellH == 0 {
		return 0, 0, false
	}

	availRows := max(rows-reservedRows, 1)
	availCols := max(cols-reservedCols, 1)

	heightPx = availRows * cellH
	widthPx = heightPx * aspectW / aspectH

	// Narrow windows run out of width before height.
	if maxWidthPx := availCols * cellW; widthPx > maxWidthPx {
		widthPx = maxWidthPx
		heightPx = widthPx * aspectH / aspectW
	}

	widthPx, heightPx = limitFitTransportSize(
		widthPx, heightPx, aspectW, aspectH, platformenv.IsWSL(),
	)
	if widthPx < 1 || heightPx < 1 {
		return 0, 0, false
	}
	return widthPx, heightPx, true
}

const wslFitMaxHeightPx = 720

// limitFitTransportSize prevents fit mode from turning a large/HiDPI Windows
// terminal into hundreds of megabytes per second of direct Kitty RGB traffic.
// Fixed-size mode remains user-controlled and is intentionally not capped.
func limitFitTransportSize(width, height, aspectW, aspectH int, wsl bool) (int, int) {
	if !wsl || height <= wslFitMaxHeightPx || aspectW <= 0 || aspectH <= 0 {
		return width, height
	}
	height = wslFitMaxHeightPx
	width = height * aspectW / aspectH
	return width, height
}

// GetTerminalSize returns terminal dimensions (cols, rows, widthPx, heightPx)
func GetTerminalSize() (cols, rows, widthPx, heightPx int, err error) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return int(ws.Col), int(ws.Row), int(ws.Xpixel), int(ws.Ypixel), nil
}
