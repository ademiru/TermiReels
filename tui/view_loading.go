package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ademiru/TermiReels/tui/colors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const loadingBarWidth = 36

var loadingMessages = []string{
	"Preparing your Reels session",
	"Connecting the browser and player",
	"Warming the local media cache",
	"Synchronizing the Reels feed",
}

func (m Model) viewLoading() string {
	if m.width == 0 || m.height == 0 {
		return "\n\n   " + m.spinner.View() + "\n\n"
	}

	// Determine bar text and style
	var barText string
	var barStyle lipgloss.Style
	if m.updateAvailable != "" {
		barText = fmt.Sprintf("Update available: v%s ➞ v%s", m.version, m.updateAvailable)
		barStyle = lipgloss.NewStyle().Bold(true).Foreground(colors.Yellow400Color)
	} else if len(m.loadingMessages) > 0 {
		barText = m.loadingMessages[m.loadingMsgIndex]
		if m.loadingFadeStep > 0 {
			barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(loadingFadeColor(m.loadingFadeStep)))
		} else {
			barStyle = lipgloss.NewStyle().Foreground(colors.Gray400Color)
		}
	}

	return renderLoadingScreen(m.width, m.height, m.version, barText, barStyle, m.loadingMsgScroll)
}

func (m Model) checkVersion() tea.Msg {
	if m.version == "dev" {
		return versionCheckMsg{}
	}
	latest, ok := fetchLatestVersion()
	if !ok || latest == "" || latest == m.version {
		return versionCheckMsg{}
	}
	return versionCheckMsg{latest: latest}
}

func renderLoadingScreen(width, height int, version, status string, statusStyle lipgloss.Style, phase int) string {
	if status == "" {
		status = loadingMessages[0]
		statusStyle = gray400
	}
	if width < 48 || height < 28 {
		wordmark := gradientText("TERMIREELS", brandRamp, lipgloss.NewStyle().Bold(true))
		line := renderLoadingWave(status, statusStyle, phase, max(min(width-4, loadingBarWidth), 8))
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			wordmark+"\n"+gray700.Render("REELS IN YOUR TERMINAL")+"\n\n"+line)
	}

	contentWidth := min(max(width-8, 40), 66)
	title := renderLoadingLogo(contentWidth)
	tagline := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Center).
		Bold(true).
		Foreground(colors.Gray400Color).
		Render("INSTAGRAM REELS  •  BUILT FOR THE TERMINAL")

	runner := renderNyanRunner(contentWidth, phase)
	message := renderLoadingWave(status, statusStyle, phase, contentWidth)

	meta := "LOCAL CHROMIUM  •  KITTY GRAPHICS"
	if version != "" && version != "dev" {
		meta = "v" + version + "  •  " + meta
	}
	metaLine := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Center).
		Foreground(colors.Gray700Color).
		Render(meta)

	flare := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(
		orange400.Render("●") + gray800.Render(" ━━━━━ ") +
			pink400.Render("●") + gray800.Render(" ━━━━━ ") +
			purple400.Render("●") + gray800.Render(" ━━━━━ ") +
			blue400.Render("●"),
	)
	body := strings.Join([]string{flare, "", title, "", tagline, "", runner, message, "", metaLine}, "\n")
	card := lipgloss.NewStyle().
		Width(contentWidth).
		Padding(1, 3).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colors.Purple500Color).
		Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, card)
}

func renderLoadingLogo(width int) string {
	termi := []string{
		"█████ █████ ████  █   █ █████",
		"  █   █     █   █ ██ ██   █  ",
		"  █   ████  ████  █ █ █   █  ",
		"  █   █     █  █  █   █   █  ",
		"  █   █████ █   █ █   █ █████",
	}
	reels := []string{
		"████  █████ █████ █     █████",
		"█   █ █     █     █     █    ",
		"████  ████  ████  █     █████",
		"█  █  █     █     █         █",
		"█   █ █████ █████ █████ █████",
	}
	return render3DLoadingWord(termi, width, brandRamp) + "\n" +
		render3DLoadingWord(reels, width, []rgb{brandRamp[3], brandRamp[2], brandRamp[1], brandRamp[0]})
}

