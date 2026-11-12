//go:build linux

// Package asr runs the streaming speech model through sherpa-onnx.
package asr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"github.com/chaz8081/gostt-writer/pad/internal/core"
)

// Config selects and tunes the model.
type Config struct {
	Dir         string        // folder with the encoder, decoder, joiner and tokens files
	Threads     int           // CPU threads
	Decoding    string        // greedy_search or modified_beam_search
	TailPadding time.Duration // silence fed by Finish so the last words come out
}

// Recognizer is a streaming transducer model. It implements core.Recognizer
// and is not safe for concurrent use.
type Recognizer struct {
	rec    *sherpa.OnlineRecognizer
	stream *sherpa.OnlineStream
	tail   []float32
}

var _ core.Recognizer = (*Recognizer)(nil)

// Files returns the model files in dir, or an error naming what is missing.
// Quantised (int8) files are preferred when both kinds are present.
func Files(dir string) (encoder, decoder, joiner, tokens string, err error) {
	find := func(part string) (string, error) {
		for _, pattern := range []string{part + ".int8.onnx", part + ".onnx", part + "*.int8.onnx", part + "*.onnx"} {
			if m, _ := filepath.Glob(filepath.Join(dir, pattern)); len(m) > 0 {
				return m[0], nil
			}
		}
		return "", fmt.Errorf("asr: no %s*.onnx in %s", part, dir)
	}
	if encoder, err = find("encoder"); err != nil {
		return
	}
	if decoder, err = find("decoder"); err != nil {
		return
	}
	if joiner, err = find("joiner"); err != nil {
		return
	}
	tokens = filepath.Join(dir, "tokens.txt")
	if _, statErr := os.Stat(tokens); statErr != nil {
		err = fmt.Errorf("asr: no tokens.txt in %s", dir)
	}
	return
}

// New loads the model.
func New(cfg Config) (*Recognizer, error) {
	encoder, decoder, joiner, tokens, err := Files(cfg.Dir)
	if err != nil {
		return nil, err
	}
	c := sherpa.OnlineRecognizerConfig{
		FeatConfig:     sherpa.FeatureConfig{SampleRate: core.SampleRate, FeatureDim: 80},
		DecodingMethod: cfg.Decoding,
		MaxActivePaths: 4,
	}
	c.ModelConfig.Transducer = sherpa.OnlineTransducerModelConfig{Encoder: encoder, Decoder: decoder, Joiner: joiner}
	c.ModelConfig.Tokens = tokens
	c.ModelConfig.NumThreads = cfg.Threads
	c.ModelConfig.Provider = "cpu"
	rec := sherpa.NewOnlineRecognizer(&c)
	if rec == nil {
		return nil, fmt.Errorf("asr: the model in %s could not be loaded", cfg.Dir)
	}
	r := &Recognizer{rec: rec, tail: make([]float32, int(cfg.TailPadding*core.SampleRate/time.Second))}
	r.stream = sherpa.NewOnlineStream(rec)
	return r, nil
}

// Accept feeds audio and decodes what is ready.
func (r *Recognizer) Accept(samples []float32) {
	if len(samples) == 0 {
		return
	}
	r.stream.AcceptWaveform(core.SampleRate, samples)
	r.decode()
}

// Text returns the hypothesis for the current stream.
func (r *Recognizer) Text() string {
	return strings.TrimSpace(r.rec.GetResult(r.stream).Text)
}

// Finish feeds trailing silence, which the model needs before it will emit
// the last word or two, and returns the final hypothesis.
func (r *Recognizer) Finish() string {
	if len(r.tail) > 0 {
		r.stream.AcceptWaveform(core.SampleRate, r.tail)
	}
	r.stream.InputFinished()
	r.decode()
	return r.Text()
}

// Reset discards the current stream and starts a new one.
func (r *Recognizer) Reset() {
	sherpa.DeleteOnlineStream(r.stream)
	r.stream = sherpa.NewOnlineStream(r.rec)
}

// Close frees the model.
func (r *Recognizer) Close() {
	sherpa.DeleteOnlineStream(r.stream)
	sherpa.DeleteOnlineRecognizer(r.rec)
}

func (r *Recognizer) decode() {
	for r.rec.IsReady(r.stream) {
		r.rec.Decode(r.stream)
	}
}
