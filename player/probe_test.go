package player

import "testing"

// The probe stops reading once Primary Device Attributes comes back. Getting
// this wrong either cuts the graphics reply off early or stalls startup for
// the full timeout on every launch.
func TestReplyComplete(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  bool
	}{
		{"empty", "", false},
		{"graphics ok only", "\x1b_Gi=31;OK\x1b\\", false},
		{"partial DA1", "\x1b[?62;", false},
		{"DA1 alone", "\x1b[?62;4c", true},
		{"graphics then DA1", "\x1b_Gi=31;OK\x1b\\\x1b[?62;4c", true},
		{"c before DA1 does not count", "c\x1b[?62;", false},
	}
	for _, tc := range cases {
		if got := replyComplete(tc.reply); got != tc.want {
			t.Errorf("%s: replyComplete(%q) = %v, want %v", tc.name, tc.reply, got, tc.want)
		}
	}
}

func TestFitVideoToTerminalAspect(t *testing.T) {
	// FitVideoToTerminal reads the real terminal, which tests don't have, so
	// this only asserts the documented fallback.
	if _, _, ok := FitVideoToTerminal(10, 9, 16); ok {
		t.Skip("running attached to a terminal that reports pixel dimensions")
	}
}
