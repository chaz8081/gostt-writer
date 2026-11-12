//go:build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chaz8081/gostt-writer/pad/internal/audio"
	"github.com/chaz8081/gostt-writer/pad/internal/config"
	"github.com/chaz8081/gostt-writer/pad/internal/core"
	"github.com/chaz8081/gostt-writer/pad/internal/gpio"
)

func runPad(cfg config.Config, f flags, log *slog.Logger) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rec, took, err := loadModel(cfg)
	if err != nil {
		log.Error("the speech model did not load", "err", err)
		return 1
	}
	defer rec.Close()
	log.Info("model loaded", "dir", cfg.Model.Dir, "took", took.Round(time.Millisecond), "threads", cfg.Model.Threads)

	// Where the keys go: the USB keyboard, or a simulated text field.
	var out io.Writer
	var sim *core.HostSim
	if f.dryRun {
		sim = &core.HostSim{}
		out = sim
	} else {
		dev := &core.DeviceWriter{Path: cfg.HID.Device}
		defer func() { _ = dev.Close() }()
		out = dev
	}
	queue := core.NewReportQueue(out, time.Duration(cfg.HID.ReportDelayMs)*time.Millisecond, log)
	if sim != nil {
		queue.OnBatch = func() { fmt.Printf("screen: %q\n", sim.Text()) }
	}
	defer queue.Close()

	// Buttons.
	events := make(chan core.KeyEvent, 64)
	var pad *gpio.Pad
	if f.keys == "gpio" || f.keys == "both" {
		pad, err = openButtons(cfg, events)
		if err != nil {
			log.Error("the buttons could not be set up; use -keys stdin to run without them", "err", err)
			return 1
		}
		defer pad.Close()
	}
	quit := make(chan struct{})
	if f.keys == "stdin" || f.keys == "both" {
		go readKeys(os.Stdin, events, quit)
		fmt.Fprintln(os.Stderr, stdinHelp)
	}

	ctrl := core.NewController(rec, queue.Send, func(listening bool) {
		pad.SetLED(listening)
		if f.dryRun {
			fmt.Printf("listening: %v\n", listening)
		}
	}, log, core.Options{
		HoldThreshold: time.Duration(cfg.Talk.HoldMs) * time.Millisecond,
		Settle:        time.Duration(cfg.Typing.SettleMs) * time.Millisecond,
		PhraseGap:     time.Duration(cfg.Typing.PhraseGapMs) * time.Millisecond,
		Capitalize:    cfg.Typing.Capitalize,
	})

	// Microphone.
	blocks := make(chan []float32, 256)
	micErr := make(chan error, 1)
	if f.wav != "" {
		samples, err := audio.ReadWAV(f.wav)
		if err != nil {
			log.Error("the WAV file could not be used", "err", err)
			return 1
		}
		go func() {
			audio.Play(ctx, samples, true, blocks)
			silence := make([]float32, audio.SampleRate*3600)
			audio.Play(ctx, silence, true, blocks)
		}()
	} else {
		go func() { micErr <- audio.RunCommand(ctx, cfg.Audio.Command, cfg.Audio.Gain, blocks) }()
	}

	log.Info("ready", "keys", f.keys, "dry_run", f.dryRun)
	for {
		select {
		case ev := <-events:
			ctrl.HandleKey(ev)
		case b := <-blocks:
			ctrl.HandleAudio(b)
		case err := <-micErr:
			if err != nil {
				log.Error("the microphone stopped", "err", err)
				return 1
			}
			return 0
		case <-quit:
			return 0
		case <-ctx.Done():
			log.Info("stopping")
			return 0
		}
	}
}

func openButtons(cfg config.Config, events chan<- core.KeyEvent) (*gpio.Pad, error) {
	chip := cfg.GPIO.Chip
	if chip == "" {
		var err error
		if chip, err = gpio.FindChip(); err != nil {
			return nil, err
		}
	}
	return gpio.Open(chip, buttonPins(cfg), cfg.GPIO.LED, time.Duration(cfg.GPIO.DebounceMs)*time.Millisecond, events)
}

func buttonPins(cfg config.Config) map[core.Key]int {
	all := map[core.Key]int{
		core.KeyTalk:       cfg.GPIO.Talk,
		core.KeyUndo:       cfg.GPIO.Undo,
		core.KeyDeleteWord: cfg.GPIO.DeleteWord,
		core.KeyNewLine:    cfg.GPIO.NewLine,
		core.KeyCancel:     cfg.GPIO.Cancel,
	}
	for k, pin := range all {
		if pin < 0 {
			delete(all, k)
		}
	}
	return all
}

const stdinHelp = `keys from the terminal (type a letter, then Enter):
  t  tap the talk key (toggle dictation)      u  undo the last phrase
  h  hold the talk key down                   w  delete the last word
  r  release the talk key                     n  new line
  c  cancel                                   q  quit
  wait 2.5   pause for that many seconds (for scripts)`

// readKeys turns lines typed in a terminal into button events, so the pad
// can be tried over SSH before any switch is wired.
func readKeys(r io.Reader, events chan<- core.KeyEvent, quit chan<- struct{}) {
	send := func(k core.Key, down bool) { events <- core.KeyEvent{Key: k, Down: down, At: time.Now()} }
	tap := func(k core.Key) {
		send(k, true)
		time.Sleep(60 * time.Millisecond)
		send(k, false)
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(strings.ToLower(sc.Text()))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "t", "tap", "talk":
			tap(core.KeyTalk)
		case "h", "hold":
			send(core.KeyTalk, true)
		case "r", "release":
			send(core.KeyTalk, false)
		case "u", "undo":
			tap(core.KeyUndo)
		case "w", "word":
			tap(core.KeyDeleteWord)
		case "n", "newline":
			tap(core.KeyNewLine)
		case "c", "cancel":
			tap(core.KeyCancel)
		case "wait":
			if len(fields) == 2 {
				if s, err := strconv.ParseFloat(fields[1], 64); err == nil && s > 0 {
					time.Sleep(time.Duration(s * float64(time.Second)))
					continue
				}
			}
			fmt.Fprintln(os.Stderr, "usage: wait SECONDS")
		case "q", "quit", "exit":
			close(quit)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown key %q\n%s\n", fields[0], stdinHelp)
		}
	}
	// End of input (for example a finished script): leave the pad running.
}
