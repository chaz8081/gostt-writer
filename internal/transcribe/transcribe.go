// Package transcribe wraps whisper.cpp Go bindings for speech-to-text.
// It loads the model once at startup and exposes Process([]float32) -> (string, error).
package transcribe

import (
	"fmt"
	"io"
	"strings"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// Transcriber wraps a whisper model and provides speech-to-text via Process.
type Transcriber struct {
	model whisper.Model
}

// NewTranscriber loads a whisper model from the given path.
// The caller must call Close() when done.
func NewTranscriber(modelPath string) (*Transcriber, error) {
	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("transcribe: load model %q: %w", modelPath, err)
	}
	return &Transcriber{model: model}, nil
}

// Close releases the whisper model resources.
func (t *Transcriber) Close() error {
	if t.model != nil {
		return t.model.Close()
	}
	return nil
}

// Process transcribes mono 16kHz float32 audio samples to text.
func (t *Transcriber) Process(samples []float32) (string, error) {
	ctx, err := t.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("transcribe: create context: %w", err)
	}

	if err := ctx.Process(samples, nil, nil, nil); err != nil {
		return "", fmt.Errorf("transcribe: process: %w", err)
	}

	var segments []string
	for {
		seg, err := ctx.NextSegment()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("transcribe: next segment: %w", err)
		}
		segments = append(segments, seg.Text)
	}

	return strings.TrimSpace(strings.Join(segments, " ")), nil
}
