package transcribe

import (
	"fmt"
	"io"
	"strings"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// WhisperTranscriber wraps a whisper.cpp model for speech-to-text.
type WhisperTranscriber struct {
	model whisper.Model
}

// NewWhisperTranscriber loads a whisper model from the given path.
// The caller must call Close() when done.
func NewWhisperTranscriber(modelPath string) (*WhisperTranscriber, error) {
	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("transcribe: load whisper model %q: %w", modelPath, err)
	}
	return &WhisperTranscriber{model: model}, nil
}

// Close releases the whisper model resources.
func (t *WhisperTranscriber) Close() error {
	if t.model != nil {
		return t.model.Close()
	}
	return nil
}

// Process transcribes mono 16kHz float32 audio samples to text.
func (t *WhisperTranscriber) Process(samples []float32) (string, error) {
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
