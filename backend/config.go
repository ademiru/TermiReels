package backend

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// confEntry describes one setting that reels.conf round-trips. read and write
// both walk confEntries, so a key can never be written under one name and
// parsed back under another.
type confEntry struct {
	key string
	// comment is emitted above the key the first time it is appended to a
	// file that doesn't have it yet.
	comment string
	// get renders the setting as the lines to write. Multiple values become
	// repeated "key = value" lines, which is how multi-bind keys work.
	get func(*Settings) []string
	// set parses the values back. vals is never empty.
	set func(*Settings, []string)
	// label names the action in the shortcut editor. Only keybinds carry one,
	// which is also what marks an entry as rebindable.
	label string
}

// boolEntry, intEntry, floatEntry and keyEntry build the four confEntry shapes.
// A malformed value leaves the setting at its default rather than failing the
// whole load.
func boolEntry(key, comment string, field func(*Settings) *bool) confEntry {
	return confEntry{
		key:     key,
		comment: comment,
		get:     func(s *Settings) []string { return []string{strconv.FormatBool(*field(s))} },
		set:     func(s *Settings, vals []string) { *field(s) = vals[len(vals)-1] == "true" },
	}
}

func intEntry(key, comment string, field func(*Settings) *int) confEntry {
	return confEntry{
		key:     key,
		comment: comment,
		get:     func(s *Settings) []string { return []string{strconv.Itoa(*field(s))} },
		set: func(s *Settings, vals []string) {
			if n, err := strconv.Atoi(vals[len(vals)-1]); err == nil {
				*field(s) = n
			}
		},
	}
}

func floatEntry(key, comment string, field func(*Settings) *float64) confEntry {
	return confEntry{
		key:     key,
		comment: comment,
		get:     func(s *Settings) []string { return []string{strconv.FormatFloat(*field(s), 'g', -1, 64)} },
		set: func(s *Settings, vals []string) {
			if n, err := strconv.ParseFloat(vals[len(vals)-1], 64); err == nil {
				*field(s) = n
			}
		},
	}
}

// keyEntry builds a keybind entry. Conf names ("space") and bubbletea key
// strings (" ") are translated in both directions here so the rest of the
// code only ever sees bubbletea strings.
func keyEntry(key, comment, label string, field func(*Settings) *[]string) confEntry {
	return confEntry{
		key:     key,
		comment: comment,
		label:   label,
		get: func(s *Settings) []string {
			binds := *field(s)
			out := make([]string, len(binds))
			for i, b := range binds {
				if name, ok := KeyToConf[b]; ok {
					out[i] = name
				} else {
					out[i] = b
				}
			}
			return out
		},
		set: func(s *Settings, vals []string) {
			binds := make([]string, len(vals))
			for i, v := range vals {
				if resolved, ok := ConfToKey[v]; ok {
					binds[i] = resolved
				} else {
					binds[i] = v
				}
			}
			*field(s) = binds
		},
	}
}

