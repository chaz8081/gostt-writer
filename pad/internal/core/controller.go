package core

import (
	"log/slog"
	"time"
)

// SampleRate is the audio rate the pad works in, in samples per second.
const SampleRate = 16000

// Recognizer is a streaming speech recogniser.
type Recognizer interface {
	// Accept feeds audio (16 kHz mono) and decodes what is ready.
	Accept(samples []float32)
	// Text returns the hypothesis for everything heard since the last Reset.
	Text() string
	// Finish flushes the stream as if the speaker had stopped and returns
	// the final hypothesis. Reset must be called before further audio.
	Finish() string
	// Reset starts a new stream.
	Reset()
}

// Key identifies a button on the pad.
type Key int

// The pad's buttons.
const (
	KeyTalk Key = iota
	KeyUndo
	KeyDeleteWord
	KeyNewLine
	KeyCancel
)

// String returns the key's name as used in configuration and logs.
func (k Key) String() string {
	switch k {
	case KeyTalk:
		return "talk"
	case KeyUndo:
		return "undo"
	case KeyDeleteWord:
		return "delete_word"
	case KeyNewLine:
		return "new_line"
	case KeyCancel:
		return "cancel"
	}
	return "unknown"
}

// KeyEvent is a button going down or up.
type KeyEvent struct {
	Key  Key
	Down bool
	At   time.Time
}

// Options tune the controller. Zero values select the defaults.
type Options struct {
	HoldThreshold time.Duration // talk key: shortest press that is a hold
	Settle        time.Duration // hypothesis unchanged this long: type its last word
	PhraseGap     time.Duration // hypothesis unchanged this long: the phrase ends
	IdleReset     time.Duration // nothing new for this long: restart the stream
	PreRoll       time.Duration // audio from before the talk key that is kept
	Capitalize    bool          // capitalise the start of sentences
}

// Defaults for Options.
const (
	DefaultSettle    = 1000 * time.Millisecond
	DefaultPhraseGap = 2000 * time.Millisecond
	DefaultIdleReset = 10 * time.Second
	DefaultPreRoll   = 300 * time.Millisecond
)

// Controller connects the buttons and the microphone to the keyboard: it
// starts and stops dictation, types words as they become stable, and applies
// the editing keys. It is not safe for concurrent use; call it from one
// goroutine.
type Controller struct {
	rec     Recognizer
	out     func([]Report)
	onState func(listening bool)
	log     *slog.Logger

	talk    TalkKey
	typist  Typist
	commit  committer
	idle    time.Duration
	preRoll []float32
	preMax  int
}

// NewController returns a controller that reads from rec and hands keyboard
// reports to out. onState, if not nil, is told when dictation starts and
// stops. log may be nil.
func NewController(rec Recognizer, out func([]Report), onState func(bool), log *slog.Logger, o Options) *Controller {
	or := func(v, d time.Duration) time.Duration {
		if v == 0 {
			return d
		}
		return v
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if onState == nil {
		onState = func(bool) {}
	}
	return &Controller{
		rec:     rec,
		out:     out,
		onState: onState,
		log:     log,
		talk:    TalkKey{HoldThreshold: o.HoldThreshold},
		typist:  Typist{Capitalize: o.Capitalize},
		commit:  committer{settle: or(o.Settle, DefaultSettle), phraseGap: or(o.PhraseGap, DefaultPhraseGap)},
		idle:    or(o.IdleReset, DefaultIdleReset),
		preMax:  int(or(o.PreRoll, DefaultPreRoll) * SampleRate / time.Second),
	}
}

// Listening reports whether dictation is running.
func (c *Controller) Listening() bool { return c.talk.Active() }

// HandleKey applies a button event.
func (c *Controller) HandleKey(ev KeyEvent) {
	if ev.Key == KeyTalk {
		var te TalkEvent
		if ev.Down {
			te = c.talk.Press(ev.At)
		} else {
			te = c.talk.Release(ev.At)
		}
		switch te {
		case TalkStart:
			c.start()
		case TalkStop:
			c.stop()
		}
		return
	}
	if !ev.Down {
		return
	}
	c.log.Debug("key", "key", ev.Key.String())
	switch ev.Key {
	case KeyUndo:
		c.send(c.typist.UndoPhrase())
	case KeyDeleteWord:
		c.send(c.typist.DeleteWord())
	case KeyNewLine:
		c.send(c.typist.NewLine())
	case KeyCancel:
		if c.talk.Active() {
			c.talk.Cancel()
			c.rec.Reset()
			c.commit.reset()
			c.typist.EndPhrase()
			c.log.Info("dictation cancelled")
			c.onState(false)
		}
	}
}

// HandleAudio takes the next block of microphone audio (16 kHz mono).
func (c *Controller) HandleAudio(samples []float32) {
	if !c.talk.Active() {
		c.keepPreRoll(samples)
		return
	}
	c.rec.Accept(samples)
	dur := time.Duration(len(samples)) * time.Second / SampleRate
	delta, endPhrase := c.commit.advance(c.rec.Text(), dur)
	c.typeText(delta)
	if endPhrase {
		c.typist.EndPhrase()
	}
	if c.commit.idle() >= c.idle {
		// A long silence with everything typed: start a fresh stream so a
		// session left on does not grow without bound.
		c.rec.Reset()
		c.commit.reset()
		c.typist.EndPhrase()
	}
}

func (c *Controller) start() {
	c.rec.Reset()
	c.commit.reset()
	if len(c.preRoll) > 0 {
		c.rec.Accept(c.preRoll)
		c.preRoll = c.preRoll[:0]
	}
	c.log.Info("dictation started")
	c.onState(true)
}

func (c *Controller) stop() {
	c.typeText(c.commit.finish(c.rec.Finish()))
	c.typist.EndPhrase()
	c.rec.Reset()
	c.log.Info("dictation stopped")
	c.onState(false)
}

func (c *Controller) typeText(text string) {
	if text == "" {
		return
	}
	c.log.Debug("type", "text", text)
	c.send(c.typist.Append(text))
}

func (c *Controller) send(reports []Report) {
	if len(reports) > 0 {
		c.out(reports)
	}
}

// keepPreRoll remembers the most recent audio so the start of a word spoken
// as the talk key goes down is not lost.
func (c *Controller) keepPreRoll(samples []float32) {
	if c.preMax == 0 {
		return
	}
	c.preRoll = append(c.preRoll, samples...)
	if extra := len(c.preRoll) - c.preMax; extra > 0 {
		c.preRoll = append(c.preRoll[:0], c.preRoll[extra:]...)
	}
}
