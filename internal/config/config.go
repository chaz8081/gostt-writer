package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	ModelPath  string           `yaml:"model_path,omitempty"` // deprecated: use Transcribe.ModelPath
	Transcribe TranscribeConfig `yaml:"transcribe"`
	Hotkey     HotkeyConfig     `yaml:"hotkey"`
	Audio      AudioConfig      `yaml:"audio"`
	Inject     InjectConfig     `yaml:"inject"`
	LogLevel   string           `yaml:"log_level"`
}

// TranscribeConfig holds transcription backend settings.
type TranscribeConfig struct {
	Backend          string `yaml:"backend"`            // "whisper" or "parakeet"
	ModelPath        string `yaml:"model_path"`         // whisper: path to ggml model file
	ParakeetModelDir string `yaml:"parakeet_model_dir"` // parakeet: dir with .mlmodelc files + vocab
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
	Method string    `yaml:"method"` // "type", "paste", or "ble"
	BLE    BLEConfig `yaml:"ble,omitempty"`
}

// BLEConfig holds BLE output settings (used when inject.method is "ble").
type BLEConfig struct {
	DeviceMAC    string `yaml:"device_mac,omitempty"`    // paired ESP32 MAC address
	SharedSecret string `yaml:"shared_secret,omitempty"` // hex-encoded 32-byte AES key
	QueueSize    int    `yaml:"queue_size,omitempty"`    // max queued messages during disconnect (default 64)
	ReconnectMax int    `yaml:"reconnect_max,omitempty"` // max reconnect backoff in seconds (default 30)
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

// DefaultDataDir returns the default data directory path for application data.
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "gostt-writer")
}

// DefaultModelsDir returns the default directory for model files.
func DefaultModelsDir() string {
	return filepath.Join(DefaultDataDir(), "models")
}

// Default returns a Config with sensible default values.
func Default() *Config {
	modelsDir := DefaultModelsDir()
	return &Config{
		ModelPath: filepath.Join(modelsDir, "ggml-base.en.bin"),
		Transcribe: TranscribeConfig{
			Backend:          "whisper",
			ModelPath:        filepath.Join(modelsDir, "ggml-base.en.bin"),
			ParakeetModelDir: filepath.Join(modelsDir, "parakeet-tdt-v2"),
		},
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
// with defaults. Tilde (~) in paths is expanded to the user's home directory.
// For backward compatibility, a top-level model_path is copied to
// Transcribe.ModelPath if the latter is not explicitly set.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Backward compat: old top-level model_path → Transcribe.ModelPath
	if cfg.ModelPath != "" && cfg.Transcribe.ModelPath == Default().Transcribe.ModelPath {
		cfg.Transcribe.ModelPath = cfg.ModelPath
	}

	// Default backend if not set
	if cfg.Transcribe.Backend == "" {
		cfg.Transcribe.Backend = "whisper"
	}

	// Expand tildes
	cfg.ModelPath = expandTilde(cfg.ModelPath)
	cfg.Transcribe.ModelPath = expandTilde(cfg.Transcribe.ModelPath)
	cfg.Transcribe.ParakeetModelDir = expandTilde(cfg.Transcribe.ParakeetModelDir)

	// Fallback: if configured model path doesn't exist, check relative path in working dir
	cfg.Transcribe.ModelPath = resolveModelPath(cfg.Transcribe.ModelPath, "models/ggml-base.en.bin")
	cfg.Transcribe.ParakeetModelDir = resolveModelPath(cfg.Transcribe.ParakeetModelDir, "models/parakeet-tdt-v2")

	return cfg, nil
}

// resolveModelPath returns the configured path if it exists, or falls back to
// a relative path in the working directory for development convenience.
func resolveModelPath(configured, relativeFallback string) string {
	if _, err := os.Stat(configured); err == nil {
		return configured
	}
	if _, err := os.Stat(relativeFallback); err == nil {
		return relativeFallback
	}
	return configured // return original (will fail later with clear error)
}

// Validate checks the config for invalid values.
func (c *Config) Validate() error {
	// Validate transcribe backend
	switch c.Transcribe.Backend {
	case "whisper", "":
		if c.Transcribe.ModelPath == "" {
			return fmt.Errorf("transcribe.model_path must not be empty for whisper backend")
		}
	case "parakeet":
		if c.Transcribe.ParakeetModelDir == "" {
			return fmt.Errorf("transcribe.parakeet_model_dir must not be empty for parakeet backend")
		}
	default:
		return fmt.Errorf("transcribe.backend must be \"whisper\" or \"parakeet\", got %q", c.Transcribe.Backend)
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
	case "ble":
		if c.Inject.BLE.DeviceMAC == "" {
			return fmt.Errorf("inject.ble.device_mac required when inject.method is \"ble\" (run: task ble-pair)")
		}
		if c.Inject.BLE.SharedSecret == "" {
			return fmt.Errorf("inject.ble.shared_secret required when inject.method is \"ble\" (run: task ble-pair)")
		}
		if len(c.Inject.BLE.SharedSecret) != 64 {
			return fmt.Errorf("inject.ble.shared_secret must be 64 hex characters (32 bytes), got %d", len(c.Inject.BLE.SharedSecret))
		}
		if _, err := hex.DecodeString(c.Inject.BLE.SharedSecret); err != nil {
			return fmt.Errorf("inject.ble.shared_secret must be valid hex: %w", err)
		}
	default:
		return fmt.Errorf("inject.method must be \"type\", \"paste\", or \"ble\", got %q", c.Inject.Method)
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

// WriteDefault creates the default config file with documented defaults.
// It creates the parent directory if needed. Returns the path written to.
// If the file already exists, it returns ("", nil) without overwriting.
func WriteDefault() (string, error) {
	path := DefaultConfigPath()
	if _, err := os.Stat(path); err == nil {
		return "", nil // already exists
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating config dir %s: %w", dir, err)
	}

	cfg := Default()
	cfg.ModelPath = "" // omit deprecated field from generated config
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshaling default config: %w", err)
	}

	header := "# gostt-writer configuration\n# See config.example.yaml for documentation\n\n"
	if err := os.WriteFile(path, []byte(header+string(data)), 0644); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return path, nil
}

// ParseLogLevel converts a log level string to a slog.Level.
func ParseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default: // "info"
		return slog.LevelInfo
	}
}
