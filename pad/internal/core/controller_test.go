package core

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTalkKeyCancel(t *testing.T) {
	t0 := time.Unix(0, 0)
	var k TalkKey
	k.Press(t0) // down, dictation on
	k.Cancel()
	if k.Active() {
		t.Fatal("still active after Cancel")
	}
	if got := k.Release(t0.Add(2 * time.Second)); got != TalkNone {
		t.Errorf("release after Cancel = %d, want nothing", got)
	}
	if got := k.Press(t0.Add(3 * time.Second)); got != TalkStart {
		t.Errorf("next press = %d, want start", got)
	}
}

func TestTypistAppend(t *testing.T) {
	ty := Typist{Capitalize: true}
	var h host
	h.apply(t, ty.Append("the quick"))
	h.apply(t, ty.Append(" brown fox"))
	h.apply(t, ty.Append("."))
	ty.EndPhrase()
	if got, want := h.String(), "The quick brown fox."; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	h.apply(t, ty.Append(" it jumped")) // leading space from the recogniser is not doubled
	h.apply(t, ty.Append(" over"))
	ty.EndPhrase()
	if got, want := h.String(), "The quick brown fox. It jumped over"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	h.apply(t, ty.Append(".")) // late punctuation joins the phrase before it
	h.apply(t, ty.Append("and then"))
	if got, want := h.String(), "The quick brown fox. It jumped over. And then"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	h.apply(t, ty.UndoPhrase())
	if got, want := h.String(), "The quick brown fox. It jumped over."; got != want {
		t.Fatalf("after undo got %q, want %q", got, want)
	}
	h.apply(t, ty.UndoPhrase()) // removes the phrase and its late full stop together
	if got, want := h.String(), "The quick brown fox."; got != want {
		t.Fatalf("after second undo got %q, want %q", got, want)
	}
}

