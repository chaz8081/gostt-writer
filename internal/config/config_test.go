package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.ModelPath == "" {
		t.Error("ModelPath should not be empty")
	}
	if cfg.Hotkey.Mode != "hold" {
		t.Errorf("Hotkey.Mode = %q, want %q", cfg.Hotkey.Mode, "hold")
	}
	if len(cfg.Hotkey.Keys) != 3 {
		t.Errorf("Hotkey.Keys length = %d, want 3", len(cfg.Hotkey.Keys))
	}
	if cfg.Audio.SampleRate != 16000 {
		t.Errorf("Audio.SampleRate = %d, want 16000", cfg.Audio.SampleRate)
	}
	if cfg.Audio.Channels != 1 {
		t.Errorf("Audio.Channels = %d, want 1", cfg.Audio.Channels)
	}
	if cfg.Inject.Method != "type" {
		t.Errorf("Inject.Method = %q, want %q", cfg.Inject.Method, "type")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad(t *testing.T) {
	yamlContent := `
model_path: /tmp/test-model.bin
hotkey:
  keys: ["alt", "d"]
  mode: toggle
audio:
  sample_rate: 44100
  channels: 2
inject:
  method: paste
log_level: debug
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ModelPath != "/tmp/test-model.bin" {
		t.Errorf("ModelPath = %q, want %q", cfg.ModelPath, "/tmp/test-model.bin")
	}
	if cfg.Hotkey.Mode != "toggle" {
		t.Errorf("Hotkey.Mode = %q, want %q", cfg.Hotkey.Mode, "toggle")
	}
	if len(cfg.Hotkey.Keys) != 2 || cfg.Hotkey.Keys[0] != "alt" || cfg.Hotkey.Keys[1] != "d" {
		t.Errorf("Hotkey.Keys = %v, want [alt d]", cfg.Hotkey.Keys)
	}
	if cfg.Audio.SampleRate != 44100 {
		t.Errorf("Audio.SampleRate = %d, want 44100", cfg.Audio.SampleRate)
	}
	if cfg.Audio.Channels != 2 {
		t.Errorf("Audio.Channels = %d, want 2", cfg.Audio.Channels)
	}
	if cfg.Inject.Method != "paste" {
		t.Errorf("Inject.Method = %q, want %q", cfg.Inject.Method, "paste")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	yamlContent := `
model_path: ~/models/test.bin
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	expected := filepath.Join(home, "models/test.bin")
	if cfg.ModelPath != expected {
		t.Errorf("ModelPath = %q, want %q", cfg.ModelPath, expected)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() should return error for nonexistent file")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "invalid hotkey mode",
			modify:  func(c *Config) { c.Hotkey.Mode = "invalid" },
			wantErr: true,
		},
		{
			name:    "invalid inject method",
			modify:  func(c *Config) { c.Inject.Method = "invalid" },
			wantErr: true,
		},
		{
			name:    "empty hotkey keys",
			modify:  func(c *Config) { c.Hotkey.Keys = nil },
			wantErr: true,
		},
		{
			name:    "zero sample rate",
			modify:  func(c *Config) { c.Audio.SampleRate = 0 },
			wantErr: true,
		},
		{
			name:    "zero channels",
			modify:  func(c *Config) { c.Audio.Channels = 0 },
			wantErr: true,
		},
		{
			name:    "invalid log level",
			modify:  func(c *Config) { c.LogLevel = "invalid" },
			wantErr: true,
		},
		{
			name:    "empty transcribe model path",
			modify:  func(c *Config) { c.Transcribe.ModelPath = "" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriteDefault_CreatesFile(t *testing.T) {
	// Use a temp dir as fake home to avoid touching real config
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, err := WriteDefault()
	if err != nil {
		t.Fatalf("WriteDefault() error = %v", err)
	}

	expectedDir := filepath.Join(tmpHome, ".config", "gostt-writer")
	expectedPath := filepath.Join(expectedDir, "config.yaml")

	if path != expectedPath {
		t.Errorf("WriteDefault() path = %q, want %q", path, expectedPath)
	}

	// Verify file exists and contains valid YAML
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	content := string(data)

	// Should have a header comment
	if !strings.HasPrefix(content, "# gostt-writer") {
		t.Error("written config should start with header comment")
	}

	// Should be valid YAML that parses into a Config
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("written config is not valid YAML: %v", err)
	}

	// Values should match defaults
	if cfg.Hotkey.Mode != "hold" {
		t.Errorf("written config Hotkey.Mode = %q, want %q", cfg.Hotkey.Mode, "hold")
	}
	if cfg.Audio.SampleRate != 16000 {
		t.Errorf("written config Audio.SampleRate = %d, want 16000", cfg.Audio.SampleRate)
	}
}

func TestWriteDefault_NoOpIfExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create config dir and file manually first
	configDir := filepath.Join(tmpHome, ".config", "gostt-writer")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingContent := []byte("model_path: /custom/model.bin\n")
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, existingContent, 0644); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	// WriteDefault should return ("", nil) without overwriting
	path, err := WriteDefault()
	if err != nil {
		t.Fatalf("WriteDefault() error = %v", err)
	}
	if path != "" {
		t.Errorf("WriteDefault() path = %q, want empty string for existing file", path)
	}

	// Verify the original content is untouched
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(data) != string(existingContent) {
		t.Error("WriteDefault() should not overwrite existing config file")
	}
}

func TestDefaultTranscribeConfig(t *testing.T) {
	cfg := Default()

	if cfg.Transcribe.Backend != "whisper" {
		t.Errorf("Transcribe.Backend = %q, want %q", cfg.Transcribe.Backend, "whisper")
	}
	if cfg.Transcribe.ModelPath != "models/ggml-base.en.bin" {
		t.Errorf("Transcribe.ModelPath = %q, want %q", cfg.Transcribe.ModelPath, "models/ggml-base.en.bin")
	}
	if cfg.Transcribe.ParakeetModelDir != "models/parakeet-tdt-v2" {
		t.Errorf("Transcribe.ParakeetModelDir = %q, want %q", cfg.Transcribe.ParakeetModelDir, "models/parakeet-tdt-v2")
	}
}

func TestLoadBackwardCompatModelPath(t *testing.T) {
	// Old-style config with top-level model_path should map to Transcribe.ModelPath
	yamlContent := `
model_path: /custom/whisper.bin
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Transcribe.ModelPath != "/custom/whisper.bin" {
		t.Errorf("Transcribe.ModelPath = %q, want %q", cfg.Transcribe.ModelPath, "/custom/whisper.bin")
	}
	if cfg.Transcribe.Backend != "whisper" {
		t.Errorf("Transcribe.Backend = %q, want %q", cfg.Transcribe.Backend, "whisper")
	}
}

func TestLoadNewStyleTranscribeConfig(t *testing.T) {
	yamlContent := `
transcribe:
  backend: parakeet
  parakeet_model_dir: /opt/models/parakeet
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Transcribe.Backend != "parakeet" {
		t.Errorf("Transcribe.Backend = %q, want %q", cfg.Transcribe.Backend, "parakeet")
	}
	if cfg.Transcribe.ParakeetModelDir != "/opt/models/parakeet" {
		t.Errorf("Transcribe.ParakeetModelDir = %q, want %q", cfg.Transcribe.ParakeetModelDir, "/opt/models/parakeet")
	}
}

func TestLoadTranscribeExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	yamlContent := `
transcribe:
  backend: parakeet
  model_path: ~/models/whisper.bin
  parakeet_model_dir: ~/models/parakeet
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	expectedModelPath := filepath.Join(home, "models/whisper.bin")
	if cfg.Transcribe.ModelPath != expectedModelPath {
		t.Errorf("Transcribe.ModelPath = %q, want %q", cfg.Transcribe.ModelPath, expectedModelPath)
	}
	expectedParakeetDir := filepath.Join(home, "models/parakeet")
	if cfg.Transcribe.ParakeetModelDir != expectedParakeetDir {
		t.Errorf("Transcribe.ParakeetModelDir = %q, want %q", cfg.Transcribe.ParakeetModelDir, expectedParakeetDir)
	}
}

func TestValidateWhisperBackendRequiresModelPath(t *testing.T) {
	cfg := Default()
	cfg.Transcribe.Backend = "whisper"
	cfg.Transcribe.ModelPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail when whisper backend has empty model_path")
	}
}

func TestValidateParakeetBackendRequiresModelDir(t *testing.T) {
	cfg := Default()
	cfg.Transcribe.Backend = "parakeet"
	cfg.Transcribe.ParakeetModelDir = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail when parakeet backend has empty parakeet_model_dir")
	}
}

func TestValidateUnknownBackendFails(t *testing.T) {
	cfg := Default()
	cfg.Transcribe.Backend = "invalid"
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail for unknown backend")
	}
}

func TestLoadBLEConfig(t *testing.T) {
	yamlContent := `
inject:
  method: ble
  ble:
    device_mac: "AA:BB:CC:DD:EE:FF"
    shared_secret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    queue_size: 32
    reconnect_max: 15
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Inject.Method != "ble" {
		t.Errorf("Inject.Method = %q, want %q", cfg.Inject.Method, "ble")
	}
	if cfg.Inject.BLE.DeviceMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Inject.BLE.DeviceMAC = %q, want %q", cfg.Inject.BLE.DeviceMAC, "AA:BB:CC:DD:EE:FF")
	}
	if cfg.Inject.BLE.SharedSecret != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Errorf("Inject.BLE.SharedSecret = %q", cfg.Inject.BLE.SharedSecret)
	}
	if cfg.Inject.BLE.QueueSize != 32 {
		t.Errorf("Inject.BLE.QueueSize = %d, want 32", cfg.Inject.BLE.QueueSize)
	}
	if cfg.Inject.BLE.ReconnectMax != 15 {
		t.Errorf("Inject.BLE.ReconnectMax = %d, want 15", cfg.Inject.BLE.ReconnectMax)
	}
}

