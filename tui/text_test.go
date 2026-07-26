package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func TestMarqueeLeavesShortTextAlone(t *testing.T) {
	if got := marquee("short", 20, 7); got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
	// Exactly filling the window is still not overflow.
	if got := marquee("12345", 5, 3); got != "12345" {
		t.Errorf("got %q, want %q", got, "12345")
	}
}

// Every window must be exactly the requested width, or the caption line would
// jitter as it scrolls.
func TestMarqueeWindowIsAlwaysWidthWide(t *testing.T) {
	text := "a long caption about istanbul with plenty of hashtags on the end"
	const width = 20
	for offset := range 200 {
		got := marquee(text, width, offset)
		if w := runewidth.StringWidth(got); w != width {
			t.Fatalf("offset %d: width %d, want %d (%q)", offset, w, width, got)
		}
	}
}

// Scrolling far enough must reveal the tail, which is the whole point: the
// end of an Instagram caption is where the hashtags live.
func TestMarqueeEventuallyShowsTheTail(t *testing.T) {
	text := "start of the caption ... and here is the #tail"
	const width = 20

	for offset := range 200 {
		if strings.Contains(marquee(text, width, offset), "#tail") {
			return
		}
	}
	t.Error("scrolling never revealed the end of the caption")
}

// Offsets advance by display column, so wide characters must not make the
// window skip ahead faster than one column per tick.
func TestMarqueeAdvancesOneColumnPerOffset(t *testing.T) {
	text := "日本語のキャプション with ascii tail 😂😂😂"
	const width = 12

	prev := marquee(text, width, 0)
	for offset := 1; offset < 60; offset++ {
		got := marquee(text, width, offset)
		if got == prev {
			t.Fatalf("offset %d produced the same window as %d: %q", offset, offset-1, got)
		}
		prev = got
	}
}

// The loop is seamless: offset and offset+loopWidth show the same window.
func TestMarqueeLoops(t *testing.T) {
	text := "abcdefghij"
	const width = 4
	loopWidth := runewidth.StringWidth(text) + runewidth.StringWidth(marqueeGap)

	for offset := range loopWidth {
		a := marquee(text, width, offset)
		b := marquee(text, width, offset+loopWidth)
		if a != b {
			t.Errorf("offset %d: %q, offset %d: %q", offset, a, offset+loopWidth, b)
		}
	}
}

func TestRenderWithMentionsColorsNonASCIIHashtags(t *testing.T) {
	// The tag scanner has to keep a non-ASCII hashtag in one piece; splitting
	// it would leave the tail styled as prose. "#yakışıklı" is 10 runes.
	if n := tagAt([]rune("#yakışıklı rest"), 0); n != 10 {
		t.Errorf("tagAt on #yakışıklı: got %d runes, want 10", n)
	}
	if n := tagAt([]rune("@user.name!"), 0); n != 10 {
		t.Errorf("tagAt on @user.name: got %d, want 10", n)
	}
	if n := tagAt([]rune("# spaced"), 0); n != 0 {
		t.Errorf("a lone # is not a tag: got %d", n)
	}
	if n := tagAt([]rune("plain"), 0); n != 0 {
		t.Errorf("plain text is not a tag: got %d", n)
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		when time.Time
		want string
	}{
		{"just now", now.Add(-10 * time.Second), "now"},
		{"minutes", now.Add(-5 * time.Minute), "5m"},
		{"hours", now.Add(-3 * time.Hour), "3h"},
		{"days", now.Add(-50 * time.Hour), "2d"},
		{"weeks", now.Add(-20 * 24 * time.Hour), "2w"},
		{"years", now.Add(-800 * 24 * time.Hour), "2y"},
	}
	for _, tc := range cases {
		if got := relativeTime(tc.when.Unix()); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}

	// A missing or future timestamp renders nothing rather than nonsense.
	if got := relativeTime(0); got != "" {
		t.Errorf("zero timestamp: got %q, want empty", got)
	}
	if got := relativeTime(now.Add(time.Hour).Unix()); got != "" {
		t.Errorf("future timestamp: got %q, want empty", got)
	}
}

func TestSplitCaptionSeparatesProseAndTags(t *testing.T) {
	prose, tags := splitCaption("A clean caption #film #dreamcore with words #night")
	if prose != "A clean caption with words" {
		t.Errorf("prose=%q", prose)
	}
	if tags != "#film #dreamcore #night" {
		t.Errorf("tags=%q", tags)
	}
}

func TestLimitedWrappedLinesMarksTruncation(t *testing.T) {
	lines := limitedWrappedLines("one two three four five six seven", 10, 2)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %#v", len(lines), lines)
	}
	if !strings.HasSuffix(lines[1], "…") {
		t.Errorf("truncated final line has no ellipsis: %q", lines[1])
	}
	for _, line := range lines {
		if runewidth.StringWidth(line) > 10 {
			t.Errorf("line exceeds budget: %q", line)
		}
	}
}

func TestCleanMetadataTextRemovesTerminalBreakingGlyphs(t *testing.T) {
	input := "  🎧\u200d✨  Feels \xff Lim\uFE0F  \uE000 feat.  Artist 💿  "
	got := cleanMetadataText(input)
	if got != "Feels Lim feat. Artist" {
		t.Errorf("cleanMetadataText=%q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Errorf("cleaned metadata still contains replacement rune: %q", got)
	}
}