// confEntries is the single source of truth for reels.conf. Order here is the
// order keys are appended to a file that lacks them.
var confEntries = []confEntry{
	boolEntry("show_navbar", "", func(s *Settings) *bool { return &s.ShowNavbar }),
	intEntry("retina_scale", "# 1 on Linux, 2 on macOS. Ignored when reel_fit is on.", func(s *Settings) *int { return &s.RetinaScale }),

	boolEntry("reel_fit", "# scale the reel to the terminal instead of using the fixed size below", func(s *Settings) *bool { return &s.ReelFit }),
	intEntry("reel_width", "# reels are scaled within this bounding box", func(s *Settings) *int { return &s.ReelWidth }),
	intEntry("reel_height", "", func(s *Settings) *int { return &s.ReelHeight }),
	intEntry("reel_size_step", "", func(s *Settings) *int { return &s.ReelSizeStep }),
	floatEntry("volume", "", func(s *Settings) *float64 { return &s.Volume }),
	intEntry("gif_cell_height", "", func(s *Settings) *int { return &s.GifCellHeight }),
	intEntry("panel_shrink_steps", "# how many reel_size_steps to shrink when opening a panel", func(s *Settings) *int { return &s.PanelShrinkSteps }),

	keyEntry("key_next", "# configurable keybinds (repeat a key to bind it more than once)", "next reel", func(s *Settings) *[]string { return &s.KeysNext }),
	keyEntry("key_previous", "", "previous reel", func(s *Settings) *[]string { return &s.KeysPrevious }),
	keyEntry("key_pause", "", "pause / resume", func(s *Settings) *[]string { return &s.KeysPause }),
	keyEntry("key_mute", "", "mute", func(s *Settings) *[]string { return &s.KeysMute }),
	keyEntry("key_like", "", "like", func(s *Settings) *[]string { return &s.KeysLike }),
	keyEntry("key_repost", "", "repost", func(s *Settings) *[]string { return &s.KeysRepost }),
	keyEntry("key_navbar", "", "toggle navbar", func(s *Settings) *[]string { return &s.KeysNavbar }),
	keyEntry("key_vol_up", "", "volume up", func(s *Settings) *[]string { return &s.KeysVolUp }),
	keyEntry("key_vol_down", "", "volume down", func(s *Settings) *[]string { return &s.KeysVolDown }),
	keyEntry("key_reel_size_inc", "", "enlarge reel", func(s *Settings) *[]string { return &s.KeysReelSizeInc }),
	keyEntry("key_reel_size_dec", "", "shrink reel", func(s *Settings) *[]string { return &s.KeysReelSizeDec }),
	keyEntry("key_copy_link", "", "copy link", func(s *Settings) *[]string { return &s.KeysCopyLink }),
	keyEntry("key_save", "", "bookmark", func(s *Settings) *[]string { return &s.KeysSave }),
	keyEntry("key_quit", "", "quit", func(s *Settings) *[]string { return &s.KeysQuit }),
	keyEntry("key_seek_forward", "", "seek forward", func(s *Settings) *[]string { return &s.KeysSeekForward }),
	keyEntry("key_seek_backward", "", "seek backward", func(s *Settings) *[]string { return &s.KeysSeekBackward }),
	keyEntry("key_select", "", "select (share / friends / react / replies)", func(s *Settings) *[]string { return &s.KeysSelect }),
	keyEntry("key_share_open", "", "open share panel", func(s *Settings) *[]string { return &s.KeysShareOpen }),
	keyEntry("key_share_close", "", "send & close share panel", func(s *Settings) *[]string { return &s.KeysShareClose }),
	keyEntry("key_comments_open", "", "open comments", func(s *Settings) *[]string { return &s.KeysCommentsOpen }),
	keyEntry("key_comments_close", "", "close comments", func(s *Settings) *[]string { return &s.KeysCommentsClose }),
	keyEntry("key_help_open", "", "open help", func(s *Settings) *[]string { return &s.KeysHelpOpen }),
	keyEntry("key_help_close", "", "close help", func(s *Settings) *[]string { return &s.KeysHelpClose }),
	keyEntry("key_friends_open", "", "open DM friends", func(s *Settings) *[]string { return &s.KeysChatsOpen }),
	keyEntry("key_friends_close", "", "close DMs / exit chat mode", func(s *Settings) *[]string { return &s.KeysChatsClose }),
	keyEntry("key_react_open", "", "open react panel", func(s *Settings) *[]string { return &s.KeysReactOpen }),
	keyEntry("key_react_close", "", "close react panel", func(s *Settings) *[]string { return &s.KeysReactClose }),
}

const confHeader = "# insta reels TUI config"

// KeyBinding is one rebindable action: the reels.conf key it is stored under,
// a human label, and its current binds as bubbletea key strings.
type KeyBinding struct {
	ConfKey string
	Label   string
	Binds   []string
}

