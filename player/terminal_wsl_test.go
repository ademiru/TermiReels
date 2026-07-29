package player

import "testing"

func TestWSLFitTransportCapPreservesAspect(t *testing.T) {
	width, height := limitFitTransportSize(1080, 1920, 9, 16, true)
	if height != wslFitMaxHeightPx || width != wslFitMaxHeightPx*9/16 {
		t.Fatalf("WSL fit cap = %dx%d", width, height)
	}
}

func TestNativeFitTransportIsUnchanged(t *testing.T) {
	width, height := limitFitTransportSize(1080, 1920, 9, 16, false)
	if width != 1080 || height != 1920 {
		t.Fatalf("native fit size changed to %dx%d", width, height)
	}
}
