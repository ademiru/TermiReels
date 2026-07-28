package player

import "sync/atomic"

type PlaybackEventType int

const (
	PlaybackRestarting PlaybackEventType = iota
	PlaybackFailed
)

// PlaybackEvent reports failures that happen after Play has returned.
type PlaybackEvent struct {
	Type PlaybackEventType
	Err  error
}

// PlaybackHealth is a lock-free diagnostic snapshot.
type PlaybackHealth struct {
	FramesRendered uint64
	FramesDropped  uint64
	Errors         uint64
	Restarts       uint64
}

type playbackCounters struct {
	framesRendered atomic.Uint64
	framesDropped  atomic.Uint64
	errors         atomic.Uint64
	restarts       atomic.Uint64
}