// KeyBindings lists every rebindable action in reels.conf order. It is derived
// from confEntries, so an action added there shows up in the editor without
// anything else to remember.
func KeyBindings(s Settings) []KeyBinding {
	var out []KeyBinding
	for _, e := range confEntries {
		if e.label == "" {
			continue
		}
		binds := make([]string, 0, 2)
		for _, name := range e.get(&s) {
			if resolved, ok := ConfToKey[name]; ok {
				binds = append(binds, resolved)
			} else {
				binds = append(binds, name)
			}
		}
		out = append(out, KeyBinding{ConfKey: e.ConfKey(), Label: e.label, Binds: binds})
	}
	return out
}

// ConfKey exposes the reels.conf name of an entry.
func (e confEntry) ConfKey() string { return e.key }

// SetKeyBinds replaces the binds for one action. Unknown keys are ignored, and
// an empty bind list is refused: an action with no key at all is unreachable
// and there would be no way to bind it back from inside the app.
func SetKeyBinds(s *Settings, confKey string, binds []string) bool {
	if len(binds) == 0 {
		return false
	}
	for _, e := range confEntries {
		if e.key != confKey || e.label == "" {
			continue
		}
		// set expects conf names, which is what the writer round-trips.
		names := make([]string, len(binds))
		for i, b := range binds {
			if name, ok := KeyToConf[b]; ok {
				names[i] = name
			} else {
				names[i] = b
			}
		}
		e.set(s, names)
		return true
	}
	return false
}

// SaveSettings writes s to configDir/reels.conf, preserving the user's
// comments and any keys reels doesn't manage.
func SaveSettings(configDir string, s Settings) error {
	return writeConf(filepath.Join(configDir, "reels.conf"), s)
}

// ConflictsWith returns the labels of the other actions bound to key. Binds are
// context-sensitive in places — select and like share space on purpose — so
// this reports rather than refuses.
func ConflictsWith(s Settings, confKey, key string) []string {
	var out []string
	for _, b := range KeyBindings(s) {
		if b.ConfKey == confKey {
			continue
		}
		for _, bind := range b.Binds {
			if bind == key {
				out = append(out, b.Label)
				break
			}
		}
	}
	return out
}

// DefaultSettings returns the shipped settings, used by the shortcut editor to
// restore an action's original keys.
func DefaultSettings() Settings { return defaultSettings() }

func defaultSettings() Settings {
	s := Settings{
		ShowNavbar:       true,
		RetinaScale:      1,
		ReelFit:          true,
		ReelWidth:        270,
		ReelHeight:       480,
		ReelSizeStep:     30,
		Volume:           1,
		GifCellHeight:    5,
		PanelShrinkSteps: 4,
		KeysNext:         []string{"j"},
		KeysPrevious:     []string{"k"},
		KeysPause:        []string{"p"},
		KeysMute:         []string{"m"},
		KeysLike:         []string{" "},
		KeysRepost:       []string{"r"},
		KeysNavbar:       []string{"e"},
		KeysReelSizeInc:  []string{"="},
		KeysReelSizeDec:  []string{"-"},
		KeysVolUp:        []string{"]"},
		KeysVolDown:      []string{"["},
		KeysQuit:         []string{"q", "ctrl+c"},
		KeysCopyLink:     []string{"y"},
		KeysSave:         []string{"b"},
		KeysSeekForward:  []string{"l"},
		KeysSeekBackward: []string{"h"},
		KeysSelect:       []string{" "},

		KeysShareOpen:  []string{"s"},
		KeysShareClose: []string{"S"},

		KeysCommentsOpen:  []string{"c"},
		KeysCommentsClose: []string{"C"},

		KeysHelpOpen:  []string{"?"},
		KeysHelpClose: []string{"?"},

		KeysChatsOpen:  []string{"d"},
		KeysChatsClose: []string{"D"},

		KeysReactOpen:  []string{"x"},
		KeysReactClose: []string{"X"},
	}

	if goruntime.GOOS == "darwin" {
		s.RetinaScale = 2
	}
	return s
}

