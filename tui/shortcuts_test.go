package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ademiru/TermiReels/backend"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestEditor(t *testing.T) (ShortcutsModel, string) {
	t.Helper()
	dir := t.TempDir()
	if err := backend.SaveSettings(dir, backend.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	m := NewShortcutsModel(dir)
	m.width, m.height = 100, 40
	return m, dir
}

func press(m ShortcutsModel, keys ...string) ShortcutsModel {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		updated, _ := m.Update(msg)
		m = updated.(ShortcutsModel)
	}
	return m
}

// bindsFor reads an action's keys back out of the config on disk, which is
// what the running app would load.
func bindsFor(t *testing.T, dir, confKey string) []string {
	t.Helper()
	backend.LoadSettings(dir)
	for _, b := range backend.KeyBindings(backend.GetSettings()) {
		if b.ConfKey == confKey {
			return b.Binds
		}
	}
	t.Fatalf("no binding for %s", confKey)
	return nil
}

// The editor lists every rebindable action, derived from the same table the
// config writer uses.
func TestEditorListsEveryKeybind(t *testing.T) {
	m, _ := newTestEditor(t)

	if len(m.bindings) == 0 {
		t.Fatal("editor listed no bindings")
	}
	for _, b := range m.bindings {
		if b.Label == "" {
			t.Errorf("%s has no label", b.ConfKey)
		}
		if len(b.Binds) == 0 {
			t.Errorf("%s has no keys", b.ConfKey)
		}
		if !strings.HasPrefix(b.ConfKey, "key_") {
			t.Errorf("%q is not a keybind", b.ConfKey)
		}
	}
}

// Rebinding has to reach reels.conf, or the whole editor is decoration.
func TestRebindWritesToConfig(t *testing.T) {
	m, dir := newTestEditor(t)

	// The first action is key_next, bound to j by default.
	if got := m.bindings[0].ConfKey; got != "key_next" {
		t.Fatalf("first action is %s, expected key_next", got)
	}

	m = press(m, "enter", "n")

	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != "n" {
		t.Errorf("after rebinding to n, config has %v", got)
	}

	// Adding a key keeps the existing one.
	m = press(m, "a", "N")
	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != "n,N" {
		t.Errorf("after adding N, config has %v", got)
	}

	// Dropping removes only the last.
	m = press(m, "d")
	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != "n" {
		t.Errorf("after dropping, config has %v", got)
	}

	// Reset restores the shipped key.
	press(m, "r")
	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != "j" {
		t.Errorf("after reset, config has %v", got)
	}
}

// Escape has to cancel capture rather than binding itself, since it's the only
// way out of capture mode.
func TestEscapeCancelsCapture(t *testing.T) {
	m, dir := newTestEditor(t)

	m = press(m, "enter", "esc")
	if m.capturing {
		t.Error("still capturing after escape")
	}
	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != "j" {
		t.Errorf("escape changed the binding to %v", got)
	}
}

// An action left with no key would be unreachable, and there'd be no way to
// bind it back from inside the app.
func TestCannotDropTheLastKey(t *testing.T) {
	m, dir := newTestEditor(t)

	m = press(m, "d") // key_next ships with only j
	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != "j" {
		t.Errorf("dropping the only key changed the binding to %v", got)
	}
	if !strings.Contains(m.status, "at least one key") {
		t.Errorf("no explanation shown, status is %q", m.status)
	}
}

// Sharing a key is legal — space is both like and select — so a clash is
// reported rather than refused.
func TestConflictIsReportedNotBlocked(t *testing.T) {
	m, dir := newTestEditor(t)

	// Bind "next reel" to space, which like and select already use.
	m = press(m, "enter", " ")

	if got := bindsFor(t, dir, "key_next"); strings.Join(got, ",") != " " {
		t.Fatalf("the bind was refused, config has %v", got)
	}
	if !strings.Contains(m.status, "also bound to") {
		t.Errorf("no conflict reported, status is %q", m.status)
	}
}

// The editor writes the same file format the app reads, comments and all.
func TestEditorPreservesUserComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reels.conf")
	if err := os.WriteFile(path, []byte("# my notes\nkey_next = j\nmy_own_key = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewShortcutsModel(dir)
	m.width, m.height = 100, 40
	press(m, "enter", "n")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, keep := range []string{"# my notes", "my_own_key = 1"} {
		if !strings.Contains(out, keep) {
			t.Errorf("editor dropped %q\n---\n%s", keep, out)
		}
	}
	if !strings.Contains(out, "key_next = n") {
		t.Errorf("editor did not write the new bind\n---\n%s", out)
	}
}
