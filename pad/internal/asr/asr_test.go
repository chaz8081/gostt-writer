//go:build linux

package asr

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chaz8081/gostt-writer/pad/internal/audio"
	"github.com/chaz8081/gostt-writer/pad/internal/core"
)

// These tests run the real model. They need GOSTT_PAD_MODEL_DIR to point at
// the model folder and are skipped without it. The audio is the repository's
// transcription benchmark set.
const clips = "../../../internal/transcribe/testdata"

func model(t *testing.T, decoding string) *Recognizer {
	t.Helper()
	dir := os.Getenv("GOSTT_PAD_MODEL_DIR")
	if dir == "" {
		t.Skip("GOSTT_PAD_MODEL_DIR is not set")
	}
	r, err := New(Config{Dir: dir, Threads: 2, Decoding: decoding, TailPadding: 2500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func clip(t *testing.T, name string) []float32 {
	t.Helper()
	s, err := audio.ReadWAV(filepath.Join(clips, name))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var notWord = regexp.MustCompile(`[^a-z0-9' ]+`)

func words(s string) string {
	return strings.Join(strings.Fields(notWord.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

type session struct {
	t    *testing.T
	c    *core.Controller
	host core.HostSim
	now  time.Time
}

func newSession(t *testing.T, rec *Recognizer) *session {
	s := &session{t: t, now: time.Unix(0, 0)}
	s.c = core.NewController(rec, func(r []core.Report) {
		if err := core.WriteReports(&s.host, r); err != nil {
			t.Fatal(err)
		}
	}, nil, nil, core.Options{Capitalize: true})
	return s
}

func (s *session) key(k core.Key, down bool) {
	s.c.HandleKey(core.KeyEvent{Key: k, Down: down, At: s.now})
}
func (s *session) tap(k core.Key) {
	s.key(k, true)
	s.now = s.now.Add(50 * time.Millisecond)
	s.key(k, false)
}
func (s *session) feed(samples []float32) {
	for i := 0; i < len(samples); i += audio.BlockSamples {
		s.c.HandleAudio(samples[i:min(i+audio.BlockSamples, len(samples))])
		s.now = s.now.Add(100 * time.Millisecond)
	}
}
func (s *session) silence(d time.Duration) {
	s.feed(make([]float32, int(d*core.SampleRate/time.Second)))
}

const (
	shortText  = "the kubernetes cluster ran three pods on each node"
	mediumText = "the web hook endpoint returned a four hundred error because the payload exceeded the maximum size limit for json serialization"
	longText   = "after deploying the micro services to production we noticed that the grpc connections were timing out the root cause was a misconfigured load balancer that did not support http two multiplexing"
)

// wordErrors is the word-level edit distance between what was typed and the
// reference. The model is not perfect and its mistakes shift with context
// and CPU, so tests allow a few rather than demand an exact transcript.
func wordErrors(got, want string) int {
	g, w := strings.Fields(words(got)), strings.Fields(want)
	d := make([]int, len(g)+1)
	for j := range d {
		d[j] = j
	}
	for i := 1; i <= len(w); i++ {
		prev := d[0]
		d[0] = i
		for j := 1; j <= len(g); j++ {
			cost := 1
			if w[i-1] == g[j-1] {
				cost = 0
			}
			prev, d[j] = d[j], min(d[j]+1, d[j-1]+1, prev+cost)
		}
	}
	return d[len(g)]
}

func TestToggleSessionTypesAndUndoes(t *testing.T) {
	for _, decoding := range []string{"modified_beam_search", "greedy_search"} {
		t.Run(decoding, func(t *testing.T) {
			s := newSession(t, model(t, decoding))
			s.silence(time.Second)
			s.tap(core.KeyTalk)
			s.feed(clip(t, "short.wav"))
			s.silence(4 * time.Second)
			afterShort := s.host.Text()
			if n := wordErrors(afterShort, shortText); n > 1 {
				t.Fatalf("after the first clip and a pause, %d word errors:\n got %q\nwant %q", n, afterShort, shortText)
			}
			s.feed(clip(t, "medium.wav"))
			s.silence(4 * time.Second)
			afterMedium := s.host.Text()
			s.feed(clip(t, "long.wav"))
			s.tap(core.KeyTalk) // stop straight after the last word

			got := s.host.Text()
			t.Logf("typed: %s", got)
			want := shortText + " " + mediumText + " " + longText
			if n := wordErrors(got, want); n > 3 {
				t.Errorf("whole session, %d word errors in %d words:\n got %q\nwant %q", n, len(strings.Fields(want)), words(got), want)
			}
			if !strings.HasPrefix(got, "The Kubernetes") {
				t.Errorf("the session should start with a capital: %q", got[:20])
			}
			if strings.Contains(got, "  ") || strings.HasPrefix(got, " ") {
				t.Errorf("stray spaces in %q", got)
			}

			s.tap(core.KeyUndo) // the long clip was one phrase
			if s.host.Text() != afterMedium {
				t.Errorf("after one undo:\n got %q\nwant %q", s.host.Text(), afterMedium)
			}
			s.tap(core.KeyUndo)
			if s.host.Text() != afterShort {
				t.Errorf("after two undos:\n got %q\nwant %q", s.host.Text(), afterShort)
			}
			s.tap(core.KeyUndo)
			if s.host.Text() != "" {
				t.Errorf("after three undos the field should be empty, got %q", s.host.Text())
			}
		})
	}
}

func TestHoldToTalkKeepsTheLastWord(t *testing.T) {
	s := newSession(t, model(t, "modified_beam_search"))
	s.key(core.KeyTalk, true)
	s.feed(clip(t, "short.wav")) // no silence after the last word
	s.key(core.KeyTalk, false)
	if got := s.host.Text(); wordErrors(got, shortText) > 1 || !strings.HasSuffix(words(got), "node") {
		t.Fatalf("got %q\nwant %q, ending with its last word", got, shortText)
	}

	// a second hold on the same line continues it
	s.tap(core.KeyNewLine)
	s.key(core.KeyTalk, true)
	s.feed(clip(t, "medium.wav"))
	s.key(core.KeyTalk, false)
	lines := strings.Split(s.host.Text(), "\n")
	if len(lines) != 2 || wordErrors(lines[1], mediumText) > 2 || !strings.HasSuffix(words(lines[1]), "serialization") {
		t.Fatalf("second line: %q", s.host.Text())
	}
	if !strings.HasPrefix(lines[1], "The ") {
		t.Errorf("a new line should start with a capital: %q", lines[1])
	}
}

func TestCancelDiscardsWhatWasNotTyped(t *testing.T) {
	s := newSession(t, model(t, "modified_beam_search"))
	s.tap(core.KeyTalk)
	s.feed(clip(t, "short.wav")[:core.SampleRate]) // one second: nothing stable yet
	s.tap(core.KeyCancel)
	typed := s.host.Text()
	s.silence(3 * time.Second)
	if s.host.Text() != typed {
		t.Errorf("text changed after cancel: %q -> %q", typed, s.host.Text())
	}
	if len(words(typed)) >= len(shortText) {
		t.Errorf("cancel after one second should not have typed the whole sentence: %q", typed)
	}

	// the pad still works afterwards
	s.key(core.KeyTalk, true)
	s.feed(clip(t, "short.wav"))
	s.key(core.KeyTalk, false)
	if !strings.HasSuffix(words(s.host.Text()), "pods on each node") {
		t.Errorf("dictation after a cancel: %q", s.host.Text())
	}
}

func TestSilenceTypesNothing(t *testing.T) {
	s := newSession(t, model(t, "modified_beam_search"))
	s.tap(core.KeyTalk)
	s.silence(15 * time.Second) // long enough to pass the idle restart
	s.tap(core.KeyTalk)
	if s.host.Text() != "" {
		t.Errorf("silence produced %q", s.host.Text())
	}
}

func TestFilesReportsWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, _, err := Files(dir); err == nil || !strings.Contains(err.Error(), "encoder") {
		t.Fatalf("empty folder: %v", err)
	}
	for _, f := range []string{"encoder.onnx", "encoder.int8.onnx", "decoder.onnx", "joiner.onnx"} {
		_ = os.WriteFile(filepath.Join(dir, f), nil, 0o600)
	}
	if _, _, _, _, err := Files(dir); err == nil || !strings.Contains(err.Error(), "tokens.txt") {
		t.Fatalf("no tokens: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "tokens.txt"), nil, 0o600)
	enc, _, _, _, err := Files(dir)
	if err != nil || filepath.Base(enc) != "encoder.int8.onnx" {
		t.Errorf("want the int8 encoder, got %q, %v", enc, err)
	}
}