func TestValidateBLEMethodRequiresPairing(t *testing.T) {
	cfg := Default()
	cfg.Inject.Method = "ble"
	// No BLE config set
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail when method=ble but no device_mac")
	}
}

func TestValidateBLEMethodWithPairing(t *testing.T) {
	cfg := Default()
	cfg.Inject.Method = "ble"
	cfg.Inject.BLE.DeviceMAC = "AA:BB:CC:DD:EE:FF"
	cfg.Inject.BLE.SharedSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestValidateBLEBadSharedSecretTooShort(t *testing.T) {
	cfg := Default()
	cfg.Inject.Method = "ble"
	cfg.Inject.BLE.DeviceMAC = "AA:BB:CC:DD:EE:FF"
	cfg.Inject.BLE.SharedSecret = "not-hex"
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail for too-short shared_secret")
	}
}

func TestValidateBLEBadSharedSecretInvalidHex(t *testing.T) {
	cfg := Default()
	cfg.Inject.Method = "ble"
	cfg.Inject.BLE.DeviceMAC = "AA:BB:CC:DD:EE:FF"
	cfg.Inject.BLE.SharedSecret = strings.Repeat("zz", 32) // 64 chars, but 'z' is invalid hex
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail for invalid hex shared_secret")
	}
}

func TestBLEConfigDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Inject.BLE.QueueSize != 0 {
		t.Errorf("default BLE.QueueSize = %d, want 0 (will use runtime default)", cfg.Inject.BLE.QueueSize)
	}
}

func TestLoadConfigWithoutBLESection(t *testing.T) {
	// Backward compat: configs without inject.ble should still load fine
	yamlContent := `
inject:
  method: type
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Inject.Method != "type" {
		t.Errorf("Inject.Method = %q, want %q", cfg.Inject.Method, "type")
	}
	if cfg.Inject.BLE.DeviceMAC != "" {
		t.Errorf("Inject.BLE.DeviceMAC = %q, want empty", cfg.Inject.BLE.DeviceMAC)
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // defaults to info
		{"", slog.LevelInfo},        // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
