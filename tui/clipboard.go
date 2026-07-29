package tui

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/ademiru/TermiReels/internal/platformenv"
)

func copyToClipboard(text string) {
	for _, candidate := range clipboardCandidates(runtime.GOOS, platformenv.IsWSL()) {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, candidate[1:]...)
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return
	}
}

func clipboardCandidates(goos string, wsl bool) [][]string {
	switch {
	case goos == "darwin":
		return [][]string{{"pbcopy"}}
	case goos == "windows":
		return [][]string{{"clip.exe"}}
	case wsl:
		// Windows executables are exposed in WSL's PATH by default. Using the
		// host clipboard avoids requiring a Linux display clipboard daemon.
		return [][]string{{"clip.exe"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}}
	default:
		return [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}}
	}
}
