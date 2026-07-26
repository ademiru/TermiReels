package player

import (
	"testing"

	"github.com/gopxl/beep/v2"
)

func testAudioPlayer(bufferBytes int) *AudioPlayer {
	a := &AudioPlayer{sampleBuf: make([]byte, bufferBytes)}
	a.clock.Store(float64(0))
	a.volume.Store(float64(1))
	a.streamer = &audioStreamer{
		player:    a,
		format:    beep.Format{SampleRate: beep.SampleRate(AudioSampleRate)},
		buffering: true,
	}
	return a
}

func TestAudioStreamerPrebuffersBeforeAdvancingClock(t *testing.T) {
	a := testAudioPlayer(audioPrebufferBytes - audioBytesPerSample)
	out := make([][2]float64, 128)

	n, ok := a.streamer.Stream(out)
	if !ok || n != len(out) {
		t.Fatalf("Stream() = (%d, %v), want (%d, true)", n, ok, len(out))
	}
	if got := a.Time(); got != 0 {
		t.Fatalf("clock advanced while prebuffering: %f", got)
	}
	if got := len(a.sampleBuf); got != audioPrebufferBytes-audioBytesPerSample {
		t.Fatalf("prebuffer was consumed early: %d bytes remain", got)
	}
}

func TestAudioStreamerResumesAfterPrebufferThreshold(t *testing.T) {
	a := testAudioPlayer(audioPrebufferBytes)
	out := make([][2]float64, 128)

	a.streamer.Stream(out)

	if a.streamer.buffering {
		t.Fatal("streamer stayed in buffering state after reaching threshold")
	}
	if got := a.Time(); got <= 0 {
		t.Fatalf("clock did not advance after playback resumed: %f", got)
	}
	if got := len(a.sampleBuf); got != audioPrebufferBytes-len(out)*audioBytesPerSample {
		t.Fatalf("unexpected remaining buffer: %d", got)
	}
}

func TestAudioStreamerRebuffersOnUnderrun(t *testing.T) {
	a := testAudioPlayer(audioPrebufferBytes)
	a.streamer.buffering = false
	a.sampleBuf = a.sampleBuf[:audioBytesPerSample]
	out := make([][2]float64, 128)

	a.streamer.Stream(out)

	if !a.streamer.buffering {
		t.Fatal("streamer did not re-enter buffering after underrun")
	}
	clockAfterUnderrun := a.Time()
	a.streamer.Stream(out)
	if got := a.Time(); got != clockAfterUnderrun {
		t.Fatalf("clock advanced while rebuilding buffer: got %f want %f", got, clockAfterUnderrun)
	}
}
