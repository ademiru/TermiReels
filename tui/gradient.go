package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// rgb is a 24-bit colour the gradient helpers interpolate between.
type rgb struct{ r, g, b uint8 }

func (c rgb) color() lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", c.r, c.g, c.b))
}

// lerp mixes two colours, t running 0 to 1.
func lerp(from, to rgb, t float64) rgb {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	mix := func(a, b uint8) uint8 {
		return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
	}
	return rgb{mix(from.r, to.r), mix(from.g, to.g), mix(from.b, to.b)}
}

// gradientRamp spreads stops evenly across the ramp and samples it at t.
func gradientRamp(stops []rgb, t float64) rgb {
	if len(stops) == 0 {
		return rgb{}
	}
	if len(stops) == 1 {
		return stops[0]
	}
	if t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}

	span := 1.0 / float64(len(stops)-1)
	i := int(t / span)
	if i >= len(stops)-1 {
		i = len(stops) - 2
	}
	return lerp(stops[i], stops[i+1], (t-float64(i)*span)/span)
}

// gradientText colours each character of s along the ramp, so a label reads as
// one object with a sweep across it rather than a run of separate colours.
//
// base carries any styling that isn't colour — bold, background — and is
// re-rendered per character with the sampled foreground.
func gradientText(s string, stops []rgb, base lipgloss.Style) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) == 1 {
		return base.Foreground(gradientRamp(stops, 0).color()).Render(s)
	}

	var b strings.Builder
	for i, r := range runes {
		t := float64(i) / float64(len(runes)-1)
		b.WriteString(base.Foreground(gradientRamp(stops, t).color()).Render(string(r)))
	}
	return b.String()
}

// gradientOnBackground is gradientText with the ramp applied to the background
// instead, for a filled button. The foreground is held constant so the label
// stays legible across the sweep.
func gradientOnBackground(s string, stops []rgb, fg lipgloss.Color, bold bool) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}

	var b strings.Builder
	for i, r := range runes {
		t := 0.0
		if len(runes) > 1 {
			t = float64(i) / float64(len(runes)-1)
		}
		style := lipgloss.NewStyle().
			Foreground(fg).
			Background(gradientRamp(stops, t).color()).
			Bold(bold)
		b.WriteString(style.Render(string(r)))
	}
	return b.String()
}

// Instagram's own sweep, used for the send button and other accents that
// should feel like the app's signature rather than a flat block of colour.
var brandRamp = []rgb{
	{0xF5, 0x85, 0x29}, // orange
	{0xDD, 0x2A, 0x7B}, // pink
	{0x89, 0x34, 0xB6}, // purple
	{0x51, 0x5B, 0xD4}, // indigo
}
