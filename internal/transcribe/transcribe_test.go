package transcribe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-audio/wav"
)

// modelPath resolves the path to the whisper model relative to the project root.
// Tests must be run from the project root (or via make test).
func modelPath(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location to find models/
	// When run via `make test` or `go test ./...` from project root,
	// the working directory is the package directory.
	// We go up two levels: internal/transcribe -> project root
	path := filepath.Join("..", "..", "models", "ggml-base.en.bin")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("model not found at %s (run 'make model' first): %v", path, err)
	}
	return path
}

func TestNewTranscriber(t *testing.T) {
	path := modelPath(t)

	tr, err := NewTranscriber(path)
	if err != nil {
		t.Fatalf("NewTranscriber(%q) returned error: %v", path, err)
	}
	if tr == nil {
		t.Fatal("NewTranscriber returned nil without error")
	}

	err = tr.Close()
	if err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}

func TestNewTranscriberBadPath(t *testing.T) {
	_, err := NewTranscriber("/nonexistent/model.bin")
	if err == nil {
		t.Fatal("NewTranscriber with bad path should return error")
	}
}

// jfkSamples loads the JFK sample WAV and returns mono float32 samples.
func jfkSamples(t *testing.T) []float32 {
	t.Helper()
	wavPath := filepath.Join("..", "..", "third_party", "whisper.cpp", "samples", "jfk.wav")
	f, err := os.Open(wavPath)
	if err != nil {
		t.Skipf("JFK sample not found at %s: %v", wavPath, err)
	}
	defer f.Close()

	dec := wav.NewDecoder(f)
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		t.Fatalf("decode WAV: %v", err)
	}

	// Convert int samples to float32 normalized to [-1.0, 1.0]
	samples := make([]float32, len(buf.Data))
	for i, s := range buf.Data {
		samples[i] = float32(s) / 32768.0
	}
	return samples
}

func TestProcessJFK(t *testing.T) {
	path := modelPath(t)
	samples := jfkSamples(t)

	tr, err := NewTranscriber(path)
	if err != nil {
		t.Fatalf("NewTranscriber: %v", err)
	}
	defer tr.Close()

	text, err := tr.Process(samples)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}

	lower := strings.ToLower(text)
	if !strings.Contains(lower, "ask not what your country") {
		t.Errorf("expected transcript to contain 'ask not what your country', got: %q", text)
	}
}

func TestProcessEmptyAudio(t *testing.T) {
	path := modelPath(t)

	tr, err := NewTranscriber(path)
	if err != nil {
		t.Fatalf("NewTranscriber: %v", err)
	}
	defer tr.Close()

	// Empty/silent audio should not error, just return empty-ish text
	silence := make([]float32, 16000) // 1 second of silence
	text, err := tr.Process(silence)
	if err != nil {
		t.Fatalf("Process on silence returned error: %v", err)
	}
	// We don't assert the exact text — whisper may hallucinate on silence.
	// Just verify it didn't crash and returned something.
	_ = text
}
