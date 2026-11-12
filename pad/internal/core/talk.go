package core

import "time"

// DefaultHoldThreshold separates a tap from a hold on the talk key.
const DefaultHoldThreshold = 350 * time.Millisecond

// TalkEvent is what the talk key asks the recogniser to do.
type TalkEvent int

const (
	// TalkNone means the key event changes nothing.
	TalkNone TalkEvent = iota
	// TalkStart means begin streaming audio to the recogniser.
	TalkStart
	// TalkStop means stop streaming and flush what was heard.
	TalkStop
)

// TalkKey implements the talk key: a short tap toggles dictation on, and the
// next tap toggles it off; a long press dictates only while held.
//
// Dictation starts on the press either way, so no speech is lost while the
// key decides whether it is a tap or a hold.
type TalkKey struct {
	// HoldThreshold is the shortest press that counts as a hold. Zero means
	// DefaultHoldThreshold.
	HoldThreshold time.Duration

	active      bool      // dictation is running
	pressedAt   time.Time // when the press that started dictation happened
	startedHere bool      // the key is down and this press started dictation
	stoppedHere bool      // the key is down and this press stopped dictation
}

// Active reports whether dictation is running.
func (k *TalkKey) Active() bool { return k.active }

// Press handles the key going down at time t.
func (k *TalkKey) Press(t time.Time) TalkEvent {
	if k.startedHere || k.stoppedHere {
		return TalkNone // key repeat or bounce: already down
	}
	if k.active {
		k.active = false
		k.stoppedHere = true
		return TalkStop
	}
	k.active = true
	k.startedHere = true
	k.pressedAt = t
	return TalkStart
}

// Release handles the key going up at time t.
func (k *TalkKey) Release(t time.Time) TalkEvent {
	if k.stoppedHere {
		k.stoppedHere = false
		return TalkNone
	}
	if !k.startedHere {
		return TalkNone
	}
	k.startedHere = false
	threshold := k.HoldThreshold
	if threshold == 0 {
		threshold = DefaultHoldThreshold
	}
	if t.Sub(k.pressedAt) >= threshold {
		k.active = false
		return TalkStop
	}
	return TalkNone // a tap: dictation stays on until the next tap
}

// Cancel turns dictation off without the usual stop, for when the user
// throws away what they are saying. A release of a key that is still down is
// then ignored.
func (k *TalkKey) Cancel() {
	k.active = false
	if k.startedHere {
		k.startedHere = false
		k.stoppedHere = true
	}
}
