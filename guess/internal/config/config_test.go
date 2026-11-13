package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleFileMatchesDefaults(t *testing.T) {
	got, err := Load(filepath.Join("..", "..", "config.example.yaml"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Errorf("config.example.yaml differs from Default():\n%+v\n%+v", got, Default())
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "c.yaml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if cfg, err := Load(filepath.Join(dir, "none.yaml"), false); err != nil || cfg != Default() {
		t.Errorf("missing optional file: %v", err)
	}
	if _, err := Load(filepath.Join(dir, "none.yaml"), true); err == nil {
		t.Error("missing required file: no error")
	}
	if cfg, err := Load(write(""), true); err != nil || cfg != Default() {
		t.Errorf("empty file: %v", err)
	}
	cfg, err := Load(write("audio:\n  volume: 2.5\ngpio:\n  play: -1\n"), true)
	if err != nil || cfg.Audio.Volume != 2.5 || cfg.GPIO.Play != -1 || cfg.GPIO.Yes != 17 {
		t.Errorf("partial file: %+v %v", cfg, err)
	}
	for body, want := range map[string]string{
		"audio:\n  volum: 2\n":       "volum",
		"gpio:\n  yes: 27\n":         "both use GPIO 27",
		"gpio:\n  light: 17\n":       "both use GPIO 17",
		"audio:\n  play: aplay\n":    "{rate}",
		"game:\n  confidence: 1.5\n": "confidence",
		"hear:\n  decoding: fast\n":  "decoding",
		"listen:\n  level: 0\n":      "level",
	} {
		if _, err := Load(write(body), true); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want it to mention %q", body, err, want)
		}
	}
}
