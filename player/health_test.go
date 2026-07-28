package player

import (
	"errors"
	"testing"
)

func TestPlaybackEventsNeverBlockWhenConsumerIsSlow(t *testing.T) {
	p := NewAVPlayer()
	for i := 0; i < cap(p.events)+10; i++ {
		p.emit(PlaybackEvent{Type: PlaybackRestarting, Err: errors.New("test")})
	}
	if got := len(p.events); got != cap(p.events) {
		t.Fatalf("buffered events = %d, want %d", got, cap(p.events))
	}
}

func TestPlaybackHealthSnapshot(t *testing.T) {
	p := NewAVPlayer()
	p.counters.framesRendered.Add(12)
	p.counters.framesDropped.Add(3)
	p.counters.errors.Add(2)
	p.counters.restarts.Add(1)

	got := p.Health()
	want := (PlaybackHealth{FramesRendered: 12, FramesDropped: 3, Errors: 2, Restarts: 1})
	if got != want {
		t.Fatalf("Health() = %+v, want %+v", got, want)
	}
}
