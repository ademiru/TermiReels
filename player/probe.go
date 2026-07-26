package player

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// GraphicsSupported reports whether the terminal speaks the Kitty graphics
// protocol. Everything this app draws — video frames, avatars, comment GIFs —
// goes out as Kitty escape sequences, so a terminal without it shows garbage
// rather than a reel.
//
// IMPORTANT: MUST BE CALLED BEFORE BUBBLETEA STARTS, since it puts stdin in
// raw mode and reads the terminal's reply directly.
func GraphicsSupported() bool {
	// a=q asks the terminal to validate a transmission without displaying it.
	// A terminal that implements the protocol answers ESC _ G i=31;OK ESC \.
	payload := base64.StdEncoding.EncodeToString([]byte{0, 0, 0})
	graphics := fmt.Sprintf("\x1b_Ga=q,i=%d,s=1,v=1,f=24,t=d;%s\x1b\\", probeImageID, payload)

	// Primary Device Attributes is answered by every terminal, so chaining it
	// after the graphics query gives a definite end to the reply. Without it
	// an unsupported terminal is indistinguishable from a slow one and we'd
	// have to sit out the full timeout on every launch.
	reply := probeTerminal(graphics + "\x1b[c")

	return strings.Contains(reply, "OK")
}

const probeImageID = 31

// probeTerminal writes query to stdout with stdin in raw mode and returns what
// the terminal writes back, stopping as soon as the reply looks complete.
func probeTerminal(query string) string {
	if !isTerminal(os.Stdout) || !isTerminal(os.Stdin) {
		return ""
	}

	stdinFd := int(os.Stdin.Fd())

	oldTermios, err := unix.IoctlGetTermios(stdinFd, ioctlGetTermios)
	if err != nil {
		return ""
	}

	raw := *oldTermios
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG
	raw.Iflag &^= unix.IXON | unix.ICRNL
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 2 // 200ms per read
	if err := unix.IoctlSetTermios(stdinFd, ioctlSetTermios, &raw); err != nil {
		return ""
	}
	defer unix.IoctlSetTermios(stdinFd, ioctlSetTermios, oldTermios)

	// Drop anything already buffered so it isn't mistaken for the reply.
	drain := make([]byte, 256)
	os.Stdin.Read(drain)

	if _, err := os.Stdout.WriteString(query); err != nil {
		return ""
	}

	var reply strings.Builder
	buf := make([]byte, 256)
	// Five 200ms reads is a full second of patience, but replyComplete
	// normally breaks out on the first one.
	for range 5 {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			reply.Write(buf[:n])
			if replyComplete(reply.String()) {
				break
			}
		}
		if err != nil {
			break
		}
	}
	return reply.String()
}

// replyComplete reports whether the terminal has finished answering the probe,
// which it has once the Primary Device Attributes reply (CSI ? ... c) arrives.
func replyComplete(reply string) bool {
	i := strings.Index(reply, "\x1b[?")
	return i >= 0 && strings.Contains(reply[i:], "c")
}

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlGetTermios)
	return err == nil
}
