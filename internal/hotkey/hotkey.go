// Package hotkey provides a global hotkey listener using gohook.
// It supports "hold" mode (press to start, release to stop) and
// "toggle" mode (press to start, press again to stop).
package hotkey

import (
	"sync"

	hook "github.com/robotn/gohook"
)

// EventType indicates whether recording should start or stop.
type EventType int

const (
	// EventStart signals that the hotkey was activated (start recording).
	EventStart EventType = iota
	// EventStop signals that the hotkey was deactivated (stop recording).
	EventStop
)

// Event is emitted on the channel returned by Listen.
type Event struct {
	Type EventType
}

// Listener manages a global hotkey and emits start/stop events.
type Listener struct {
	keys []string
	mode string // "hold" or "toggle"
	ch   chan Event
	done chan struct{}
	once sync.Once
}

// NewListener creates a Listener for the given key combo and mode.
// keys should be lowercase key names (e.g., ["ctrl", "shift", "r"]).
// mode must be "hold" or "toggle".
func NewListener(keys []string, mode string) *Listener {
	return &Listener{
		keys: keys,
		mode: mode,
		ch:   make(chan Event, 16),
		done: make(chan struct{}),
	}
}

// Events returns the channel that receives hotkey events.
// The channel is closed when Stop is called.
func (l *Listener) Events() <-chan Event {
	return l.ch
}

// Start begins listening for the global hotkey.
// This function blocks until Stop is called. Run it in a goroutine.
func (l *Listener) Start() {
	switch l.mode {
	case "toggle":
		l.startToggle()
	default: // "hold"
		l.startHold()
	}
}

// startHold implements hold-to-talk mode:
// KeyDown -> EventStart, KeyUp -> EventStop.
func (l *Listener) startHold() {
	hook.Register(hook.KeyDown, l.keys, func(e hook.Event) {
		select {
		case l.ch <- Event{Type: EventStart}:
		default: // don't block if channel is full
		}
	})

	hook.Register(hook.KeyUp, l.keys, func(e hook.Event) {
		select {
		case l.ch <- Event{Type: EventStop}:
		default:
		}
	})

	evChan := hook.Start()
	go func() {
		<-l.done
		hook.End()
	}()
	<-hook.Process(evChan)
	close(l.ch)
}

// startToggle implements toggle mode:
// First press -> EventStart, second press -> EventStop, etc.
func (l *Listener) startToggle() {
	var mu sync.Mutex
	recording := false

	hook.Register(hook.KeyDown, l.keys, func(e hook.Event) {
		mu.Lock()
		defer mu.Unlock()
		if recording {
			select {
			case l.ch <- Event{Type: EventStop}:
			default:
			}
			recording = false
		} else {
			select {
			case l.ch <- Event{Type: EventStart}:
			default:
			}
			recording = true
		}
	})

	evChan := hook.Start()
	go func() {
		<-l.done
		hook.End()
	}()
	<-hook.Process(evChan)
	close(l.ch)
}

// Stop terminates the hotkey listener.
// It is safe to call multiple times.
func (l *Listener) Stop() {
	l.once.Do(func() {
		close(l.done)
	})
}
