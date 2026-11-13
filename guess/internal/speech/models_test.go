package speech

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
	"github.com/chaz8081/gostt-writer/guess/internal/game"
)

// These tests run the real models: a reply is played into the listener as
// if from a microphone, and the words that come back must mean what was
// said. Set GOSTT_GUESS_ASR_DIR and
// GOSTT_GUESS_VOICE_DIR to the two model folders to run them.
func models(t *testing.T) (*ASR, *Voice) {
	t.Helper()
	asrDir, voiceDir := os.Getenv("GOSTT_GUESS_ASR_DIR"), os.Getenv("GOSTT_GUESS_VOICE_DIR")
	if asrDir == "" || voiceDir == "" {
		t.Skip("set GOSTT_GUESS_ASR_DIR and GOSTT_GUESS_VOICE_DIR to run")
	}
	asr, err := NewASR(ASRConfig{Dir: asrDir, Threads: 2, Decoding: "modified_beam_search", TailPadding: 1500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(asr.Close)
	voice, err := NewVoice(VoiceConfig{Dir: voiceDir, Threads: 2, Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(voice.Close)
	return asr, voice
}

// resample converts to the microphone rate by linear interpolation.
func resample(in []float32, from int) []float32 {
	if from == audio.SampleRate {
		return in
	}
	out := make([]float32, len(in)*audio.SampleRate/from)
	for i := range out {
		pos := float64(i) * float64(from) / audio.SampleRate
		j := int(pos)
		if j+1 >= len(in) {
			out[i] = in[len(in)-1]
			continue
		}
		f := float32(pos - float64(j))
		out[i] = in[j]*(1-f) + in[j+1]*f
	}
	return out
}

// hearClip plays a clip into a listener, with silence around it, and
// returns the words heard.
func hearClip(t *testing.T, asr *ASR, clip []float32, rate int) string {
	t.Helper()
	mic := make(chan []float32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sound := make([]float32, audio.SampleRate/2)
		sound = append(sound, resample(clip, rate)...)
		sound = append(sound, make([]float32, 3*audio.SampleRate)...)
		audio.Play(ctx, sound, false, mic)
	}()
	l := &Listener{Rec: asr, Mic: mic, Level: 0.02, Quiet: 600 * time.Millisecond, MaxSpeech: 5 * time.Second}
	heard, err := l.Hear(ctx, 4*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return heard
}

func TestSpokenRepliesAreUnderstood(t *testing.T) {
	asr, voice := models(t)
	// Only longer replies: the synthetic voice says a lone "yes" badly
	// enough that no model hears it reliably. Lone words are tested with
	// real recordings in TestRealPeopleSayingYesAndNo.
	cases := map[string]game.Reply{
		"Yes it does.": game.Yes, "I think so.": game.Yes,
		"No, it doesn't.": game.No, "Not really.": game.No, "I don't think so.": game.No,
		"I don't know.": game.DontKnow, "I'm not sure.": game.DontKnow,
		"Say that again.": game.Repeat, "Start over.": game.Restart,
	}
	for said, want := range cases {
		clip, rate, err := voice.Clip(said)
		if err != nil {
			t.Fatal(err)
		}
		heard := hearClip(t, asr, clip, rate)
		got := game.ParseReply(heard)
		t.Logf("said %-18q heard %-22q meaning %v", said, heard, got)
		if got != want {
			t.Errorf("said %q, heard %q, understood as %v, want %v", said, heard, got, want)
		}
	}
}

func TestSilenceIsNotAReply(t *testing.T) {
	asr, _ := models(t)
	mic := make(chan []float32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go audio.Play(ctx, make([]float32, 10*audio.SampleRate), false, mic)
	l := &Listener{Rec: asr, Mic: mic, Level: 0.02, Quiet: 600 * time.Millisecond, MaxSpeech: 5 * time.Second}
	if heard, err := l.Hear(ctx, 3*time.Second); heard != "" || err != nil {
		t.Fatalf("heard %q, err %v", heard, err)
	}
}

func TestVoiceKeepsClips(t *testing.T) {
	_, voice := models(t)
	if voice.Cached("Hello there.") {
		t.Fatal("cached before it was made")
	}
	a, rate, err := voice.Clip("Hello there.")
	if err != nil || rate < 8000 || len(a) < rate/4 {
		t.Fatalf("%d samples at %d Hz, err %v", len(a), rate, err)
	}
	if !voice.Cached("Hello there.") {
		t.Fatal("not cached after it was made")
	}
	files, _ := filepath.Glob(filepath.Join(voice.cfg.Cache, "*.wav"))
	b, _, err := voice.Clip("Hello there.")
	if err != nil || len(files) != 1 || len(b) != len(a) {
		t.Fatalf("%d files, %d vs %d samples, err %v", len(files), len(b), len(a), err)
	}
}

// Real people saying one word. GOSTT_GUESS_WORDS_DIR must hold folders
// named "yes" and "no" of 16 kHz WAV files, such as those in the Google
// Speech Commands test set. A reply heard as the opposite answer is the
// failure that matters most; one that is not understood is only asked again.
func TestRealPeopleSayingYesAndNo(t *testing.T) {
	dir := os.Getenv("GOSTT_GUESS_WORDS_DIR")
	if dir == "" {
		t.Skip("set GOSTT_GUESS_WORDS_DIR to run")
	}
	asr, _ := models(t)
	for word, want := range map[string]game.Reply{"yes": game.Yes, "no": game.No} {
		files, _ := filepath.Glob(filepath.Join(dir, word, "*.wav"))
		if len(files) == 0 {
			t.Fatalf("no WAV files in %s/%s", dir, word)
		}
		right, opposite, silent := 0, 0, 0
		misheard := map[string]int{}
		for _, f := range files {
			clip, rate, err := audio.ReadWAV(f)
			if err != nil {
				t.Fatal(err)
			}
			heard := hearClip(t, asr, clip, rate)
			switch got := game.ParseReply(heard); {
			case got == want:
				right++
			case heard == "":
				silent++
			case got == game.Yes || got == game.No:
				opposite++
				misheard["!"+heard]++
			default:
				misheard[heard]++
			}
		}
		n := len(files)
		t.Logf("%q: %d clips, %d right (%.1f%%), %d heard as the opposite, %d not heard at all, %d unclear",
			word, n, right, 100*float64(right)/float64(n), opposite, silent, n-right-opposite-silent)
		t.Logf("  misheard as: %v", misheard)
	}
}