func render3DLoadingWord(lines []string, width int, ramp []rgb) string {
	if len(lines) == 0 {
		return ""
	}
	artWidth := 0
	for _, line := range lines {
		artWidth = max(artWidth, runewidth.StringWidth(line))
	}
	canvasWidth := artWidth + 2
	canvas := make([][]string, len(lines)+1)
	for y := range canvas {
		canvas[y] = make([]string, canvasWidth)
		for x := range canvas[y] {
			canvas[y][x] = " "
		}
	}

	shadow := lipgloss.NewStyle().Foreground(colors.Purple900Color).Bold(true)
	for y, line := range lines {
		for x, r := range []rune(line) {
			if r != ' ' && x+2 < canvasWidth {
				canvas[y+1][x+2] = shadow.Render("▓")
			}
		}
	}
	for y, line := range lines {
		for x, r := range []rune(line) {
			if r == ' ' {
				continue
			}
			t := 0.0
			if artWidth > 1 {
				t = float64(x) / float64(artWidth-1)
			}
			style := lipgloss.NewStyle().Foreground(gradientRamp(ramp, t).color()).Bold(true)
			canvas[y][x] = style.Render(string(r))
		}
	}

	out := make([]string, len(canvas))
	for y, row := range canvas {
		line := strings.Join(row, "")
		out[y] = lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(line)
	}
	return strings.Join(out, "\n")
}

func renderNyanRunner(width, phase int) string {
	if width < 1 {
		return ""
	}
	const (
		tailLength = 26
		catWidth   = 7
	)
	cat := []string{" /\\_/\\ ", "( o.o )", " > ^ < "}
	head := ((phase%(width+tailLength+catWidth))+width+tailLength+catWidth)%
		(width+tailLength+catWidth) - catWidth

	lines := make([]string, len(cat))
	for row, catLine := range cat {
		cells := make([]string, width)
		for i := range cells {
			cells[i] = " "
		}
		tailStart := max(head-tailLength, 0)
		tailEnd := min(head, width)
		for x := tailStart; x < tailEnd; x++ {
			t := float64((x+phase+row*5)%tailLength) / float64(tailLength-1)
			color := gradientRamp(brandRamp, t).color()
			cells[x] = lipgloss.NewStyle().Foreground(color).Bold(true).Render("━")
		}
		for i, r := range catLine {
			x := head + i
			if x < 0 || x >= width {
				continue
			}
			style := gray200.Bold(true)
			if r == 'o' || r == '^' {
				style = pink300.Bold(true)
			}
			cells[x] = style.Render(string(r))
		}
		lines[row] = strings.Join(cells, "")
	}
	return strings.Join(lines, "\n")
}

// renderLoadingWave keeps the status copy physically stable while a bright
// brand-colour crest travels through it from left to right. This produces a
// shimmer without the layout jitter of moving the whole sentence.
func renderLoadingWave(text string, base lipgloss.Style, phase, width int) string {
	if width < 1 {
		return ""
	}
	visible := marquee(text, width, 0)
	textWidth := runewidth.StringWidth(visible)
	if textWidth < width {
		left := (width - textWidth) / 2
		visible = strings.Repeat(" ", left) + visible +
			strings.Repeat(" ", width-left-textWidth)
	}

	const waveRadius = 5
	head := ((phase%(width+2*waveRadius))+width+2*waveRadius)%
		(width+2*waveRadius) - waveRadius
	var b strings.Builder
	col := 0
	for _, r := range visible {
		runeWidth := max(runewidth.RuneWidth(r), 1)
		center := col + runeWidth/2
		distance := center - head
		if distance < 0 {
			distance = -distance
		}
		if distance <= waveRadius {
			t := float64(distance) / float64(waveRadius)
			color := gradientRamp([]rgb{
				brandRamp[1],
				brandRamp[2],
				brandRamp[3],
				brandRamp[0],
			}, t).color()
			style := lipgloss.NewStyle().Foreground(color)
			if distance <= 1 {
				style = style.Bold(true)
			}
			b.WriteString(style.Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
		col += runeWidth
		if col >= width {
			break
		}
	}
	if col < width {
		b.WriteString(strings.Repeat(" ", width-col))
	}
	return b.String()
}

// Loading data & tick functions

func (m Model) fetchLoadingMessages() tea.Msg {
	return loadingMsgsMsg{messages: append([]string(nil), loadingMessages...)}
}

func (m Model) loadingMsgTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return loadingMsgTickMsg{}
	})
}

func (m Model) loadingScrollTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return loadingScrollTickMsg{}
	})
}

func (m Model) loadingFadeTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return loadingFadeTickMsg{}
	})
}

// loadingFadeColor returns the hex color for the current fade step.
// Steps 1-6: fade out (gray400 -> gray800), steps 7-12: fade in (gray800 -> gray400).
func loadingFadeColor(step int) string {
	grays := [7]string{"#A8A8A8", "#949494", "#808080", "#6B6B6B", "#555555", "#363636", "#262626"}
	switch {
	case step <= 0:
		return "#A8A8A8"
	case step <= 6:
		return grays[step]
	case step <= 12:
		return grays[12-step]
	default:
		return "#A8A8A8"
	}
}