func TestTypistCapitalization(t *testing.T) {
	tests := []struct {
		name string
		do   func(ty *Typist, h *host, t *testing.T)
		want string
	}{
		{"start of session", func(ty *Typist, h *host, t *testing.T) { h.apply(t, ty.Phrase("hello")) }, "Hello"},
		{"mid sentence stays lower", func(ty *Typist, h *host, t *testing.T) {
			h.apply(t, ty.Phrase("hello"))
			h.apply(t, ty.Phrase("world"))
		}, "Hello world"},
		{"after question mark", func(ty *Typist, h *host, t *testing.T) {
			h.apply(t, ty.Phrase("really?"))
			h.apply(t, ty.Phrase("yes"))
		}, "Really? Yes"},
		{"after new line", func(ty *Typist, h *host, t *testing.T) {
			h.apply(t, ty.Phrase("one"))
			h.apply(t, ty.NewLine())
			h.apply(t, ty.Phrase("two"))
		}, "One\nTwo"},
		{"unknown after delete word", func(ty *Typist, h *host, t *testing.T) {
			h.apply(t, ty.Phrase("one two"))
			h.apply(t, ty.DeleteWord())
			h.apply(t, ty.Phrase("three"))
		}, "One three"},
		{"a number is not capitalised past", func(ty *Typist, h *host, t *testing.T) { h.apply(t, ty.Phrase("3 pods")) }, "3 pods"},
		{"model capitals are kept", func(ty *Typist, h *host, t *testing.T) {
			h.apply(t, ty.Phrase("we use"))
			h.apply(t, ty.Phrase("Kubernetes"))
		}, "We use Kubernetes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ty := Typist{Capitalize: true}
			var h host
			tt.do(&ty, &h, t)
			if got := h.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	ty := Typist{}
	var h host
	h.apply(t, ty.Phrase("hello"))
	if got := h.String(); got != "hello" {
		t.Errorf("Capitalize off: got %q", got)
	}
}

func TestCommitter(t *testing.T) {
	c := committer{settle: time.Second, phraseGap: 2 * time.Second}
	step := 100 * time.Millisecond
	type in struct {
		hyp   string
		delta string
		end   bool
	}
	seq := []in{
		{"", "", false},
		{"The quick", "The", false},          // last word held back
		{"The quick brown", " quick", false}, // previous last word now complete
		{"The quick brown", "", false},
	}
	for i, s := range seq {
		d, e := c.advance(s.hyp, step)
		if d != s.delta || e != s.end {
			t.Fatalf("step %d: got %q, %v; want %q, %v", i, d, e, s.delta, s.end)
		}
	}
	// unchanged for the settle time: the held word is typed
	var got string
	for range 9 {
		d, e := c.advance("The quick brown", step)
		got += d
		if e {
			t.Fatal("phrase ended before the gap")
		}
	}
	if got != " brown" {
		t.Fatalf("after settle got %q, want %q", got, " brown")
	}
	// unchanged for the phrase gap: the phrase ends exactly once
	ends := 0
	for range 15 {
		if _, e := c.advance("The quick brown", step); e {
			ends++
		}
	}
	if ends != 1 {
		t.Fatalf("phrase ended %d times, want 1", ends)
	}
	if c.idle() < 2*time.Second {
		t.Errorf("idle = %v, want at least the phrase gap", c.idle())
	}
	// more speech, then the stream finishes
	if d, _ := c.advance("The quick brown fox jum", step); d != " fox" {
		t.Fatalf("got %q, want %q", d, " fox")
	}
	if c.idle() != 0 {
		t.Error("idle must be zero while text is pending")
	}
	if d := c.finish("The quick brown fox jumps."); d != " jumps." {
		t.Fatalf("finish got %q, want %q", d, " jumps.")
	}
	if d, _ := c.advance("New", step); d != "" || c.committed != "" {
		t.Errorf("after finish the stream starts over; got delta %q committed %q", d, c.committed)
	}
}

func TestCommitterRevision(t *testing.T) {
	c := committer{settle: time.Second, phraseGap: 2 * time.Second}
	c.advance("I scream for", 0) // types "I scream"
	if d, _ := c.advance("Ice cream for all", 0); d != "" {
		t.Fatalf("a rewritten hypothesis must not be typed over; got %q", d)
	}
	if d, _ := c.advance("Ice cream for all of us", 0); d != " of" {
		t.Fatalf("after a revision typing resumes with new words; got %q", d)
	}
}

// fakeRec is a recogniser driven by a script: its hypothesis is the longest
// scripted text whose time has been reached.
type fakeRec struct {
	script []struct {
		at   time.Duration
		text string
	}
	final  string
	heard  time.Duration
	resets int
	fed    int
}

func (f *fakeRec) Accept(s []float32) {
	f.fed += len(s)
	f.heard += time.Duration(len(s)) * time.Second / SampleRate
}
func (f *fakeRec) Text() string {
	out := ""
	for _, s := range f.script {
		if f.heard >= s.at {
			out = s.text
		}
	}
	return out
}
func (f *fakeRec) Finish() string { return f.final }
func (f *fakeRec) Reset()         { f.heard = 0; f.resets++ }

type rig struct {
	t     *testing.T
	c     *Controller
	rec   *fakeRec
	host  host
	now   time.Time
	state []bool
}

func newRig(t *testing.T, rec *fakeRec) *rig {
	r := &rig{t: t, rec: rec, now: time.Unix(0, 0)}
	r.c = NewController(rec, func(rep []Report) { r.host.apply(t, rep) }, func(on bool) { r.state = append(r.state, on) }, nil,
		Options{Capitalize: true})
	return r
}

func (r *rig) key(k Key, down bool) { r.c.HandleKey(KeyEvent{Key: k, Down: down, At: r.now}) }
func (r *rig) tap(k Key) {
	r.key(k, true)
	r.now = r.now.Add(50 * time.Millisecond)
	r.key(k, false)
}
func (r *rig) audio(d time.Duration) {
	for ; d > 0; d -= 100 * time.Millisecond {
		r.c.HandleAudio(make([]float32, SampleRate/10))
		r.now = r.now.Add(100 * time.Millisecond)
	}
}
func (r *rig) want(text string) {
	r.t.Helper()
	if got := r.host.String(); got != text {
		r.t.Fatalf("host text = %q, want %q", got, text)
	}
}

func script(pairs ...any) []struct {
	at   time.Duration
	text string
} {
	var out []struct {
		at   time.Duration
		text string
	}
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, struct {
			at   time.Duration
			text string
		}{pairs[i].(time.Duration), pairs[i+1].(string)})
	}
	return out
}

