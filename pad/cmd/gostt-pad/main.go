//go:build linux

// Command gostt-pad turns a Raspberry Pi into a standalone dictation macro
// pad: it listens to a microphone, transcribes speech on the device, and
// types the words into the connected computer as a USB keyboard.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/chaz8081/gostt-writer/pad/internal/asr"
	"github.com/chaz8081/gostt-writer/pad/internal/config"
)

const usage = `gostt-pad: on-device dictation that types as a USB keyboard

Usage:
  gostt-pad [run] [flags]        listen and type
  gostt-pad check [flags]        test the model, microphone, USB keyboard and buttons
  gostt-pad bench [flags] a.wav  transcribe WAV files and report the speed
  gostt-pad version

Flags:
`

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

type flags struct {
	configPath string
	dryRun     bool
	keys       string
	wav        string
	threads    int
	debug      bool
	buttons    time.Duration
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := "run"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var f flags
	fs := flag.NewFlagSet("gostt-pad", flag.ContinueOnError)
	fs.StringVar(&f.configPath, "config", config.DefaultPath(), "settings file")
	fs.BoolVar(&f.dryRun, "dry-run", false, "print what would be typed instead of sending it over USB")
	fs.StringVar(&f.keys, "keys", "gpio", "where button presses come from: gpio, stdin, both or none")
	fs.StringVar(&f.wav, "wav", "", "use this WAV file as the microphone (16 kHz mono 16-bit), then silence")
	fs.IntVar(&f.threads, "threads", 0, "CPU threads for the model (overrides the settings file)")
	fs.BoolVar(&f.debug, "debug", false, "log every decision")
	fs.DurationVar(&f.buttons, "buttons", 0, "check only: wait this long for button presses and report them (for example 20s)")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	// Accept flags before the command as well: gostt-pad -config x bench a.wav
	if rest := fs.Args(); cmd == "run" && len(rest) > 0 {
		switch rest[0] {
		case "run", "check", "bench", "version", "help":
			cmd = rest[0]
			if err := fs.Parse(rest[1:]); err != nil {
				return 2
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q\n\n", rest[0])
			fs.Usage()
			return 2
		}
	}

	level := slog.LevelInfo
	if f.debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if cmd == "version" {
		fmt.Println("gostt-pad", version)
		return 0
	}
	if cmd == "help" {
		fs.Usage()
		return 0
	}
	cfg, err := config.Load(f.configPath)
	if err != nil {
		log.Error("settings are not usable", "err", err)
		return 2
	}
	if f.threads > 0 {
		cfg.Model.Threads = f.threads
	}
	switch f.keys {
	case "gpio", "stdin", "both", "none":
	default:
		log.Error("-keys must be gpio, stdin, both or none", "got", f.keys)
		return 2
	}

	switch cmd {
	case "run":
		return runPad(cfg, f, log)
	case "check":
		return runCheck(cfg, f)
	case "bench":
		return runBench(cfg, fs.Args())
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		fs.Usage()
		return 2
	}
}

func loadModel(cfg config.Config) (*asr.Recognizer, time.Duration, error) {
	start := time.Now()
	rec, err := asr.New(asr.Config{
		Dir:         cfg.Model.Dir,
		Threads:     cfg.Model.Threads,
		Decoding:    cfg.Model.Decoding,
		TailPadding: time.Duration(cfg.Model.TailPaddingM) * time.Millisecond,
	})
	return rec, time.Since(start), err
}
