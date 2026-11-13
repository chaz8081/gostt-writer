// Package speech is the device's voice and hearing: a speech recogniser, a
// text-to-speech voice with a clip cache, and the logic that decides when
// the player has finished a reply.
package speech

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
)

// Recognizer turns audio into words as it arrives.
type Recognizer interface {
	Accept(samples []float32)
	Text() string   // the words so far
	Finish() string // flush and return the final words
	Reset()         // start a new utterance
}

// Listener hears one reply at a time from a stream of microphone blocks.
type Listener struct {
	Rec Recognizer
	Mic <-chan []float32
	// Level is the loudness (RMS, 0 to 1) above which a block counts as
	// speech. The listener raises it by itself in a noisy room.
	Level float64
	// Quiet is how long the player must be silent before the reply is
	// taken as finished.
	Quiet time.Duration
	// MaxSpeech ends a reply that goes on this long.
	MaxSpeech time.Duration
	// Skip is ignored at the start, while the speaker is still ringing.
	Skip time.Duration
	// Enough, if set, is called with the words so far once a reply has gone
	// on for a while without a pause; returning true ends it. This keeps
	// the game moving when the room never goes quiet.
	Enough func(text string) bool

	floor    float64 // loudness of the room's background noise
	floorSet bool
}

// enoughAfter is how long a reply runs before Enough is consulted. Sooner,
// and "not sure" would be cut off at "not".
const enoughAfter = 1500 * time.Millisecond

// leadIn is how many blocks from before the first loud one are kept.
const leadIn = 3

// Hear listens for one reply. It returns "" if nothing was said within
// wait.
func (l *Listener) Hear(ctx context.Context, wait time.Duration) (string, error) {
	l.drain()
	l.Rec.Reset()
	var heard, sinceLoud, speech time.Duration
	var before [][]float32 // the last moments before speech, so its first sound is not lost
	started := false
	for {
		var block []float32
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case b, ok := <-l.Mic:
			if !ok {
				return "", errors.New("speech: the microphone stopped")
			}
			block = b
		}
		d := time.Duration(len(block)) * time.Second / audio.SampleRate
		heard += d
		if heard <= l.Skip {
			continue
		}
		switch loud := l.loud(block); {
		case loud && !started:
			started = true
			for _, b := range before {
				l.Rec.Accept(b)
			}
			before = nil
			sinceLoud = 0
		case loud:
			sinceLoud = 0
		default:
			sinceLoud += d
		}
		if !started {
			// Only speech reaches the model; fed minutes of room noise, it
			// would start to hear words in it.
			if before = append(before, block); len(before) > leadIn {
				before = before[1:]
			}
			if heard >= l.Skip+wait {
				return "", nil
			}
			continue
		}
		l.Rec.Accept(block)
		speech += d
		switch {
		case sinceLoud >= l.Quiet, speech >= l.MaxSpeech:
			return l.Rec.Finish(), nil
		case speech >= enoughAfter && l.Enough != nil:
			if text := l.Rec.Text(); text != "" && l.Enough(text) {
				return l.Rec.Finish(), nil
			}
		}
	}
}

// loud reports whether a block is speech rather than room noise, and keeps
// track of the room noise.
func (l *Listener) loud(block []float32) bool {
	var sum float64
	for _, s := range block {
		sum += float64(s) * float64(s)
	}
	rms := math.Sqrt(sum / float64(max(1, len(block))))
	switch {
	case !l.floorSet:
		// A reply may start at once, so the first block is not trusted to
		// be room noise beyond what would still let speech through.
		l.floor, l.floorSet = min(rms, l.Level/3), true
	case rms < l.floor:
		l.floor = rms
	default:
		// Creep up, so a room that gets noisier is followed.
		l.floor = max(l.floor*1.02, 1e-5)
	}
	return rms >= max(l.Level, 3*l.floor)
}

// drain throws away audio captured while the device was not listening.
func (l *Listener) drain() {
	for {
		select {
		case _, ok := <-l.Mic:
			if !ok {
				return
			}
		default:
			return
		}
	}
}