func TestControllerToggleSession(t *testing.T) {
	rec := &fakeRec{script: script(
		1*time.Second, "the web",
		2*time.Second, "the webhook returned",
		3*time.Second, "the webhook returned an error",
		4*time.Second, "the webhook returned an error.",
		9*time.Second, "the webhook returned an error. it was",
	), final: "the webhook returned an error. it was fixed."}
	r := newRig(t, rec)

	r.audio(time.Second) // before the key: nothing is typed
	r.want("")
	r.tap(KeyTalk)
	if !r.c.Listening() {
		t.Fatal("tap did not start dictation")
	}
	r.audio(1500 * time.Millisecond)
	r.want("The")
	r.audio(2 * time.Second)
	r.want("The webhook returned an")
	r.audio(3 * time.Second) // a pause: the last word and full stop settle, the phrase ends
	r.want("The webhook returned an error.")
	r.audio(3 * time.Second) // now past 9 s
	r.want("The webhook returned an error. It")
	r.tap(KeyTalk) // stop: the rest is flushed
	r.want("The webhook returned an error. It was fixed.")
	if r.c.Listening() {
		t.Fatal("second tap did not stop dictation")
	}

	r.tap(KeyUndo) // only the second phrase goes
	r.want("The webhook returned an error.")
	r.tap(KeyUndo)
	r.want("")
	if len(r.state) != 2 || !r.state[0] || r.state[1] {
		t.Errorf("state changes = %v, want [true false]", r.state)
	}
}

func TestControllerHoldToTalk(t *testing.T) {
	rec := &fakeRec{script: script(time.Second, "hello"), final: "hello world"}
	r := newRig(t, rec)
	r.key(KeyTalk, true)
	r.audio(1500 * time.Millisecond)
	r.want("")            // one word, still held back
	r.key(KeyTalk, false) // released after 1.5 s: a hold, so it stops
	r.want("Hello world")
	if r.c.Listening() {
		t.Fatal("still listening after releasing a hold")
	}
}

func TestControllerEditingKeys(t *testing.T) {
	rec := &fakeRec{final: "first line"}
	r := newRig(t, rec)
	r.key(KeyTalk, true)
	r.audio(time.Second)
	r.key(KeyTalk, false)
	r.tap(KeyNewLine)
	rec.final = "second line here"
	r.key(KeyTalk, true)
	r.audio(time.Second)
	r.key(KeyTalk, false)
	r.want("First line\nSecond line here")
	r.tap(KeyDeleteWord)
	r.want("First line\nSecond line ")
	r.tap(KeyUndo) // the count is unknown after delete-word: nothing happens
	r.want("First line\nSecond line ")
}

func TestControllerCancel(t *testing.T) {
	rec := &fakeRec{script: script(time.Second, "keep this but"), final: "keep this but not that"}
	r := newRig(t, rec)
	r.tap(KeyTalk)
	r.audio(1500 * time.Millisecond)
	r.want("Keep this")
	r.tap(KeyCancel)
	r.want("Keep this") // what was typed stays; the rest is thrown away
	if r.c.Listening() {
		t.Fatal("still listening after cancel")
	}
	r.tap(KeyCancel) // harmless when idle
	r.tap(KeyUndo)
	r.want("")
}

func TestControllerCancelDuringHold(t *testing.T) {
	rec := &fakeRec{final: "never typed"}
	r := newRig(t, rec)
	r.key(KeyTalk, true)
	r.audio(time.Second)
	r.tap(KeyCancel)
	r.key(KeyTalk, false) // releasing the held key must not flush or restart
	r.want("")
	if r.c.Listening() {
		t.Fatal("listening after cancel and release")
	}
}

