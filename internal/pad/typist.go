package pad

// Typist turns recognised phrases and editing keys into keyboard reports,
// and remembers what it typed so the last phrase can be removed exactly.
//
// The record is only as good as the pad's knowledge of the host: if the user
// moves the cursor or the application rewrites the text, an undo deletes the
// wrong characters. Actions whose effect the pad cannot count clear the
// record instead of guessing.
type Typist struct {
	history []step // undoable steps, oldest first
}

// step is one thing the pad typed that it can take back.
type step struct {
	chars   int  // characters it put on the host
	newLine bool // it was an Enter, so the line after it starts empty
}

// lineDirty reports whether the pad has typed on the current line, as far as
// it knows.
func (t *Typist) lineDirty() bool {
	return len(t.history) > 0 && !t.history[len(t.history)-1].newLine
}

// Phrase returns the reports that type one finished phrase. A space is put
// before it when it follows other text on the same line.
func (t *Typist) Phrase(text string) []Report {
	if text == "" {
		return nil
	}
	if t.lineDirty() {
		text = " " + text
	}
	reports, typed := TypeString(text)
	if typed == 0 {
		return nil
	}
	t.history = append(t.history, step{chars: typed})
	return reports
}

// UndoPhrase returns the reports that delete the most recent phrase or new
// line the pad typed, or nil when there is nothing it can safely undo.
func (t *Typist) UndoPhrase() []Report {
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
	t.history = append(t.history, step{chars: 1, newLine: true})
	return Tap(0, KeyEnter)
}

// DeleteWord returns the reports for the macOS delete-word shortcut
// (Option+Backspace). The pad cannot know how much the host deleted, so the
// undo record is cleared.
func (t *Typist) DeleteWord() []Report {
	t.history = nil
	return Tap(ModOption, KeyBackspace)
}

// Reset forgets everything typed, for when the user has moved elsewhere.
func (t *Typist) Reset() {
	t.history = nil
}
