package tui

import "testing"

func TestClipboardCandidatesPreferWindowsHostInsideWSL(t *testing.T) {
	got := clipboardCandidates("linux", true)
	if len(got) == 0 || len(got[0]) == 0 || got[0][0] != "clip.exe" {
		t.Fatalf("WSL clipboard candidates = %#v, want clip.exe first", got)
	}
}

func TestClipboardCandidatesKeepLinuxFallbacks(t *testing.T) {
	got := clipboardCandidates("linux", false)
	if len(got) != 2 || got[0][0] != "wl-copy" || got[1][0] != "xclip" {
		t.Fatalf("Linux clipboard candidates = %#v", got)
	}
}