// parseConf reads path into a key -> values map. Repeated keys accumulate,
// which is how a single action ends up with several binds.
func parseConf(path string) map[string][]string {
	result := make(map[string][]string)
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			key := strings.TrimSpace(k)
			result[key] = append(result[key], strings.TrimSpace(v))
		}
	}
	return result
}

// parseSettings builds a Settings from path, leaving anything absent or
// malformed at its default.
func parseSettings(path string) Settings {
	s := defaultSettings()
	conf := parseConf(path)
	for _, e := range confEntries {
		if vals, ok := conf[e.key]; ok && len(vals) > 0 {
			e.set(&s, vals)
		}
	}
	normalizeSettings(&s)
	return s
}

// normalizeSettings keeps hand-edited or partially corrupted config values
// inside ranges the renderer and audio backend can safely consume. Hot reload
// makes this especially important: invalid values must not reach a live player.
func normalizeSettings(s *Settings) {
	defaults := defaultSettings()

	s.RetinaScale = min(max(s.RetinaScale, 1), 4)
	s.ReelWidth = min(max(s.ReelWidth, 90), 2160)
	s.ReelHeight = min(max(s.ReelHeight, 160), 3840)
	s.ReelSizeStep = min(max(s.ReelSizeStep, 1), 1000)
	s.Volume = min(max(s.Volume, 0), 1)
	s.GifCellHeight = min(max(s.GifCellHeight, 1), 20)
	s.PanelShrinkSteps = min(max(s.PanelShrinkSteps, 0), 20)

	for _, e := range confEntries {
		if e.label == "" {
			continue
		}
		values := e.get(s)
		filtered := values[:0]
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == 0 {
			e.set(s, e.get(&defaults))
			continue
		}
		e.set(s, filtered)
	}
}

// LoadSettings loads reels.conf from configDir into Config, falling back to
// defaults for anything missing.
func LoadSettings(configDir string) {
	path := filepath.Join(configDir, "reels.conf")
	s := parseSettings(path)

	settingsMu.Lock()
	Config = s
	settingsMu.Unlock()

	rememberConfStat(path)
}

// ReloadSettings re-reads reels.conf and installs it as the live config.
// Returns the new settings and whether anything actually changed.
func ReloadSettings(configDir string) (Settings, bool) {
	path := filepath.Join(configDir, "reels.conf")
	s := parseSettings(path)

	settingsMu.Lock()
	changed := !settingsEqual(Config, s)
	if changed {
		Config = s
	}
	settingsMu.Unlock()

	rememberConfStat(path)
	return s, changed
}

func settingsEqual(a, b Settings) bool {
	if a.ShowNavbar != b.ShowNavbar || a.RetinaScale != b.RetinaScale ||
		a.ReelFit != b.ReelFit || a.ReelWidth != b.ReelWidth || a.ReelHeight != b.ReelHeight ||
		a.ReelSizeStep != b.ReelSizeStep || a.Volume != b.Volume ||
		a.GifCellHeight != b.GifCellHeight || a.PanelShrinkSteps != b.PanelShrinkSteps {
		return false
	}
	for _, e := range confEntries {
		av, bv := e.get(&a), e.get(&b)
		if len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
	}
	return true
}

// Config file watching
//
// The app rewrites reels.conf itself (volume, reel size, navbar), so a naive
// mtime watch would treat every one of its own writes as a user edit and
// reload in a loop. confStat records the file state we last produced or read;
// ConfigFileChanged only reports edits that don't match it.

var confStat struct {
	mu      sync.Mutex
	known   bool
	modTime time.Time
	size    int64
}

func rememberConfStat(path string) {
	confStat.mu.Lock()
	defer confStat.mu.Unlock()
	rememberConfStatLocked(path)
}

