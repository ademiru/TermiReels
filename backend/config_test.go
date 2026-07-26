package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every key writeConf emits must be a key parseSettings reads back. This is
// what caught panel_shrink being written while panel_shrink_steps was read.
func TestConfKeysRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reels.conf")

	want := defaultSettings()
	want.PanelShrinkSteps = 7
	want.ReelWidth = 333
	want.Volume = 0.4
	want.ShowNavbar = false
	want.KeysNext = []string{"J", "down"}
	want.KeysLike = []string{" "}

	if err := writeConf(path, want); err != nil {
		t.Fatalf("writeConf: %v", err)
	}

	got := parseSettings(path)
	if !settingsEqual(want, got) {
		t.Errorf("settings did not round-trip through %s", path)
		for _, e := range confEntries {
			a, b := e.get(&want), e.get(&got)
			if strings.Join(a, ",") != strings.Join(b, ",") {
				t.Errorf("  %s: wrote %v, read back %v", e.key, a, b)
			}
		}
	}
}

// A rewrite must not eat the user's comments, blank lines, or keys reels
// doesn't know about.
func TestWriteConfPreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reels.conf")

	original := strings.Join([]string{
		"# my hand-tuned config",
		"",
		"# louder than default",
		"volume = 0.5",
		"future_option = 42",
		"key_next = j",
		"key_next = down",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	s := parseSettings(path)
	if s.Volume != 0.5 {
		t.Fatalf("volume: got %v, want 0.5", s.Volume)
	}
	if strings.Join(s.KeysNext, ",") != "j,down" {
		t.Fatalf("key_next: got %v, want [j down]", s.KeysNext)
	}

	s.Volume = 0.8
	if err := writeConf(path, s); err != nil {
		t.Fatalf("writeConf: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)

	for _, keep := range []string{
		"# my hand-tuned config",
		"# louder than default",
		"future_option = 42",
	} {
		if !strings.Contains(out, keep) {
			t.Errorf("rewrite dropped %q\n---\n%s", keep, out)
		}
	}
	if !strings.Contains(out, "volume = 0.8") {
		t.Errorf("rewrite did not update volume\n---\n%s", out)
	}
	if strings.Count(out, "key_next = ") != 2 {
		t.Errorf("multi-bind key_next not preserved exactly once each\n---\n%s", out)
	}

	if again := parseSettings(path); again.Volume != 0.8 {
		t.Errorf("volume after rewrite: got %v, want 0.8", again.Volume)
	}
}

// The watcher must ignore the app's own writes, or hot-reload would fire in a
// loop every time the volume changes.
func TestConfigFileChangedIgnoresOwnWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reels.conf")

	s := defaultSettings()
	if err := writeConf(path, s); err != nil {
		t.Fatal(err)
	}
	LoadSettings(dir)

	if ConfigFileChanged(dir) {
		t.Error("reported a change right after loading")
	}

	s.Volume = 0.3
	if err := writeConf(path, s); err != nil {
		t.Fatal(err)
	}
	if ConfigFileChanged(dir) {
		t.Error("reported our own write as an external edit")
	}

	// An outside edit must be reported, and only once.
	edited := "# edited by hand\nvolume = 0.9\n"
	if err := os.WriteFile(path, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	if !ConfigFileChanged(dir) {
		t.Error("missed an external edit")
	}
	if ConfigFileChanged(dir) {
		t.Error("reported the same external edit twice")
	}
}

func TestParseSettingsNormalizesUnsafeLiveValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reels.conf")
	unsafe := strings.Join([]string{
		"retina_scale = 0",
		"reel_width = -500",
		"reel_height = 999999",
		"reel_size_step = 0",
		"volume = 4.2",
		"gif_cell_height = -1",
		"panel_shrink_steps = -8",
		"key_quit = ",
	}, "\n")
	if err := os.WriteFile(path, []byte(unsafe), 0644); err != nil {
		t.Fatal(err)
	}

	got := parseSettings(path)
	if got.RetinaScale != 1 || got.ReelWidth != 90 || got.ReelHeight != 3840 {
		t.Errorf("unsafe dimensions were not normalized: %+v", got)
	}
	if got.ReelSizeStep != 1 || got.Volume != 1 || got.GifCellHeight != 1 || got.PanelShrinkSteps != 0 {
		t.Errorf("unsafe runtime settings were not normalized: %+v", got)
	}
	if strings.Join(got.KeysQuit, ",") != "q,ctrl+c" {
		t.Errorf("empty key binding did not fall back to defaults: %v", got.KeysQuit)
	}
}
