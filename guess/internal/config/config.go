// Package config loads the device's settings from a YAML file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// The folder names of the default models.
const (
	ASRModel   = "sherpa-onnx-streaming-zipformer-en-kroko-2025-08-06"
	VoiceModel = "vits-piper-en_US-amy-low"
)

// Config is everything the device can be told.
type Config struct {
	Game   Game   `yaml:"game"`
	Hear   Hear   `yaml:"hear"`
	Voice  Voice  `yaml:"voice"`
	Audio  Audio  `yaml:"audio"`
	Listen Listen `yaml:"listen"`
	GPIO   GPIO   `yaml:"gpio"`
}

// Game selects the pack and the rules of a round.
type Game struct {
	Pack         string  `yaml:"pack"`          // the pack file
	Collection   string  `yaml:"collection"`    // where found items are remembered; empty forgets them at power off
	MaxQuestions int     `yaml:"max_questions"` // questions before the device must guess
	MaxGuesses   int     `yaml:"max_guesses"`   // wrong guesses before it gives up
	Confidence   float64 `yaml:"confidence"`    // guess once one item is this likely, 0.5 to 0.99
	Mistakes     float64 `yaml:"mistakes"`      // chance that an answer is wrong, 0.01 to 0.3
}

// Hear selects the speech recognition model.
type Hear struct {
	Dir           string `yaml:"dir"`
	Threads       int    `yaml:"threads"`
	Decoding      string `yaml:"decoding"` // greedy_search or modified_beam_search
	TailPaddingMs int    `yaml:"tail_padding_ms"`
}

// Voice selects the text-to-speech voice.
type Voice struct {
	Dir     string  `yaml:"dir"`
	Threads int     `yaml:"threads"`
	Speed   float32 `yaml:"speed"` // 1 is normal, lower is slower
	Cache   string  `yaml:"cache"` // folder for finished clips
}

// Audio selects the microphone and speaker.
type Audio struct {
	// Capture must write raw 16-bit little-endian mono 16 kHz audio to its
	// standard output.
	Capture string  `yaml:"capture"`
	Gain    float32 `yaml:"gain"` // microphone volume
	// Play must read raw 16-bit little-endian mono audio on its standard
	// input; {rate} is replaced by the sample rate.
	Play   string  `yaml:"play"`
	Volume float32 `yaml:"volume"` // speaker volume
}

// Listen tunes how replies are heard.
type Listen struct {
	WaitMs      int     `yaml:"wait_ms"`       // how long to wait for a reply before asking again
	Tries       int     `yaml:"tries"`         // how often a question is asked before the device stops
	Level       float64 `yaml:"level"`         // loudness that counts as speech, 0 to 1
	QuietMs     int     `yaml:"quiet_ms"`      // silence that ends a reply
	MaxSpeechMs int     `yaml:"max_speech_ms"` // longest reply
	SkipMs      int     `yaml:"skip_ms"`       // ignored after the device stops speaking
}

// GPIO maps buttons to BCM GPIO numbers. A button connects its pin to
// ground. Minus one leaves a button or the light unused.
type GPIO struct {
	Chip       string `yaml:"chip"` // empty: found automatically
	Yes        int    `yaml:"yes"`
	No         int    `yaml:"no"`
	NotSure    int    `yaml:"not_sure"`
	Play       int    `yaml:"play"`
	Light      int    `yaml:"light"` // lit while listening
	DebounceMs int    `yaml:"debounce_ms"`
}

// Default returns the settings used when a file leaves a value out.
func Default() Config {
	return Config{
		Game: Game{
			Pack: "/etc/gostt-guess/pack.yaml", Collection: "/var/lib/gostt-guess/found.json",
			MaxQuestions: 20, MaxGuesses: 3, Confidence: 0.8, Mistakes: 0.05,
		},
		Hear:  Hear{Dir: "/opt/gostt-guess/models/" + ASRModel, Threads: 2, Decoding: "modified_beam_search", TailPaddingMs: 1500},
		Voice: Voice{Dir: "/opt/gostt-guess/models/" + VoiceModel, Threads: 2, Speed: 0.9, Cache: "/var/lib/gostt-guess/clips"},
		Audio: Audio{
			Capture: "arecord -q -D default -f S16_LE -r 16000 -c 1 -t raw", Gain: 1,
			Play: "aplay -q -D default -t raw -f S16_LE -c 1 -r {rate}", Volume: 1,
		},
		Listen: Listen{WaitMs: 8000, Tries: 3, Level: 0.02, QuietMs: 600, MaxSpeechMs: 5000, SkipMs: 200},
		GPIO:   GPIO{Yes: 17, No: 27, NotSure: 22, Play: 23, Light: 26, DebounceMs: 10},
	}
}

// Load reads the file at path over the defaults. A missing file is an error
// only when required.
func Load(path string, required bool) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !required {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && err.Error() != "EOF" {
		return cfg, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}

// Validate reports the first setting that cannot work.
func (c Config) Validate() error {
	bad := func(format string, a ...any) error { return fmt.Errorf("config: "+format, a...) }
	switch {
	case c.Game.MaxQuestions < 1 || c.Game.MaxGuesses < 1:
		return bad("game.max_questions and game.max_guesses must be at least 1")
	case c.Game.Confidence < 0.5 || c.Game.Confidence > 0.99:
		return bad("game.confidence must be between 0.5 and 0.99, not %v", c.Game.Confidence)
	case c.Game.Mistakes < 0.01 || c.Game.Mistakes > 0.3:
		return bad("game.mistakes must be between 0.01 and 0.3, not %v", c.Game.Mistakes)
	case c.Hear.Decoding != "greedy_search" && c.Hear.Decoding != "modified_beam_search":
		return bad("hear.decoding must be greedy_search or modified_beam_search, not %q", c.Hear.Decoding)
	case c.Hear.Threads < 1 || c.Voice.Threads < 1:
		return bad("hear.threads and voice.threads must be at least 1")
	case c.Voice.Speed < 0.5 || c.Voice.Speed > 2:
		return bad("voice.speed must be between 0.5 and 2, not %v", c.Voice.Speed)
	case c.Audio.Gain <= 0 || c.Audio.Volume <= 0:
		return bad("audio.gain and audio.volume must be above 0")
	case !strings.Contains(c.Audio.Play, "{rate}"):
		return bad("audio.play must contain {rate}, which is replaced by the sample rate of the voice")
	case c.Listen.Level <= 0 || c.Listen.Level >= 1:
		return bad("listen.level must be between 0 and 1, not %v", c.Listen.Level)
	case c.Listen.WaitMs < 1000 || c.Listen.QuietMs < 200 || c.Listen.MaxSpeechMs < 1000 || c.Listen.Tries < 1:
		return bad("listen: wait_ms and max_speech_ms must be at least 1000, quiet_ms at least 200, tries at least 1")
	}
	pins := map[int]string{}
	for name, pin := range c.GPIO.Buttons() {
		if pin < 0 {
			continue
		}
		if other, dup := pins[pin]; dup {
			return bad("gpio: %s and %s both use GPIO %d", other, name, pin)
		}
		pins[pin] = name
	}
	if other, dup := pins[c.GPIO.Light]; dup && c.GPIO.Light >= 0 {
		return bad("gpio: light and %s both use GPIO %d", other, c.GPIO.Light)
	}
	return nil
}

// Buttons returns the button pins by name.
func (g GPIO) Buttons() map[string]int {
	return map[string]int{"yes": g.Yes, "no": g.No, "not_sure": g.NotSure, "play": g.Play}
}
