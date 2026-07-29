package tui

import "sync"

// playbackGate serializes the final hand-off to the player. Downloads may
// finish out of order, especially when switching between the main and creator
// feeds where numeric indexes overlap. Only the newest request is allowed to
// replace the video currently owned by the UI.
type playbackGate struct {
	mu         sync.Mutex
	generation uint64
}

func (g *playbackGate) next() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.generation++
	return g.generation
}

func (g *playbackGate) invalidate() {
	g.mu.Lock()
	g.generation++
	g.mu.Unlock()
}

func (g *playbackGate) isCurrent(generation uint64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.generation == generation
}

func (g *playbackGate) commit(generation uint64, play func() error) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.generation != generation {
		return false, nil
	}
	return true, play()
}
