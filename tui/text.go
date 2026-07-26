package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// cleanMetadataText makes third-party labels safe for a terminal UI.
//
// Instagram music titles frequently contain decorative emoji chains, ZWJ /
// variation selectors, private-use glyphs and occasionally invalid UTF-8.
// Terminal fonts render those as a long run of replacement diamonds. Captions
// keep their full expression; this stricter path is only for compact metadata
// where one broken glyph run can destroy the whole layout.
func cleanMetadataText(text string) string {
	text = strings.ToValidUTF8(text, "")
	var b strings.Builder
	lastSpace := true
	for _, r := range text {
		switch {
		case r == utf8.RuneError:
			continue
		case r >= 0xFE00 && r <= 0xFE0F:
			continue
		case r >= 0xE0100 && r <= 0xE01EF:
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r):
			continue
		case unicode.IsSymbol(r):
			// Drop emoji/decorative symbol runs. The UI supplies its own stable
			// music icon, so these add risk without carrying essential text.
			continue
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		default:
			b.WriteRune(r)
			lastSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// isMentionChar reports whether r can appear in an @ mention handle or a
// #hashtag. Hashtags allow any letter, so that non-ASCII tags stay in one
// piece rather than losing their tail to the base style.
func isMentionChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}

// tagAt returns the length in runes of the @mention or #hashtag starting at
// runes[i], or 0 if there isn't one.
func tagAt(runes []rune, i int) int {
	if runes[i] != '@' && runes[i] != '#' {
		return 0
	}
	j := i + 1
	for j < len(runes) && isMentionChar(runes[j]) {
		j++
	}
	if j == i+1 {
		return 0
	}
	return j - i
}

// renderWithMentions renders text with @mentions in blue and #hashtags in
// pink, and the remainder in base. Captions are mostly tags, so leaving them
// the same colour as the surrounding prose wastes the only structure they have.
func renderWithMentions(text string, base lipgloss.Style) string {
	var b strings.Builder
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		if n := tagAt(runes, i); n > 0 {
			style := blue400
			if runes[i] == '#' {
				style = pink300
			}
			b.WriteString(style.Render(string(runes[i : i+n])))
			i += n
			continue
		}
		start := i
		for i < len(runes) && tagAt(runes, i) == 0 {
			i++
		}
		b.WriteString(base.Render(string(runes[start:i])))
	}
	return b.String()
}

// relativeTime renders a unix timestamp as a compact age like "2h" or "3d".
// Returns "" for a zero or future timestamp, so callers can leave it out.
func relativeTime(unix int64) string {
	if unix <= 0 {
		return ""
	}
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < 0:
		return ""
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// marqueeGap separates the end of a looping marquee from its restart.
const marqueeGap = "     "

// marquee returns the width-column window of text that starts offset columns
// into an endlessly looping "text + gap + text". Text that already fits is
// returned unchanged.
//
// Offsets are counted in display columns rather than runes, so captions full
// of emoji and CJK scroll at a steady speed instead of lurching.
func marquee(text string, width, offset int) string {
	if width <= 0 {
		return ""
	}
	textWidth := runewidth.StringWidth(text)
	if textWidth <= width {
		return text
	}

	loopWidth := textWidth + runewidth.StringWidth(marqueeGap)
	offset = ((offset % loopWidth) + loopWidth) % loopWidth

	scroll := []rune(text + marqueeGap + text)
	start, cols := 0, 0
	for i, r := range scroll {
		if cols >= offset {
			start = i
			break
		}
		cols += runewidth.RuneWidth(r)
	}

	// A cell can't hold half a double-width glyph, so when the offset lands
	// inside one, stand in for its clipped half with a space. Without this the
	// window would hold still for a tick and then jump two columns.
	lead := max(cols-offset, 0)
	visible := strings.Repeat(" ", lead) + truncateByWidth(string(scroll[start:]), max(width-lead, 0))

	// Near the loop boundary the tail is shorter than the window.
	if w := runewidth.StringWidth(visible); w < width {
		visible += strings.Repeat(" ", width-w)
	}
	return visible
}

// isBreakable returns true if the rune can be broken before or after
// without needing a space (CJK ideographs, fullwidth chars, emoji, etc).
func isBreakable(r rune) bool {
	return unicode.In(r,
		unicode.Han,
		unicode.Hangul,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Yi,
	) || runewidth.RuneWidth(r) == 2
}

// wrapByWidth wraps text to fit within maxWidth display columns,
// preferring word boundaries and treating CJK/fullwidth chars as individually breakable.
func wrapByWidth(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return nil
	}

	words := splitWords(text)
	var lines []string
	var currentLine strings.Builder
	currentWidth := 0

	for _, word := range words {
		wordWidth := runewidth.StringWidth(word)

		// Word fits on current line, do nothing
		if currentWidth+wordWidth <= maxWidth {
			currentLine.WriteString(word)
			currentWidth += wordWidth
			continue
		}

		// Word doesn't fit
		// flush current line if non-empty
		if currentWidth > 0 {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentWidth = 0

			// Skip leading space on new line
			if word == " " {
				continue
			}
		}

		// Word itself exceeds maxWidth
		// fall back and break it character by character
		if wordWidth > maxWidth {
			for _, r := range word {
				rw := runewidth.RuneWidth(r)
				if currentWidth+rw > maxWidth {
					lines = append(lines, currentLine.String())
					currentLine.Reset()
					currentWidth = 0
				}
				currentLine.WriteRune(r)
				currentWidth += rw
			}
			continue
		}

		currentLine.WriteString(word)
		currentWidth += wordWidth
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}
	return lines
}

// splitWords splits text into tokens: spaces, breakable characters (each as its own token),
// and runs of non-breakable non-space characters.
func splitWords(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if r == ' ' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, " ")
		} else if isBreakable(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

// truncateByWidth truncates text to fit within maxWidth display columns.
func truncateByWidth(text string, maxWidth int) string {
	var result strings.Builder
	currentWidth := 0

	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if currentWidth+rw > maxWidth {
			break
		}
		result.WriteRune(r)
		currentWidth += rw
	}
	return result.String()
}
