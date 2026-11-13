package speech

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
)

// VoiceConfig selects the text-to-speech voice.
type VoiceConfig struct {
	Dir     string  // folder with a Piper voice: one .onnx file, tokens.txt and espeak-ng-data
	Threads int     // CPU threads
	Speed   float32 // 1 is normal; lower is slower
	Cache   string  // folder for finished clips; empty keeps none
}

// Voice turns lines into audio. Each line is made once and then kept as a
// WAV file in the cache, so a line the device has said before starts
// playing at once. The model is loaded only when a line is missing.
type Voice struct {
	cfg VoiceConfig
	mu  sync.Mutex
	tts *sherpa.OfflineTts
}

// NewVoice returns a Voice. It does not load the model yet.
func NewVoice(cfg VoiceConfig) (*Voice, error) {
	if cfg.Speed <= 0 {
		cfg.Speed = 1
	}
	if cfg.Cache != "" {
		if err := os.MkdirAll(cfg.Cache, 0o755); err != nil {
			return nil, fmt.Errorf("speech: voice cache: %w", err)
		}
	}
	return &Voice{cfg: cfg}, nil
}

// clipPath is where a line's clip is kept. The name depends on the voice
// and speed, so changing either makes new clips.
func (v *Voice) clipPath(text string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%.2f|%s", filepath.Base(v.cfg.Dir), v.cfg.Speed, text))
	return filepath.Join(v.cfg.Cache, hex.EncodeToString(sum[:10])+".wav")
}

// Cached reports whether a line's clip has already been made.
func (v *Voice) Cached(text string) bool {
	if v.cfg.Cache == "" {
		return false
	}
	_, err := os.Stat(v.clipPath(text))
	return err == nil
}

// Clip returns the audio for a line, making it if needed.
func (v *Voice) Clip(text string) (samples []float32, rate int, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cfg.Cache != "" {
		if samples, rate, err := audio.ReadWAV(v.clipPath(text)); err == nil {
			return samples, rate, nil
		}
	}
	if err := v.load(); err != nil {
		return nil, 0, err
	}
	out := v.tts.Generate(text, 0, v.cfg.Speed)
	if out == nil || len(out.Samples) == 0 {
		return nil, 0, fmt.Errorf("speech: the voice produced no audio for %q", text)
	}
	if v.cfg.Cache != "" {
		if err := audio.WriteWAV(v.clipPath(text), out.Samples, out.SampleRate); err != nil {
			return nil, 0, err
		}
	}
	return out.Samples, out.SampleRate, nil
}

func (v *Voice) load() error {
	if v.tts != nil {
		return nil
	}
	model, err := modelFile(v.cfg.Dir, "*.onnx")
	if err != nil {
		return err
	}
	for _, need := range []string{"tokens.txt", "espeak-ng-data"} {
		if _, err := os.Stat(filepath.Join(v.cfg.Dir, need)); err != nil {
			return fmt.Errorf("speech: no %s in %s", need, v.cfg.Dir)
		}
	}
	var c sherpa.OfflineTtsConfig
	c.Model.Vits = sherpa.OfflineTtsVitsModelConfig{
		Model:       model,
		Tokens:      filepath.Join(v.cfg.Dir, "tokens.txt"),
		DataDir:     filepath.Join(v.cfg.Dir, "espeak-ng-data"),
		NoiseScale:  0.667,
		NoiseScaleW: 0.8,
		LengthScale: 1,
	}
	c.Model.NumThreads = v.cfg.Threads
	c.Model.Provider = "cpu"
	c.MaxNumSentences = 1
	tts := sherpa.NewOfflineTts(&c)
	if tts == nil {
		return fmt.Errorf("speech: the voice in %s could not be loaded", v.cfg.Dir)
	}
	v.tts = tts
	return nil
}

// Close frees the model if it was loaded.
func (v *Voice) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.tts != nil {
		sherpa.DeleteOfflineTts(v.tts)
		v.tts = nil
	}
}
