package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPIO.Talk != 17 || cfg.HID.Device != "/dev/hidg0" || !cfg.Typing.Capitalize {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadOverridesOnlyWhatIsGiven(t *testing.T) {
	t.Setenv("HOME", "/home/pi")
	cfg, err := Load(write(t, "model:\n  dir: ~/models/kroko\n  threads: 4\ngpio:\n  cancel: -1\ntyping:\n  capitalize: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model.Dir != "/home/pi/models/kroko" {
		t.Errorf("~ not expanded: %q", cfg.Model.Dir)
	}
	if cfg.Model.Threads != 4 || cfg.GPIO.Cancel != -1 || cfg.Typing.Capitalize {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.Model.Decoding != "modified_beam_search" || cfg.GPIO.Talk != 17 || cfg.Typing.SettleMs != 1500 {
		t.Errorf("defaults lost: %+v", cfg)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	if _, err := Load(write(t, "")); err != nil {
		t.Fatalf("an empty file should give the defaults: %v", err)
	}
	if _, err := Load(write(t, "# only a comment\n")); err != nil {
		t.Fatalf("a comment-only file should give the defaults: %v", err)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := map[string]struct{ body, want string }{
		"unknown key":      {"gpio:\n  tlak: 5\n", "tlak"},
		"bad yaml":         {"gpio: [", "config"},
		"duplicate pin":    {"gpio:\n  undo: 17\n", "both use GPIO 17"},
		"pin out of range": {"gpio:\n  talk: 40\n", "GPIO 0 to 27"},
		"bad decoding":     {"model:\n  decoding: fast\n", "greedy_search"},
		"zero threads":     {"model:\n  threads: 0\n", "threads"},
		"empty command":    {"audio:\n  command: \"  \"\n", "audio.command"},
		"zero gain":        {"audio:\n  gain: 0\n", "gain"},
		"negative time":    {"talk:\n  hold_ms: -1\n", "negative"},
		"gap below settle": {"typing:\n  settle_ms: 3000\n", "phrase_gap_ms"},
		"empty model dir":  {"model:\n  dir: \"\"\n", "model.dir"},
		"empty hid":        {"hid:\n  device: \"\"\n", "hid.device"},
		"negative padding": {"model:\n  tail_padding_ms: -5\n", "tail_padding_ms"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("want an error mentioning %q, got %v", tt.want, err)
			}
		})
	}
}

func TestLoadUnreadable(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil { // a directory, not a file
		t.Error("want an error reading a directory")
	}
}

func TestPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/cfg")
	t.Setenv("XDG_DATA_HOME", "/x/data")
	if got := DefaultPath(); got != "/x/cfg/gostt-pad/config.yaml" {
		t.Errorf("DefaultPath = %q", got)
	}
	if got := Default().Model.Dir; got != "/x/data/gostt-pad/models/"+ModelName {
		t.Errorf("model dir = %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/pi")
	if got := DefaultPath(); got != "/home/pi/.config/gostt-pad/config.yaml" {
		t.Errorf("DefaultPath = %q", got)
	}
}

func TestDefaultPathPrefersUserThenSystem(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	old := systemPath
	defer func() { systemPath = old }()
	systemPath = filepath.Join(dir, "etc.yaml")
	user := filepath.Join(dir, "gostt-pad", "config.yaml")

	if got := DefaultPath(); got != user {
		t.Errorf("neither file exists: got %q, want the user path", got)
	}
	_ = os.WriteFile(systemPath, nil, 0o600)
	if got := DefaultPath(); got != systemPath {
		t.Errorf("only the system file exists: got %q", got)
	}
	_ = os.MkdirAll(filepath.Dir(user), 0o700)
	_ = os.WriteFile(user, nil, 0o600)
	if got := DefaultPath(); got != user {
		t.Errorf("both exist: got %q, want the user file", got)
	}
}

// The example file is what the installer copies, so it must load and must
// say the same as the built-in defaults.
func TestExampleFileMatchesDefaults(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/opt/gostt-pad")
	cfg, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.Model.Dir = "/opt/gostt-pad/models/" + ModelName
	if cfg != want {
		t.Errorf("config.example.yaml differs from the defaults:\n got %+v\nwant %+v", cfg, want)
	}
}
