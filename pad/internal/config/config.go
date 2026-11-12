// Package config loads the pad's settings from a YAML file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelName is the directory name of the default speech model.
const ModelName = "sherpa-onnx-streaming-zipformer-en-kroko-2025-08-06"

// Config is everything the pad can be told.
type Config struct {
	Model  Model  `yaml:"model"`
	Audio  Audio  `yaml:"audio"`
	HID    HID    `yaml:"hid"`
	GPIO   GPIO   `yaml:"gpio"`
	Talk   Talk   `yaml:"talk"`
	Typing Typing `yaml:"typing"`
}

// Model selects the speech model.
type Model struct {
	Dir          string `yaml:"dir"`             // folder with encoder, decoder, joiner and tokens
	Threads      int    `yaml:"threads"`         // CPU threads for the model
	Decoding     string `yaml:"decoding"`        // greedy_search or modified_beam_search
	TailPaddingM int    `yaml:"tail_padding_ms"` // silence fed after the talk key stops
}

// Audio selects the microphone.
type Audio struct {
	// Command must write raw 16-bit little-endian mono 16 kHz audio to its
	// standard output.
	Command string  `yaml:"command"`
	Gain    float32 `yaml:"gain"`
}

// HID selects the USB keyboard output.
type HID struct {
	Device        string `yaml:"device"`
	ReportDelayMs int    `yaml:"report_delay_ms"` // pause after each report
}

// GPIO maps buttons to BCM GPIO numbers. A button connects its pin to
// ground. Minus one leaves a button or the LED unused.
type GPIO struct {
	Chip       string `yaml:"chip"` // empty: found automatically
	Talk       int    `yaml:"talk"`
	Undo       int    `yaml:"undo"`
	DeleteWord int    `yaml:"delete_word"`
	NewLine    int    `yaml:"new_line"`
	Cancel     int    `yaml:"cancel"`
	LED        int    `yaml:"led"` // lit while listening
	DebounceMs int    `yaml:"debounce_ms"`
}

// Talk tunes the talk key.
type Talk struct {
	HoldMs int `yaml:"hold_ms"` // a press at least this long is a hold
}

// Typing tunes how text reaches the host.
type Typing struct {
	Capitalize  bool `yaml:"capitalize"`
	SettleMs    int  `yaml:"settle_ms"`     // the last word is typed after this much quiet
	PhraseGapMs int  `yaml:"phrase_gap_ms"` // a pause this long starts a new undo step
}

// Default returns the settings used when the file does not say otherwise.
func Default() Config {
	return Config{
		Model: Model{
			Dir:          filepath.Join(dataDir(), "models", ModelName),
			Threads:      2,
			Decoding:     "modified_beam_search",
			TailPaddingM: 2500,
		},
		Audio: Audio{
			Command: "arecord -q -D default -f S16_LE -r 16000 -c 1 -t raw",
			Gain:    1,
		},
		HID:    HID{Device: "/dev/hidg0", ReportDelayMs: 2},
		GPIO:   GPIO{Talk: 17, Undo: 27, DeleteWord: 22, NewLine: 23, Cancel: 24, LED: 25, DebounceMs: 10},
		Talk:   Talk{HoldMs: 350},
		Typing: Typing{Capitalize: true, SettleMs: 1000, PhraseGapMs: 2000},
	}
}

// DefaultPath is where the settings file is looked for.
func DefaultPath() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "gostt-pad", "config.yaml")
	}
	return filepath.Join(home(), ".config", "gostt-pad", "config.yaml")
}

// Load reads the settings file at path over the defaults. A missing file is
// not an error: the defaults are returned.
func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return cfg, fmt.Errorf("config: %w", err)
	default:
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil && err.Error() != "EOF" {
			return cfg, fmt.Errorf("config: %s: %w", path, err)
		}
	}
	cfg.Model.Dir = expand(cfg.Model.Dir)
	return cfg, cfg.Validate()
}

// Validate reports the first setting that cannot work.
func (c Config) Validate() error {
	switch {
	case c.Model.Dir == "":
		return errors.New("config: model.dir is empty")
	case c.Model.Threads < 1:
		return errors.New("config: model.threads must be at least 1")
	case c.Model.Decoding != "greedy_search" && c.Model.Decoding != "modified_beam_search":
		return fmt.Errorf("config: model.decoding %q must be greedy_search or modified_beam_search", c.Model.Decoding)
	case c.Model.TailPaddingM < 0:
		return errors.New("config: model.tail_padding_ms must not be negative")
	case strings.TrimSpace(c.Audio.Command) == "":
		return errors.New("config: audio.command is empty")
	case c.Audio.Gain <= 0:
		return errors.New("config: audio.gain must be above 0")
	case c.HID.Device == "":
		return errors.New("config: hid.device is empty")
	case c.HID.ReportDelayMs < 0 || c.Talk.HoldMs < 0 || c.Typing.SettleMs < 0 || c.Typing.PhraseGapMs < 0 || c.GPIO.DebounceMs < 0:
		return errors.New("config: times must not be negative")
	case c.Typing.PhraseGapMs < c.Typing.SettleMs:
		return errors.New("config: typing.phrase_gap_ms must not be shorter than typing.settle_ms")
	}
	seen := map[int]string{}
	for _, p := range c.GPIO.Pins() {
		if p.Pin < 0 {
			continue
		}
		if p.Pin > 27 {
			return fmt.Errorf("config: gpio.%s is %d; the header has GPIO 0 to 27", p.Name, p.Pin)
		}
		if other, dup := seen[p.Pin]; dup {
			return fmt.Errorf("config: gpio.%s and gpio.%s both use GPIO %d", other, p.Name, p.Pin)
		}
		seen[p.Pin] = p.Name
	}
	return nil
}

// Pin is one named GPIO assignment.
type Pin struct {
	Name string
	Pin  int
}

// Pins lists the GPIO assignments, buttons first and the LED last.
func (g GPIO) Pins() []Pin {
	return []Pin{
		{"talk", g.Talk}, {"undo", g.Undo}, {"delete_word", g.DeleteWord}, {"new_line", g.NewLine}, {"cancel", g.Cancel}, {"led", g.LED},
	}
}

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func dataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "gostt-pad")
	}
	return filepath.Join(home(), ".local", "share", "gostt-pad")
}

func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(home(), p[1:])
	}
	return p
}
