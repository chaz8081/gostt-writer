package transcribe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parakeetModelDir returns the path to the parakeet model directory, skipping if not found.
func parakeetModelDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "models", "parakeet-tdt-v2")
	if _, err := os.Stat(filepath.Join(dir, "Encoder.mlmodelc")); err != nil {
		t.Skipf("Parakeet models not found at %s (run 'make parakeet-model' first)", dir)
	}
	return dir
}

func TestPadAudioShorter(t *testing.T) {
	input := []float32{1.0, 2.0, 3.0}
	result := padAudio(input, 5)
	if len(result) != 5 {
		t.Fatalf("padAudio len = %d, want 5", len(result))
	}
	// Original values preserved
	if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
		t.Errorf("original values not preserved: %v", result[:3])
	}
	// Padding is zero
	if result[3] != 0.0 || result[4] != 0.0 {
		t.Errorf("padding not zero: %v", result[3:])
	}
}

func TestPadAudioExact(t *testing.T) {
	input := []float32{1.0, 2.0, 3.0}
	result := padAudio(input, 3)
	if len(result) != 3 {
		t.Fatalf("padAudio len = %d, want 3", len(result))
	}
	if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
		t.Errorf("values changed: %v", result)
	}
}

func TestPadAudioLonger(t *testing.T) {
	input := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
	result := padAudio(input, 3)
	if len(result) != 3 {
		t.Fatalf("padAudio len = %d, want 3", len(result))
	}
	if result[0] != 1.0 || result[1] != 2.0 || result[2] != 3.0 {
		t.Errorf("truncated values wrong: %v", result)
	}
}

func TestNewParakeetTranscriber(t *testing.T) {
	dir := parakeetModelDir(t)

	tr, err := NewParakeetTranscriber(dir)
	if err != nil {
		t.Fatalf("NewParakeetTranscriber: %v", err)
	}
	defer tr.Close()
}

func TestParakeetProcessJFK(t *testing.T) {
	dir := parakeetModelDir(t)
	samples := jfkSamples(t)

	tr, err := NewParakeetTranscriber(dir)
	if err != nil {
		t.Fatalf("NewParakeetTranscriber: %v", err)
	}
	defer tr.Close()

	text, err := tr.Process(samples)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	lower := strings.ToLower(text)
	if !strings.Contains(lower, "ask not what your country") {
		t.Errorf("expected transcript to contain 'ask not what your country', got: %q", text)
	}
}