func TestControllerPreRoll(t *testing.T) {
	rec := &fakeRec{}
	r := newRig(t, rec)
	r.audio(2 * time.Second) // idle audio: only the last 300 ms is kept
	r.tap(KeyTalk)
	if want := SampleRate * 3 / 10; rec.fed != want {
		t.Errorf("pre-roll fed %d samples, want %d", rec.fed, want)
	}
}

func TestControllerIdleReset(t *testing.T) {
	rec := &fakeRec{script: script(time.Second, "done.")}
	r := newRig(t, rec)
	r.tap(KeyTalk)
	before := rec.resets
	r.audio(11500 * time.Millisecond) // the stream restarts about 10 s after the text settled
	r.want("Done.")
	if rec.resets != before+1 {
		t.Errorf("resets during a long silence = %d, want 1", rec.resets-before)
	}
	rec.script = script(time.Second, "next")
	r.audio(3 * time.Second)
	r.want("Done. Next")
}

func TestHostSimOnChange(t *testing.T) {
	var seen []string
	h := HostSim{OnChange: func(s string) { seen = append(seen, s) }}
	reports, _ := TypeString("ab")
	if err := WriteReports(&h, reports); err != nil {
		t.Fatal(err)
	}
	if strings.Join(seen, ",") != "a,ab" {
		t.Errorf("OnChange saw %v", seen)
	}
	if _, err := h.Write([]byte{0, 0, 0xFF, 0, 0, 0, 0, 0}); err != nil || h.Text() != "ab" {
		t.Error("an unknown key must be ignored")
	}
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

type failAfter struct {
	n   int
	got int
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.got >= f.n {
		return 0, errors.New("unplugged")
	}
	f.got++
	return len(p), nil
}

func TestReportQueue(t *testing.T) {
	var buf lockedBuf
	q := NewReportQueue(&buf, 0, nil)
	q.Send(Tap(0, KeyEnter))
	q.Send(Tap(ModShift, 0x04))
	q.Close()
	if got := buf.b.Len(); got != 5*8 { // two taps and the final release
		t.Errorf("wrote %d bytes, want 40", got)
	}

	// a failed write drops the rest of that batch and the queue carries on
	fw := &failAfter{n: 1}
	q = NewReportQueue(fw, time.Microsecond, nil)
	q.Send(Tap(0, KeyEnter))
	q.Close()
	if fw.got != 1 {
		t.Errorf("writes that succeeded = %d, want 1", fw.got)
	}
}

// The model often adds a sentence's full stop a couple of seconds late. That
// must not hide the pause after the sentence, and the full stop must stay
// part of the phrase it belongs to.
func TestCommitterLatePunctuationDoesNotHideAPause(t *testing.T) {
	c := committer{settle: 1500 * time.Millisecond, phraseGap: 2 * time.Second}
	step := 100 * time.Millisecond
	feed := func(hyp string, d time.Duration) (typed string, ends int) {
		for ; d > 0; d -= step {
			delta, end := c.advance(hyp, step)
			typed += delta
			if end {
				ends++
			}
		}
		return
	}
	typed, ends := feed("on each node", 1200*time.Millisecond)
	if typed != "on each" || ends != 0 {
		t.Fatalf("got %q, %d", typed, ends)
	}
	// the full stop arrives 1.2 s after the last word
	typed, ends = feed("on each node.", 1400*time.Millisecond)
	if typed != "" || ends != 0 {
		t.Fatalf("before the full stop settles: got %q, %d ends", typed, ends)
	}
	// The pause is now 2.6 s by words, past the gap, but "node." is still
	// waiting to settle: the phrase must not end with text pending.
	typed, ends = feed("on each node.", 300*time.Millisecond)
	if typed != " node." || ends != 1 {
		t.Fatalf("the word and its full stop are typed, then the phrase ends: got %q, %d ends", typed, ends)
	}
	typed, ends = feed("on each node. The web", 100*time.Millisecond)
	if typed != " The" || ends != 0 {
		t.Fatalf("next phrase: got %q, %d", typed, ends)
	}
}
