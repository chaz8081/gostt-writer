package core

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestEncodeRune(t *testing.T) {
	tests := []struct {
		r        rune
		mod, key byte
		ok       bool
	}{
		{'a', 0, 0x04, true},
		{'z', 0, 0x1D, true},
		{'A', ModShift, 0x04, true},
		{'1', 0, 0x1E, true},
		{'0', 0, 0x27, true},
		{' ', 0, 0x2C, true},
		{'.', 0, 0x37, true},
		{'?', ModShift, 0x38, true},
		{'/', 0, 0x38, true},
		{'\n', 0, KeyEnter, true},
		{'’', 0, 0x34, true}, // curly apostrophe typed as a straight one
		{'é', 0, 0, false},
	}
	for _, tt := range tests {
		mod, key, ok := EncodeRune(tt.r)
		if mod != tt.mod || key != tt.key || ok != tt.ok {
			t.Errorf("EncodeRune(%q) = %#x, %#x, %v; want %#x, %#x, %v", tt.r, mod, key, ok, tt.mod, tt.key, tt.ok)
		}
	}
}

func TestTypeString(t *testing.T) {
	reports, typed := TypeString("Hié!")
	if typed != 3 {
		t.Fatalf("typed = %d, want 3 (é skipped)", typed)
	}
	want := []Report{
		{ModShift, 0, 0x0B}, {}, // H
		{0, 0, 0x0C}, {}, // i
		{ModShift, 0, 0x1E}, {}, // !
	}
	if len(reports) != len(want) {
		t.Fatalf("got %d reports, want %d", len(reports), len(want))
	}
	for i := range want {
		if reports[i] != want[i] {
			t.Errorf("report %d = %v, want %v", i, reports[i], want[i])
		}
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("gone") }

func TestWriteReports(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReports(&buf, Tap(ModOption, KeyBackspace)); err != nil {
		t.Fatal(err)
	}
	want := []byte{ModOption, 0, KeyBackspace, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("wrote %v, want %v", buf.Bytes(), want)
	}
	if err := WriteReports(failWriter{}, Tap(0, KeyEnter)); err == nil {
		t.Error("want an error from a failing writer")
	}
}

func TestTalkKey(t *testing.T) {
	t0 := time.Unix(0, 0)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	type step struct {
		press bool
		ms    int
		want  TalkEvent
	}
	tests := []struct {
		name   string
		steps  []step
		active bool
	}{
		{"tap toggles on", []step{{true, 0, TalkStart}, {false, 100, TalkNone}}, true},
		{"second tap toggles off", []step{
			{true, 0, TalkStart}, {false, 100, TalkNone},
			{true, 5000, TalkStop}, {false, 5100, TalkNone},
		}, false},
		{"hold stops on release", []step{{true, 0, TalkStart}, {false, 2000, TalkStop}}, false},
		{"exactly the threshold is a hold", []step{{true, 0, TalkStart}, {false, 350, TalkStop}}, false},
		{"long press while toggled on stops once", []step{
			{true, 0, TalkStart}, {false, 100, TalkNone},
			{true, 5000, TalkStop}, {false, 9000, TalkNone},
		}, false},
		{"repeat press while down is ignored", []step{
			{true, 0, TalkStart}, {true, 50, TalkNone}, {false, 100, TalkNone},
		}, true},
		{"stray release is ignored", []step{{false, 0, TalkNone}}, false},
		{"tap, tap, hold", []step{
			{true, 0, TalkStart}, {false, 100, TalkNone},
			{true, 1000, TalkStop}, {false, 1100, TalkNone},
			{true, 2000, TalkStart}, {false, 4000, TalkStop},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var k TalkKey
			for i, s := range tt.steps {
				var got TalkEvent
				if s.press {
					got = k.Press(at(s.ms))
				} else {
					got = k.Release(at(s.ms))
				}
				if got != s.want {
					t.Fatalf("step %d: got event %d, want %d", i, got, s.want)
				}
			}
			if k.Active() != tt.active {
				t.Errorf("Active() = %v, want %v", k.Active(), tt.active)
			}
		})
	}
}

func TestTalkKeyCustomThreshold(t *testing.T) {
	k := TalkKey{HoldThreshold: time.Second}
	t0 := time.Unix(0, 0)
	k.Press(t0)
	if got := k.Release(t0.Add(500 * time.Millisecond)); got != TalkNone {
		t.Errorf("500 ms with a 1 s threshold: got %d, want a tap", got)
	}
}

// host wraps HostSim for tests.
type host struct{ sim HostSim }

func (h *host) apply(t *testing.T, reports []Report) {
	t.Helper()
	if err := WriteReports(&h.sim, reports); err != nil {
		t.Fatal(err)
	}
}

func (h *host) String() string { return h.sim.Text() }

func TestTypist(t *testing.T) {
	var ty Typist
	var h host
	check := func(want string) {
		t.Helper()
		if got := h.String(); got != want {
			t.Fatalf("host text = %q, want %q", got, want)
		}
	}

	h.apply(t, ty.Phrase("Hello there."))
	check("Hello there.")
	h.apply(t, ty.Phrase("How are you?"))
	check("Hello there. How are you?")

	h.apply(t, ty.UndoPhrase())
	check("Hello there.")

	h.apply(t, ty.NewLine())
	h.apply(t, ty.Phrase("Second line."))
	check("Hello there.\nSecond line.") // no leading space after a new line

	h.apply(t, ty.UndoPhrase())
	check("Hello there.\n")
	h.apply(t, ty.Phrase("Retry."))
	check("Hello there.\nRetry.") // still at the start of the line, so no space
	h.apply(t, ty.UndoPhrase())
	check("Hello there.\n")
	h.apply(t, ty.UndoPhrase()) // the new line itself
	check("Hello there.")
	h.apply(t, ty.Phrase("Again."))
	check("Hello there. Again.") // back on a line with text, so a space

	h.apply(t, ty.UndoPhrase())
	h.apply(t, ty.UndoPhrase())
	check("")
	if r := ty.UndoPhrase(); r != nil {
		t.Errorf("undo with empty history returned %d reports", len(r))
	}
}

func TestTypistDeleteWordClearsUndo(t *testing.T) {
	var ty Typist
	var h host
	h.apply(t, ty.Phrase("one two three"))
	h.apply(t, ty.DeleteWord())
	if got := h.String(); got != "one two " {
		t.Fatalf("host text = %q, want %q", got, "one two ")
	}
	if r := ty.UndoPhrase(); r != nil {
		t.Error("undo after delete-word must do nothing: the count is no longer known")
	}
}

func TestTypistSkipsEmptyAndUntypeable(t *testing.T) {
	var ty Typist
	if r := ty.Phrase(""); r != nil {
		t.Error("empty phrase produced reports")
	}
	if r := ty.Phrase("éé"); r != nil {
		t.Error("untypeable phrase produced reports")
	}
	if r := ty.UndoPhrase(); r != nil {
		t.Error("nothing was typed, so nothing to undo")
	}
}

func TestTypistReset(t *testing.T) {
	var ty Typist
	var h host
	h.apply(t, ty.Phrase("first"))
	ty.Reset()
	if r := ty.UndoPhrase(); r != nil {
		t.Error("undo after Reset must do nothing")
	}
	h.apply(t, ty.Phrase("second"))
	if got := h.String(); got != "firstsecond" {
		t.Errorf("host text = %q; after Reset the pad should not add a space", got)
	}
}
