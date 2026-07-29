package platformenv

import "testing"

func TestIsWSLRelease(t *testing.T) {
	for _, test := range []struct {
		release string
		want    bool
	}{
		{"6.6.87.2-microsoft-standard-WSL2", true},
		{"5.15.153.1-microsoft-standard-WSL2", true},
		{"6.10.2-arch1-1", false},
		{"23.6.0", false},
	} {
		if got := isWSLRelease(test.release); got != test.want {
			t.Errorf("isWSLRelease(%q) = %v, want %v", test.release, got, test.want)
		}
	}
}
