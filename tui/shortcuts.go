package tui

import (
	"fmt"
	"strings"

	"github.com/ademiru/TermiReels/backend"
	"github.com/ademiru/TermiReels/tui/colors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ShortcutsModel is the `reels --shortcut` editor: a list of every rebindable
// action, its current keys, and a capture mode that takes the next keypress.
//
// It runs as its own program with no browser and no player, so it starts
// instantly. Every change is written to reels.conf straight away, which a
// running copy of reels picks up without a restart.
type ShortcutsModel struct {
	configDir string
	settings  backend.Settings
	bindings  []backend.KeyBinding

	cursor int
	scroll int
	height int
	width  int

	// capturing is set while waiting for the keypress that becomes the bind;
	// appending distinguishes replacing the binds from adding to them
	capturing bool
	appending bool

	status      string
	statusStyle lipgloss.Style
	quitting    bool
}

// NewShortcutsModel builds the editor over the config in configDir.
func NewShortcutsModel(configDir string) ShortcutsModel {
	backend.LoadSettings(configDir)
	settings := backend.GetSettings()
	return ShortcutsModel{
		configDir: configDir,
		settings:  settings,
		bindings:  backend.KeyBindings(settings),
	}
}

func (m ShortcutsModel) Init() tea.Cmd { return nil }

func (m ShortcutsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.capturing {
			return m.captureKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey drives the list when not capturing.
func (m ShortcutsModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.cursor = 0
		m.clampScroll()
	case "G", "end":
		m.cursor = len(m.bindings) - 1
		m.clampScroll()

	case "enter":
		m.capturing, m.appending = true, false
		m.status = ""
	case "a":
		m.capturing, m.appending = true, true
		m.status = ""

	case "d":
		return m.dropLastBind()

	case "r":
		return m.resetToDefault()
	}
	return m, nil
}

// captureKey turns the pressed key into a bind for the selected action.
func (m ShortcutsModel) captureKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// escape cancels rather than binding itself, since it's the only way out
	// of capture mode.
	if key == "esc" {
		m.capturing = false
		m.setStatus("cancelled", colors.Gray500Color)
		return m, nil
	}

	m.capturing = false
	b := m.bindings[m.cursor]

	binds := []string{key}
	if m.appending {
		for _, existing := range b.Binds {
			if existing == key {
				m.setStatus(fmt.Sprintf("%s is already bound to %s", displayKey(key), b.Label), colors.Yellow400Color)
				return m, nil
			}
		}
		binds = append(append([]string{}, b.Binds...), key)
	}

	conflicts := backend.ConflictsWith(m.settings, b.ConfKey, key)

	if !backend.SetKeyBinds(&m.settings, b.ConfKey, binds) {
		m.setStatus("could not set that bind", colors.Red500Color)
		return m, nil
	}
	return m, m.persist(conflicts, fmt.Sprintf("%s → %s", b.Label, displayKey(key)))
}

// dropLastBind removes the last key from the selected action, refusing to
// leave it with none.
func (m ShortcutsModel) dropLastBind() (tea.Model, tea.Cmd) {
	b := m.bindings[m.cursor]
	if len(b.Binds) <= 1 {
		m.setStatus("an action needs at least one key", colors.Yellow400Color)
		return m, nil
	}
	binds := b.Binds[:len(b.Binds)-1]
	if !backend.SetKeyBinds(&m.settings, b.ConfKey, binds) {
		m.setStatus("could not remove that bind", colors.Red500Color)
		return m, nil
	}
	return m, m.persist(nil, fmt.Sprintf("%s → %s", b.Label, displayKeys(binds)))
}

// resetToDefault restores the selected action's shipped keys.
func (m ShortcutsModel) resetToDefault() (tea.Model, tea.Cmd) {
	b := m.bindings[m.cursor]
	for _, def := range backend.KeyBindings(backend.DefaultSettings()) {
		if def.ConfKey != b.ConfKey {
			continue
		}
		if !backend.SetKeyBinds(&m.settings, b.ConfKey, def.Binds) {
			break
		}
		return m, m.persist(nil, fmt.Sprintf("%s reset to %s", b.Label, displayKeys(def.Binds)))
	}
	m.setStatus("no default for that action", colors.Red500Color)
	return m, nil
}

// persist saves the config and refreshes the list. Conflicts are reported, not
// blocked: several actions share a key on purpose (select and like are both
// space, and which one fires depends on whether a panel is open).
func (m *ShortcutsModel) persist(conflicts []string, done string) tea.Cmd {
	m.bindings = backend.KeyBindings(m.settings)

	if err := backend.SaveSettings(m.configDir, m.settings); err != nil {
		m.setStatus("could not write reels.conf: "+err.Error(), colors.Red500Color)
		return nil
	}

	if len(conflicts) > 0 {
		m.setStatus(done+"  (also bound to "+strings.Join(conflicts, ", ")+")", colors.Yellow400Color)
	} else {
		m.setStatus(done, colors.Pink300Color)
	}
	return nil
}

func (m *ShortcutsModel) setStatus(text string, c lipgloss.Color) {
	m.status = text
	m.statusStyle = lipgloss.NewStyle().Foreground(c)
}

func (m *ShortcutsModel) moveCursor(delta int) {
	m.cursor = min(max(m.cursor+delta, 0), len(m.bindings)-1)
	m.clampScroll()
}

func (m *ShortcutsModel) clampScroll() {
	visible := m.visibleRows()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if visible > 0 && m.cursor >= m.scroll+visible {
		m.scroll = m.cursor - visible + 1
	}
	m.scroll = max(m.scroll, 0)
}

// visibleRows is how many actions fit between the header and the footer.
func (m ShortcutsModel) visibleRows() int {
	return max(m.height-6, 1)
}

func (m ShortcutsModel) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var b strings.Builder

	b.WriteString("  " + pink400.Bold(true).Render("Reels shortcuts"))
	b.WriteString(gray600.Render("   " + m.configDir + "/reels.conf"))
	b.WriteString("\n\n")

	visible := m.visibleRows()
	labelWidth := 0
	for _, bind := range m.bindings {
		labelWidth = max(labelWidth, lipgloss.Width(bind.Label))
	}

	for i := m.scroll; i < len(m.bindings) && i-m.scroll < visible; i++ {
		bind := m.bindings[i]
		atCursor := i == m.cursor

		bar := "  "
		label := gray300.Render(bind.Label)
		if atCursor {
			bar = pink500.Render("▌ ")
			label = pink50.Bold(true).Render(bind.Label)
		}

		pad := strings.Repeat(" ", max(labelWidth-lipgloss.Width(bind.Label), 0))

		keys := yellow300.Render(displayKeys(bind.Binds))
		if atCursor && m.capturing {
			verb := "press a key to bind"
			if m.appending {
				verb = "press a key to add"
			}
			keys = purple300.Bold(true).Render(verb + "  (esc cancels)")
		}

		b.WriteString("  " + bar + label + pad + "   " + keys + "\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString("  " + m.statusStyle.Render(m.status) + "\n")
	} else {
		b.WriteString("  " + gray700.Render("changes save immediately; a running reels picks them up") + "\n")
	}

	b.WriteString("  " + strings.Join([]string{
		navHintRaw("enter", "rebind"),
		navHintRaw("a", "add key"),
		navHintRaw("d", "drop key"),
		navHintRaw("r", "reset"),
		navHintRaw("j/k", "move"),
		navHintRaw("q", "done"),
	}, "  "))

	return b.String()
}

// navHintRaw is navHint for a literal key name rather than a bind list.
func navHintRaw(key, label string) string {
	return yellow300.Render(key) + gray600.Render(":"+label)
}

// displayKey formats one bubbletea key string the way reels.conf writes it.
func displayKey(key string) string {
	if name, ok := backend.KeyToConf[key]; ok {
		return name
	}
	return key
}
