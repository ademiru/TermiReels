package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ademiru/TermiReels/player"
	"github.com/ademiru/TermiReels/tui"
	tea "github.com/charmbracelet/bubbletea"
)

var Version = "dev"

// SyncFile wraps *os.File with a mutex to serialize writes while preserving Fd() for ioctls
type SyncFile struct {
	mu sync.Mutex
	*os.File
}

func main() {
	loginFlag := flag.Bool("login", false, "Open browser in headed mode for Instagram login, also used for debugging since the app does not try to control the browser.")
	headedFlag := flag.Bool("headed", false, "Run browser in headed mode")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	skipTermCheck := flag.Bool("skip-terminal-check", false, "Start even if the terminal doesn't answer the Kitty graphics probe")
	shortcutFlag := flag.Bool("shortcut", false, "Edit keyboard shortcuts and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(Version)
		return
	}

	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "reels")

	// The shortcut editor only touches reels.conf, so it skips the graphics
	// probe, the browser and the player entirely and opens instantly.
	if *shortcutFlag {
		p := tea.NewProgram(tui.NewShortcutsModel(configDir), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Every pixel this app draws goes out as a Kitty graphics escape. Probing
	// up front turns "my screen filled with garbage" into an explanation.
	// Must happen before Bubble Tea takes over the terminal.
	if !*skipTermCheck && !player.GraphicsSupported() {
		fmt.Fprint(os.Stderr, unsupportedTerminalMessage())
		os.Exit(1)
	}

	// If installed via npm and a newer release exists, swap the binary on disk
	// and re-exec into it. Does nothing for non-npm installs or when already up
	// to date. Must run before any child processes are spawned.

	// Set up directories:
	// Browser data: 	~/.local/share/reels/
	// Logs:			~/.local/state/reels/
	// Cache:			~/.cache/reels/
	// Settings: 		~/.config/reels/
	userDataDir := filepath.Join(homeDir, ".local", "share", "reels", "chrome-data")
	logDir := filepath.Join(homeDir, ".local", "state", "reels")
	cacheDir := filepath.Join(homeDir, ".cache", "reels")

	// Create synchronized file wrapper for both Bubble Tea and video renderer
	syncOut := &SyncFile{File: os.Stdout}

	p := tea.NewProgram(
		tui.NewModel(userDataDir, logDir, cacheDir, configDir, syncOut, Version, tui.Config{LoginMode: *loginFlag, HeadedMode: *headedFlag}),
		tea.WithAltScreen(),
		// All-motion tracking, not just drag, so the status row can highlight
		// the control under the pointer. updateMouse discards motion events
		// off that row before they cost anything.
		tea.WithMouseAllMotion(),
		tea.WithOutput(syncOut),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// unsupportedTerminalMessage explains why reels won't start here and what to
// do about it. TERM is included because a multiplexer is the usual culprit:
// tmux and screen don't pass graphics escapes through.
func unsupportedTerminalMessage() string {
	var b strings.Builder
	b.WriteString("reels needs a terminal that supports the Kitty graphics protocol,\n")
	b.WriteString("and this one did not answer the probe.\n\n")
	fmt.Fprintf(&b, "  TERM=%s", os.Getenv("TERM"))
	if mux := os.Getenv("TMUX"); mux != "" {
		b.WriteString("  (running inside tmux)")
	} else if strings.HasPrefix(os.Getenv("TERM"), "screen") {
		b.WriteString("  (running inside screen)")
	}
	b.WriteString("\n\n")
	b.WriteString("Known-good terminals: Ghostty, Kitty, WezTerm, iTerm2, st, Konsole.\n")
	b.WriteString("tmux and screen do not forward graphics escapes; run reels outside them.\n\n")
	b.WriteString("If you believe this is wrong, start anyway with:\n\n")
	b.WriteString("  reels --skip-terminal-check\n")
	return b.String()
}
