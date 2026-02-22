// Package transcribe provides speech-to-text backends.
//
// Supported backends:
//   - whisper: whisper.cpp via Go bindings (default)
//   - parakeet: Parakeet TDT 0.6B v2 via CoreML
package transcribe

import (
	"fmt"

	"github.com/chaz8081/gostt-writer/internal/config"
)

// Transcriber converts audio samples to text.
type Transcriber interface {
	// Process transcribes mono 16kHz float32 audio samples to text.
	Process(samples []float32) (string, error)
	// Close releases backend resources.
	Close() error
}

// New creates a Transcriber based on the config backend setting.
func New(cfg *config.TranscribeConfig) (Transcriber, error) {
	switch cfg.Backend {
	case "parakeet":
		return NewParakeetTranscriber(cfg.ParakeetModelDir)
	case "whisper", "":
		return NewWhisperTranscriber(cfg.ModelPath)
	default:
		return nil, fmt.Errorf("transcribe: unknown backend %q (supported: whisper, parakeet)", cfg.Backend)
	}
}