func rememberConfStatLocked(path string) {
	info, err := os.Stat(path)
	if err != nil {
		confStat.known = false
		return
	}
	confStat.known = true
	confStat.modTime = info.ModTime()
	confStat.size = info.Size()
}

// ConfigFileChanged reports whether reels.conf was modified by something other
// than this process since the last load or write. It records the new state, so
// a given edit is reported exactly once.
func ConfigFileChanged(configDir string) bool {
	path := filepath.Join(configDir, "reels.conf")

	// Hold the write lock so a write in flight can't be seen half-done as an
	// external edit.
	confWriteMu.Lock()
	defer confWriteMu.Unlock()

	confStat.mu.Lock()
	defer confStat.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !confStat.known {
		rememberConfStatLocked(path)
		return false
	}
	if info.ModTime().Equal(confStat.modTime) && info.Size() == confStat.size {
		return false
	}
	confStat.modTime = info.ModTime()
	confStat.size = info.Size()
	return true
}

// Config file writing

var confWriteMu sync.Mutex

// writeConf updates the managed keys in path to match s, preserving comments,
// blank lines, ordering, and any keys it doesn't know about. The file is
// replaced atomically so a concurrent reader never sees a partial config.
func writeConf(path string, s Settings) error {
	confWriteMu.Lock()
	defer confWriteMu.Unlock()

	lines, existed := readLines(path)

	// Desired value lines per managed key.
	desired := make(map[string][]string, len(confEntries))
	for _, e := range confEntries {
		desired[e.key] = e.get(&s)
	}

	out := make([]string, 0, len(lines)+len(confEntries))
	emitted := make(map[string]bool, len(confEntries))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		k, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			out = append(out, line)
			continue
		}
		key := strings.TrimSpace(k)
		vals, managed := desired[key]
		if !managed {
			out = append(out, line)
			continue
		}
		if emitted[key] {
			// A repeated bind for a key we already rewrote in full. Its value
			// is carried in the block above, so drop the leftover line.
			continue
		}
		emitted[key] = true
		for _, v := range vals {
			out = append(out, fmt.Sprintf("%s = %s", key, v))
		}
	}

	// Append anything the file didn't already have.
	if !existed {
		out = append(out, confHeader, "")
	}
	for _, e := range confEntries {
		if emitted[e.key] {
			continue
		}
		if e.comment != "" {
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
				out = append(out, "")
			}
			out = append(out, e.comment)
		}
		for _, v := range desired[e.key] {
			out = append(out, fmt.Sprintf("%s = %s", e.key, v))
		}
	}

	if err := writeFileAtomic(path, []byte(strings.Join(out, "\n")+"\n")); err != nil {
		return err
	}

	confStat.mu.Lock()
	rememberConfStatLocked(path)
	confStat.mu.Unlock()
	return nil
}

func readLines(path string) (lines []string, existed bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, true
	}
	return strings.Split(text, "\n"), true
}

// writeFileAtomic writes data to a temp file in the same directory and renames
// it over path, so the config is never observed truncated or half-written.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".reels.conf-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// Coalesced background writes
//
// Volume and resize fire a write on every keypress. queueConfWrite collapses a
// burst of those into a single write of the newest settings.

var confQueue struct {
	mu      sync.Mutex
	pending *Settings
	path    string
	running bool
}

// queueConfWrite schedules a write of s to path. If a write is already
// scheduled or in flight, the newest settings win and only one more write
// happens.
func queueConfWrite(path string, s Settings) {
	confQueue.mu.Lock()
	defer confQueue.mu.Unlock()

	confQueue.pending = &s
	confQueue.path = path
	if confQueue.running {
		return
	}
	confQueue.running = true

	go func() {
		for {
			confQueue.mu.Lock()
			next, path := confQueue.pending, confQueue.path
			confQueue.pending = nil
			if next == nil {
				confQueue.running = false
				confQueue.mu.Unlock()
				return
			}
			confQueue.mu.Unlock()

			writeConf(path, *next)
		}
	}()
}
