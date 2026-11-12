package core

import (
	"strings"
	"time"
)

// committer decides how much of a growing hypothesis is stable enough to
// type. A keyboard cannot cheaply take text back, so it types whole words
// only: everything before the last space at once, and the final word once
// the hypothesis has stopped changing.
type committer struct {
	settle    time.Duration // unchanged this long: the last word is typed
	phraseGap time.Duration // unchanged this long: the phrase ends

	hyp        string        // the hypothesis last seen
	committed  string        // the prefix of it already typed
	unchanged  time.Duration // audio time since the hypothesis last changed
	phraseOpen bool          // something was typed since the last phrase end
}

// advance takes the current hypothesis after dur more audio. It returns the
// text to type now and whether the phrase has ended.
func (c *committer) advance(hyp string, dur time.Duration) (delta string, endPhrase bool) {
	if hyp != c.hyp {
		c.hyp = hyp
		c.unchanged = 0
	} else {
		c.unchanged += dur
	}
	delta = c.take(c.unchanged >= c.settle)
	if c.phraseOpen && c.unchanged >= c.phraseGap {
		c.phraseOpen = false
		endPhrase = true
	}
	return delta, endPhrase
}

// finish takes the final hypothesis of a stream and returns what is left to
// type.
func (c *committer) finish(hyp string) string {
	c.hyp = hyp
	delta := c.take(true)
	*c = committer{settle: c.settle, phraseGap: c.phraseGap}
	return delta
}

// idle reports how long a fully typed hypothesis has been unchanged, or zero
// when text is pending or nothing has been heard.
func (c *committer) idle() time.Duration {
	if c.hyp == "" || c.committed != c.hyp {
		return 0
	}
	return c.unchanged
}

// reset forgets the stream, keeping the timing settings.
func (c *committer) reset() {
	*c = committer{settle: c.settle, phraseGap: c.phraseGap}
}

func (c *committer) take(all bool) string {
	if !strings.HasPrefix(c.hyp, c.committed) {
		// The recogniser rewrote text that is already on screen. It cannot
		// be fixed from here; skip to the new hypothesis rather than type
		// the same words twice.
		c.committed = c.hyp
		return ""
	}
	end := len(c.hyp)
	if !all {
		end = strings.LastIndexByte(c.hyp, ' ')
	}
	if end <= len(c.committed) {
		return ""
	}
	delta := c.hyp[len(c.committed):end]
	c.committed = c.hyp[:end]
	if strings.TrimSpace(delta) != "" {
		c.phraseOpen = true
	}
	return delta
}
