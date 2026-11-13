package speech

import (
	"context"
	"testing"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
)

// fakeRec "recognises" a word once it has been given enough loud audio.
type fakeRec struct {
	loud     int
	blocks   int
	word     string
	needs    int
	finished bool
	resets   int
}

func (f *fakeRec) Accept(s []float32) {
	f.blocks++
	if len(s) > 0 && s[0] > 0.05 {
		f.loud++
	}
}
func (f *fakeRec) Text() string {
	if f.loud >= f.needs {
		return f.word
	}
	return ""
}
func (f *fakeRec) Finish() string { f.finished = true; return f.word }
func (f *fakeRec) Reset()         { f.loud, f.finished = 0, false; f.resets++ }

// feed delivers one 100 ms block per level, as a microphone would, and then
// closes the channel.
func feed(levels ...float32) chan []float32 {
	ch := make(chan []float32)
	go func() {
		defer close(ch)
		for _, lv := range levels {
			b := make([]float32, audio.BlockSamples)
			for i := range b {
				b[i] = lv
			}
			ch <- b
		}
	}()
	return ch
}

func repeat(level float32, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = level
	}
	return out
}

const quiet, speech = 0.001, 0.3

func listener(rec Recognizer, mic chan []float32) *Listener {
	return &Listener{Rec: rec, Mic: mic, Level: 0.02, Quiet: 300 * time.Millisecond, MaxSpeech: time.Second}
}

func TestHearEndsAfterQuiet(t *testing.T) {
	rec := &fakeRec{word: "yes", needs: 99}
	l := listener(rec, feed(quiet, quiet, speech, speech, quiet, quiet, quiet, speech, speech))
	got, err := l.Hear(context.Background(), 5*time.Second)
	if err != nil || got != "yes" || !rec.finished {
		t.Fatalf("got %q, err %v, finished %v", got, err, rec.finished)
	}
	if rec.loud != 2 || rec.blocks != 2+2+3 {
		t.Errorf("want 2 lead-in, 2 loud and 3 quiet blocks; got %d loud of %d", rec.loud, rec.blocks)
	}
}

func TestHearReturnsEmptyOnSilence(t *testing.T) {
	rec := &fakeRec{word: "yes", needs: 1}
	got, err := listener(rec, feed(repeat(quiet, 10)...)).Hear(context.Background(), 400*time.Millisecond)
	if err != nil || got != "" || rec.finished {
		t.Fatalf("got %q, err %v, finished %v", got, err, rec.finished)
	}
}

func TestHearStopsLongSpeech(t *testing.T) {
	rec := &fakeRec{word: "blah blah", needs: 99}
	l := listener(rec, feed(append([]float32{quiet}, repeat(speech, 30)...)...))
	if got, _ := l.Hear(context.Background(), 5*time.Second); got != "blah blah" || rec.loud != 10 {
		t.Fatalf("got %q after %d loud blocks", got, rec.loud)
	}
}

func TestEnoughEndsEarlyInNoise(t *testing.T) {
	rec := &fakeRec{word: "no", needs: 3}
	l := listener(rec, feed(append([]float32{quiet}, repeat(speech, 30)...)...))
	l.MaxSpeech = 10 * time.Second
	l.Enough = func(s string) bool { return s == "no" }
	if got, _ := l.Hear(context.Background(), 5*time.Second); got != "no" || rec.loud != 15 {
		t.Fatalf("got %q after %d loud blocks", got, rec.loud)
	}
}

func TestHearDropsStaleAudioAndSkipsSpeakerTail(t *testing.T) {
	rec := &fakeRec{word: "yes", needs: 1}
	mic := make(chan []float32, 32)
	for range 3 { // captured before listening began
		mic <- repeat(speech, audio.BlockSamples)
	}
	l := listener(rec, mic)
	l.Skip = 200 * time.Millisecond
	go func() {
		for b := range feed(append([]float32{speech, speech}, repeat(quiet, 8)...)...) {
			mic <- b
		}
	}()
	got, _ := l.Hear(context.Background(), 400*time.Millisecond)
	if got != "" || rec.loud != 0 {
		t.Fatalf("heard %q from %d loud blocks that should have been ignored", got, rec.loud)
	}
}

func TestHearStopsWithContextOrClosedMic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listener(&fakeRec{}, make(chan []float32)).Hear(ctx, time.Second); err != context.Canceled {
		t.Fatalf("err %v", err)
	}
	if _, err := listener(&fakeRec{}, feed()).Hear(context.Background(), time.Second); err == nil {
		t.Fatal("no error when the microphone stream ends")
	}
}
