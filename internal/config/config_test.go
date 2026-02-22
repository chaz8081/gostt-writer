package config

import (
	"os"
	"path/filepath"
	"testing"
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
			name:    "empty model path",
			modify:  func(c *Config) { c.ModelPath = "" },
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
