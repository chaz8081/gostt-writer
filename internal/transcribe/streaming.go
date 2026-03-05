package transcribe

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// StreamResult holds the result of a streaming transcription step.
type StreamResult struct {
	Text    string // full accumulated text so far
	Delta   string // new text since last result (for display/logging)
	IsFinal bool   // true for the final transcription after recording stops
}

// StreamingTranscriber performs sliding-window streaming transcription using
// whisper.cpp. It periodically transcribes audio snapshots during recording
// and emits incremental text deltas.
type StreamingTranscriber struct {
	model    whisper.Model
	stepMs   int
	lengthMs int
	keepMs   int

	mu       sync.Mutex
	prevText string // accumulated text from previous windows
	cancel   context.CancelFunc
	done     chan struct{}
}

// AudioFunc returns the current audio buffer snapshot (mono 16kHz float32).
type AudioFunc func() []float32

// DeltaFunc is called with each incremental text update.
type DeltaFunc func(backspaces int, newText string)

// NewStreamingTranscriber creates a streaming transcriber that shares the
// given whisper model. The model must remain open for the lifetime of this
// transcriber.
func NewStreamingTranscriber(model whisper.Model, stepMs, lengthMs, keepMs int) *StreamingTranscriber {
	return &StreamingTranscriber{
		model:    model,
		stepMs:   stepMs,
		lengthMs: lengthMs,
		keepMs:   keepMs,
	}
}

// Start begins the streaming transcription loop. It calls audioFn every
// stepMs milliseconds to get the current audio, transcribes a sliding window,
// and calls deltaFn with incremental text updates. Blocks until Stop() is
// called or ctx is cancelled. After stopping, performs one final transcription
// of all audio.
func (s *StreamingTranscriber) Start(audioFn AudioFunc, deltaFn DeltaFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.cancel = cancel
	s.prevText = ""
	s.done = make(chan struct{})
	s.mu.Unlock()

	go func() {
		defer close(s.done)
		s.run(ctx, audioFn, deltaFn)
	}()
}

// Stop signals the streaming loop to stop and waits for the final
// transcription to complete.
func (s *StreamingTranscriber) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// FinalText returns the accumulated transcription text after Stop() completes.
// Used by the rewrite feature to know how many characters to backspace.
func (s *StreamingTranscriber) FinalText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prevText
}

func (s *StreamingTranscriber) run(ctx context.Context, audioFn AudioFunc, deltaFn DeltaFunc) {
	ticker := time.NewTicker(time.Duration(s.stepMs) * time.Millisecond)
	defer ticker.Stop()

	sampleRate := 16000 // whisper expects 16kHz
	windowSamples := sampleRate * s.lengthMs / 1000
	keepSamples := sampleRate * s.keepMs / 1000

	var prompt string // context carry-forward from previous window

	for {
		select {
		case <-ctx.Done():
			// Final transcription of all audio
			s.finalTranscribe(audioFn, deltaFn, prompt)
			return
		case <-ticker.C:
			samples := audioFn()
			if len(samples) == 0 {
				continue
			}

			// Build window: take last lengthMs of audio
			window := samples
			if len(window) > windowSamples {
				// Keep overlap from previous window for continuity
				start := len(window) - windowSamples
				if keepSamples > 0 && start > keepSamples {
					start -= keepSamples
				}
				window = window[start:]
			}

			start := time.Now()
			text, err := s.transcribeWindow(window, prompt)
			elapsed := time.Since(start)
			if err != nil {
				slog.Error("streaming: transcribe step failed", "error", err)
				continue
			}

			if elapsed > time.Duration(s.stepMs)*time.Millisecond {
				slog.Warn("streaming: transcription slower than step interval",
					"elapsed", elapsed.Round(time.Millisecond),
					"step_ms", s.stepMs)
			}

			if text == "" {
				continue
			}

			// Use last segment as prompt for next window
			prompt = text

			// Compute and emit delta
			s.mu.Lock()
			backspaces, appendText := computeDelta(s.prevText, text)
			if backspaces > 0 || appendText != "" {
				s.prevText = text
				s.mu.Unlock()
				deltaFn(backspaces, appendText)
				slog.Debug("streaming: delta",
					"backspaces", backspaces,
					"append", appendText,
					"elapsed", elapsed.Round(time.Millisecond))
			} else {
				s.mu.Unlock()
			}
		}
	}
}

func (s *StreamingTranscriber) finalTranscribe(audioFn AudioFunc, deltaFn DeltaFunc, prompt string) {
	samples := audioFn()
	if len(samples) == 0 {
		return
	}

	text, err := s.transcribeWindow(samples, prompt)
	if err != nil {
		slog.Error("streaming: final transcribe failed", "error", err)
		return
	}

	if text == "" {
		return
	}

	s.mu.Lock()
	backspaces, appendText := computeDelta(s.prevText, text)
	s.prevText = text
	s.mu.Unlock()

	if backspaces > 0 || appendText != "" {
		deltaFn(backspaces, appendText)
	}

	slog.Info("streaming: final transcription", "text", text)
}

func (s *StreamingTranscriber) transcribeWindow(samples []float32, prompt string) (string, error) {
	ctx, err := s.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("streaming: create context: %w", err)
	}

	if prompt != "" {
		ctx.SetInitialPrompt(prompt)
	}

	if err := ctx.Process(samples, nil, nil, nil); err != nil {
		return "", fmt.Errorf("streaming: process: %w", err)
	}

	var segments []string
	for {
		seg, err := ctx.NextSegment()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("streaming: next segment: %w", err)
		}
		segments = append(segments, seg.Text)
	}

	return strings.TrimSpace(strings.Join(segments, " ")), nil
}
