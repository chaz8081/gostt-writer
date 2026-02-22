package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	ModelPath string       `yaml:"model_path"`
	Hotkey    HotkeyConfig `yaml:"hotkey"`
	Audio     AudioConfig  `yaml:"audio"`
	Inject    InjectConfig `yaml:"inject"`
	LogLevel  string       `yaml:"log_level"`
}

// HotkeyConfig holds hotkey-related settings.
type HotkeyConfig struct {
	Keys []string `yaml:"keys"`
	Mode string   `yaml:"mode"` // "hold" or "toggle"
}

// AudioConfig holds audio capture settings.
type AudioConfig struct {
	SampleRate uint32 `yaml:"sample_rate"`
	Channels   uint32 `yaml:"channels"`
}

// InjectConfig holds text injection settings.
type InjectConfig struct {
	Method string `yaml:"method"` // "type" or "paste"
}

// DefaultConfigDir returns the default config directory path.
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gostt-writer")
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "config.yaml")
}

// Default returns a Config with sensible default values.
func Default() *Config {
	home, _ := os.UserHomeDir()
	modelPath := filepath.Join(home, ".local", "share", "gostt-writer", "models", "ggml-base.en.bin")

	return &Config{
		ModelPath: modelPath,
		Hotkey: HotkeyConfig{
			Keys: []string{"ctrl", "shift", "r"},
			Mode: "hold",
		},
		Audio: AudioConfig{
			SampleRate: 16000,
			Channels:   1,
		},
		Inject: InjectConfig{
			Method: "type",
		},
		LogLevel: "info",
	}
}

// Load reads and parses a YAML config file. Missing fields are filled
// with defaults. Tilde (~) in model_path is expanded to the user's home directory.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.ModelPath = expandTilde(cfg.ModelPath)

	return cfg, nil
}

// Validate checks the config for invalid values.
func (c *Config) Validate() error {
	if c.ModelPath == "" {
		return fmt.Errorf("model_path must not be empty")
	}

	if len(c.Hotkey.Keys) == 0 {
		return fmt.Errorf("hotkey.keys must not be empty")
	}

	switch c.Hotkey.Mode {
	case "hold", "toggle":
	default:
		return fmt.Errorf("hotkey.mode must be \"hold\" or \"toggle\", got %q", c.Hotkey.Mode)
	}

	if c.Audio.SampleRate == 0 {
		return fmt.Errorf("audio.sample_rate must be > 0")
	}

	if c.Audio.Channels == 0 {
		return fmt.Errorf("audio.channels must be > 0")
	}

	switch c.Inject.Method {
	case "type", "paste":
	default:
		return fmt.Errorf("inject.method must be \"type\" or \"paste\", got %q", c.Inject.Method)
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level must be debug, info, warn, or error, got %q", c.LogLevel)
	}

	return nil
}

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
