package player

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"
)

var (
	speakerInitOnce sync.Once
	speakerInitErr  error
)

// ensureSpeaker lazily opens the system audio device. Package initialization
// used to do this before main or any test could run, which made `--shortcut`
// unnecessarily touch audio and could hang indefinitely on headless systems.
func ensureSpeaker() error {
	speakerInitOnce.Do(func() {
		format := beep.Format{SampleRate: beep.SampleRate(AudioSampleRate)}
		speakerInitErr = speaker.Init(format.SampleRate, format.SampleRate.N(50*1000000))
	})
	return speakerInitErr
}

// AudioPlayer decodes and plays audio, providing the master clock
type AudioPlayer struct {
	codecCtx *astiav.CodecContext
	swrCtx   *astiav.SoftwareResampleContext
	frame    *astiav.Frame

	// Clock tracking
	clock   atomic.Value // float64
	clockMu sync.RWMutex
	playing atomic.Bool
	paused  atomic.Bool
	muted   atomic.Bool
	volume  atomic.Value // float64, 0.0–1.0

	// Beep streamer
	streamer *audioStreamer
	ctrl     *beep.Ctrl

	// Sample buffer for decoded audio
	sampleBuf []byte
	buffMu    sync.Mutex

	closed bool
	mu     sync.Mutex
}

// audioStreamer implements beep.Streamer for our decoded audio
type audioStreamer struct {
	player    *AudioPlayer
	buf       []byte
	pos       int
	format    beep.Format
	buffering bool
}

const (
	audioBytesPerSample = 4 // s16le stereo
	// Hold roughly 150 ms before starting (and after an underrun). This absorbs
	// short decoder/renderer scheduling spikes instead of exposing them as
	// repeated clicks and gaps at the speaker.
	audioPrebufferBytes = AudioSampleRate * audioBytesPerSample * 150 / 1000
)

func (s *audioStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	s.player.buffMu.Lock()
	defer s.player.buffMu.Unlock()

	// when paused, fill buff with silence and don't advance clock
	if s.player.paused.Load() {
		for i := range samples {
			samples[i][0] = 0
			samples[i][1] = 0
		}
		return len(samples), true
	}

	if s.buffering {
		if len(s.player.sampleBuf) < audioPrebufferBytes {
			fillSilence(samples)
			return len(samples), true
		}
		s.buffering = false
	}

	muted := s.player.muted.Load()
	volume := s.player.volume.Load().(float64)
	volume = volume * volume // since volume is (0.0 - 1.0), this scales the volume exponentially for human hearing

	// sampleBuf (raw bytes from FFmpeg):
	// ┌────┬────┬────┬────┬────┬────┬────┬────┬─...
	// │ L0 │ L0 │ R0 │ R0 │ L1 │ L1 │ R1 │ R1 │
	// │ lo │ hi │ lo │ hi │ lo │ hi │ lo │ hi │
	// └────┴────┴────┴────┴────┴────┴────┴────┴─...
	// 	b0 	 b1   b2   b3
	//
	// (4 bytes = 1 stereo sample)
	samplesPlayed := 0

	for i := range samples {

		if len(s.player.sampleBuf) < audioBytesPerSample {
			// Do not repeatedly dip in and out of an empty buffer. Re-enter
			// buffering and resume only after a useful cushion has accumulated.
			fillSilence(samples[i:])
			s.buffering = true
			break
		}

		if muted {
			// consume buffer but output silence
			samples[i][0] = 0
			samples[i][1] = 0
		} else {
			// Convert sampleBuf to float64 and normalize
			// to [-1, 1] that beep expects
			//
			// left low byte | left high byte << 8
			// 	  8 bits            8 bits
			const MAX_INT_16 = int16(32767)
			left := int16(s.player.sampleBuf[0]) | int16(s.player.sampleBuf[1])<<8
			right := int16(s.player.sampleBuf[2]) | int16(s.player.sampleBuf[3])<<8
			samples[i][0] = float64(left) / float64(MAX_INT_16) * volume
			samples[i][1] = float64(right) / float64(MAX_INT_16) * volume
		}

		// consume
		s.player.sampleBuf = s.player.sampleBuf[audioBytesPerSample:]
		samplesPlayed++
	}

	// update clock based on actual samples played
	// time elapsed = samples played / audio sample rate
	if samplesPlayed > 0 {
		s.player.clock.Store(s.player.clock.Load().(float64) + float64(samplesPlayed)/float64(AudioSampleRate))
	}

	return len(samples), true
}

func fillSilence(samples [][2]float64) {
	for i := range samples {
		samples[i][0] = 0
		samples[i][1] = 0
	}
}

func (s *audioStreamer) Err() error {
	return nil
}

