package core

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Typist turns recognised text and editing keys into keyboard reports, and
// remembers what it typed so the last phrase can be removed exactly.
//
// The record is only as good as the pad's knowledge of the host: if the user
// moves the cursor or the application rewrites the text, an undo deletes the
// wrong characters. Actions whose effect the pad cannot count clear the
// record instead of guessing.
type Typist struct {
	// Capitalize makes the first letter of a sentence upper case when the
	// pad knows it is at the start of one.
	Capitalize bool

	history []step // undoable steps, oldest first
	open    bool   // the last step is a phrase still being typed
	unknown bool   // the pad no longer knows what is before the cursor
}

// step is one thing the pad typed that it can take back.
type step struct {
	chars        int  // characters it put on the host
	newLine      bool // it was an Enter, so the line after it starts empty
	endsSentence bool // its last visible character was . ! or ?
}

// lineDirty reports whether the pad has typed on the current line, as far as
// it knows.
func (t *Typist) lineDirty() bool {
	return len(t.history) > 0 && !t.history[len(t.history)-1].newLine
}

// sentenceStart reports whether the next text begins a sentence. It is false
// when the pad cannot tell.
func (t *Typist) sentenceStart() bool {
	if len(t.history) == 0 {
		return !t.unknown
	}
	last := t.history[len(t.history)-1]
	return last.newLine || last.endsSentence
}

// Append returns the reports that type text as part of the current phrase,
// starting a phrase if none is open. Text that begins a phrase gets a space
// before it when it follows other text on the line, unless it starts with
// closing punctuation, which is attached to the previous phrase.
func (t *Typist) Append(text string) []Report {
	if text == "" {
		return nil
	}
	first, _ := utf8.DecodeRuneInString(strings.TrimLeft(text, " "))
	attach := strings.ContainsRune(".,!?;:", first) && !strings.HasPrefix(text, " ")
	if !t.open && !attach {
		text = strings.TrimLeft(text, " ")
		if t.lineDirty() {
			text = " " + text
		}
	}
	if !t.lineDirty() {
		text = strings.TrimLeft(text, " ")
	}
	if t.Capitalize && t.sentenceStart() {
		text = capitalizeFirst(text)
	}
	reports, typed := TypeString(text)
	if typed == 0 {
		return nil
	}
	ends, visible := endsSentence(text)
	extend := len(t.history) > 0 && !t.history[len(t.history)-1].newLine && (t.open || attach)
	if extend {
		last := &t.history[len(t.history)-1]
		last.chars += typed
		if visible {
			last.endsSentence = ends
		}
	} else {
		t.history = append(t.history, step{chars: typed, endsSentence: ends})
	}
	if !attach {
		t.open = true
	}
	return reports
}

// EndPhrase closes the current phrase, so the next text starts a new one and
// an undo removes them separately.
func (t *Typist) EndPhrase() { t.open = false }

// Phrase types one complete phrase: Append followed by EndPhrase.
func (t *Typist) Phrase(text string) []Report {
	reports := t.Append(text)
	t.EndPhrase()
	return reports
}

// UndoPhrase returns the reports that delete the most recent phrase or new
// line the pad typed, or nil when there is nothing it can safely undo.
func (t *Typist) UndoPhrase() []Report {
	t.open = false
	if len(t.history) == 0 {
		return nil
	}
	n := t.history[len(t.history)-1].chars
	t.history = t.history[:len(t.history)-1]
	reports := make([]Report, 0, 2*n)
	for range n {
		reports = append(reports, Tap(0, KeyBackspace)...)
	}
	return reports
}

// NewLine returns the reports for Enter. It can be undone like a phrase.
func (t *Typist) NewLine() []Report {
	t.open = false
	t.unknown = false
	t.history = append(t.history, step{chars: 1, newLine: true})
	return Tap(0, KeyEnter)
}

// DeleteWord returns the reports for the macOS delete-word shortcut
// (Option+Backspace). The pad cannot know how much the host deleted, so the
// undo record is cleared.
func (t *Typist) DeleteWord() []Report {
	t.Reset()
	return Tap(ModOption, KeyBackspace)
}

// Reset forgets everything typed, for when the pad no longer knows what is
// before the cursor.
func (t *Typist) Reset() {
	t.history = nil
	t.open = false
	t.unknown = true
}

// capitalizeFirst upper-cases the first letter of s if it is the first
// letter or digit in it.
func capitalizeFirst(s string) string {
	for i, r := range s {
		if unicode.IsLetter(r) {
			return s[:i] + string(unicode.ToUpper(r)) + s[i+utf8.RuneLen(r):]
		}
		if unicode.IsDigit(r) {
			return s
		}
	}
	return s
}

// endsSentence reports whether the last visible character of s ends a
// sentence, and whether s has any visible character at all.
func endsSentence(s string) (ends, visible bool) {
	s = strings.TrimRight(s, " ")
	if s == "" {
		return false, false
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune(".!?", r), true
}
