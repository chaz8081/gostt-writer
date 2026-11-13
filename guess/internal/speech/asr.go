package speech

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
)

// ASRConfig selects and tunes the speech recognition model.
type ASRConfig struct {
	Dir         string        // folder with the model files
	Threads     int           // CPU threads
	Decoding    string        // greedy_search or modified_beam_search
	TailPadding time.Duration // silence fed by Finish so the last word comes out
}

// ASR is a streaming transducer model run through sherpa-onnx. It is not
// safe for concurrent use.
type ASR struct {
	rec    *sherpa.OnlineRecognizer
	stream *sherpa.OnlineStream
	tail   []float32
}

var _ Recognizer = (*ASR)(nil)

// modelFile returns the first file in dir matching one of the patterns.
func modelFile(dir string, patterns ...string) (string, error) {
	for _, pattern := range patterns {
		if m, _ := filepath.Glob(filepath.Join(dir, pattern)); len(m) > 0 {
			return m[0], nil
		}
	}
	return "", fmt.Errorf("speech: no %s in %s", patterns[len(patterns)-1], dir)
}

// NewASR loads the model. Quantised (int8) files are preferred when both
// kinds are present.
func NewASR(cfg ASRConfig) (*ASR, error) {
	var files [3]string
	for i, part := range []string{"encoder", "decoder", "joiner"} {
		f, err := modelFile(cfg.Dir, part+".int8.onnx", part+".onnx", part+"*.int8.onnx", part+"*.onnx")
		if err != nil {
			return nil, err
		}
		files[i] = f
	}
	tokens := filepath.Join(cfg.Dir, "tokens.txt")
	if _, err := os.Stat(tokens); err != nil {
		return nil, fmt.Errorf("speech: no tokens.txt in %s", cfg.Dir)
	}
	c := sherpa.OnlineRecognizerConfig{
		FeatConfig:     sherpa.FeatureConfig{SampleRate: audio.SampleRate, FeatureDim: 80},
		DecodingMethod: cfg.Decoding,
		MaxActivePaths: 4,
	}
	c.ModelConfig.Transducer = sherpa.OnlineTransducerModelConfig{Encoder: files[0], Decoder: files[1], Joiner: files[2]}
	c.ModelConfig.Tokens = tokens
	c.ModelConfig.NumThreads = cfg.Threads
	c.ModelConfig.Provider = "cpu"
	rec := sherpa.NewOnlineRecognizer(&c)
	if rec == nil {
		return nil, fmt.Errorf("speech: the model in %s could not be loaded", cfg.Dir)
	}
	a := &ASR{rec: rec, tail: make([]float32, int(cfg.TailPadding*audio.SampleRate/time.Second))}
	a.stream = sherpa.NewOnlineStream(rec)
	return a, nil
}

// Accept feeds audio and decodes what is ready.
func (a *ASR) Accept(samples []float32) {
	if len(samples) == 0 {
		return
	}
	a.stream.AcceptWaveform(audio.SampleRate, samples)
	a.decode()
}

// Text returns the words recognised so far.
func (a *ASR) Text() string {
	return strings.TrimSpace(a.rec.GetResult(a.stream).Text)
}

// Finish feeds trailing silence, which the model needs before it will emit
// the last word, and returns the final words.
func (a *ASR) Finish() string {
	if len(a.tail) > 0 {
		a.stream.AcceptWaveform(audio.SampleRate, a.tail)
	}
	a.stream.InputFinished()
	a.decode()
	return a.Text()
}

// Reset discards the current utterance and starts a new one.
func (a *ASR) Reset() {
	sherpa.DeleteOnlineStream(a.stream)
	a.stream = sherpa.NewOnlineStream(a.rec)
}

// Close frees the model.
func (a *ASR) Close() {
	sherpa.DeleteOnlineStream(a.stream)
	sherpa.DeleteOnlineRecognizer(a.rec)
}

func (a *ASR) decode() {
	for a.rec.IsReady(a.stream) {
		a.rec.Decode(a.stream)
	}
}