// NewAudioPlayer creates an audio player from codec parameters
func NewAudioPlayer(codecParams *astiav.CodecParameters) (*AudioPlayer, error) {
	if err := ensureSpeaker(); err != nil {
		return nil, fmt.Errorf("failed to initialize audio output: %w", err)
	}

	a := &AudioPlayer{
		sampleBuf: make([]byte, 0, 192000), // ~1 second buffer
	}
	a.clock.Store(float64(0))

	// Find decoder
	codec := astiav.FindDecoder(codecParams.CodecID())
	if codec == nil {
		return nil, fmt.Errorf("audio codec not found: %s", codecParams.CodecID())
	}

	// Allocate codec context
	a.codecCtx = astiav.AllocCodecContext(codec)
	if a.codecCtx == nil {
		return nil, fmt.Errorf("failed to allocate audio codec context")
	}

	// Copy parameters
	if err := codecParams.ToCodecContext(a.codecCtx); err != nil {
		a.Close()
		return nil, fmt.Errorf("failed to copy audio codec params: %w", err)
	}

	// Open codec
	if err := a.codecCtx.Open(codec, nil); err != nil {
		a.Close()
		return nil, fmt.Errorf("failed to open audio codec: %w", err)
	}

	// Allocate decode frame
	a.frame = astiav.AllocFrame()

	// Setup resampler - we'll configure it on first frame
	a.swrCtx = astiav.AllocSoftwareResampleContext()
	if a.swrCtx == nil {
		a.Close()
		return nil, fmt.Errorf("failed to allocate swr context")
	}

	format := beep.Format{
		SampleRate: beep.SampleRate(AudioSampleRate),
	}

	// Create streamer
	a.streamer = &audioStreamer{
		player:    a,
		format:    format,
		buffering: true,
	}
	a.ctrl = &beep.Ctrl{Streamer: a.streamer}

	return a, nil
}

// Start begins audio playback
func (a *AudioPlayer) Start() {
	a.playing.Store(true)
	speaker.Play(a.ctrl)
}

// DecodePacket decodes an audio packet and queues samples for playback
func (a *AudioPlayer) DecodePacket(pkt *astiav.Packet, pts float64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return fmt.Errorf("audio player closed")
	}

	// Send packet to decoder
	if err := a.codecCtx.SendPacket(pkt); err != nil {
		return fmt.Errorf("failed to send audio packet: %w", err)
	}

	// Receive decoded frames
	for {
		if err := a.codecCtx.ReceiveFrame(a.frame); err != nil {
			if err == astiav.ErrEof || err == astiav.ErrEagain {
				break
			}
			return fmt.Errorf("failed to receive audio frame: %w", err)
		}

		// Create output frame for resampled audio
		outFrame := astiav.AllocFrame()
		outFrame.SetSampleFormat(astiav.SampleFormatS16)
		outFrame.SetSampleRate(AudioSampleRate)
		outFrame.SetChannelLayout(astiav.ChannelLayoutStereo)
		outFrame.SetNbSamples(a.frame.NbSamples())

		// Allocate buffer for output frame
		if err := outFrame.AllocBuffer(0); err != nil {
			a.frame.Unref()
			outFrame.Free()
			continue
		}

		// Resample frame
		if err := a.swrCtx.ConvertFrame(a.frame, outFrame); err != nil {
			a.frame.Unref()
			outFrame.Free()
			// Skip frames that fail to resample instead of erroring
			continue
		}

		// Get resampled data - plane 0 for interleaved S16
		data := outFrame.Data()
		if data != nil {
			// Calculate actual byte size: samples * channels * bytes_per_sample
			numSamples := outFrame.NbSamples()
			byteSize := numSamples * 2 * 2 // 2 channels * 2 bytes per sample (S16)
			plane, err := data.Bytes(0)
			if err == nil && plane != nil && len(plane) >= byteSize {
				a.buffMu.Lock()
				a.sampleBuf = append(a.sampleBuf, plane[:byteSize]...)
				a.buffMu.Unlock()
			}
		}

		a.frame.Unref()
		outFrame.Free()
	}

	return nil
}

// Time returns the current playback time (master clock)
func (a *AudioPlayer) Time() float64 {
	return a.clock.Load().(float64)
}

// SetVolume sets the playback volume (0.0–1.0)
func (a *AudioPlayer) SetVolume(vol float64) {
	vol = min(max(vol, 0), 1)
	a.volume.Store(vol)
}

// Mute toggles mute state
func (a *AudioPlayer) Mute() {
	a.muted.Store(!a.muted.Load())
}

// IsPlaying returns true if audio is playing
func (a *AudioPlayer) IsPlaying() bool {
	return a.playing.Load() && !a.paused.Load()
}

// Pause toggles pause state
func (a *AudioPlayer) Pause() {
	a.paused.Store(!a.paused.Load())
}

// Seek clears buffered audio samples and resets the clock to the given position.
func (a *AudioPlayer) Seek(seconds float64) {
	a.buffMu.Lock()
	a.sampleBuf = a.sampleBuf[:0]
	a.streamer.buffering = true
	a.buffMu.Unlock()

	a.clock.Store(seconds)
}

// Close releases all resources
func (a *AudioPlayer) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}
	a.closed = true

	a.playing.Store(false)
	speaker.Clear()

	if a.frame != nil {
		a.frame.Free()
		a.frame = nil
	}
	if a.swrCtx != nil {
		a.swrCtx.Free()
		a.swrCtx = nil
	}
	if a.codecCtx != nil {
		a.codecCtx.Free()
		a.codecCtx = nil
	}
}
