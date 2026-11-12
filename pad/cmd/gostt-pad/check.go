//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/chaz8081/gostt-writer/pad/internal/audio"
	"github.com/chaz8081/gostt-writer/pad/internal/config"
	"github.com/chaz8081/gostt-writer/pad/internal/core"
	"github.com/chaz8081/gostt-writer/pad/internal/gpio"
)

// runCheck tests each part of the pad on its own and says plainly what
// works, so a wiring or setup problem is found before dictating.
func runCheck(cfg config.Config, f flags) int {
	failures := 0
	ok := func(part, msg string, args ...any) { fmt.Printf("ok    %-10s %s\n", part, fmt.Sprintf(msg, args...)) }
	warn := func(part, msg string, args ...any) { fmt.Printf("WARN  %-10s %s\n", part, fmt.Sprintf(msg, args...)) }
	fail := func(part, msg string, args ...any) {
		failures++
		fmt.Printf("FAIL  %-10s %s\n", part, fmt.Sprintf(msg, args...))
	}

	if _, err := os.Stat(f.configPath); err == nil {
		ok("settings", "read from %s", f.configPath)
	} else {
		ok("settings", "no file at %s; using the defaults", f.configPath)
	}

	// Model.
	if rec, took, err := loadModel(cfg); err != nil {
		fail("model", "%v", err)
	} else {
		ok("model", "loaded from %s in %v", cfg.Model.Dir, took.Round(time.Millisecond))
		rec.Close()
	}

	// Microphone: record three seconds and look at the level.
	if f.wav == "" {
		checkMic(cfg, ok, warn, fail)
	} else if _, err := audio.ReadWAV(f.wav); err != nil {
		fail("microphone", "%v", err)
	} else {
		ok("microphone", "using %s in place of a microphone", f.wav)
	}

	// USB keyboard.
	switch {
	case f.dryRun:
		ok("keyboard", "skipped (-dry-run)")
	default:
		dev := &core.DeviceWriter{Path: cfg.HID.Device, Timeout: 2 * time.Second}
		if _, err := dev.Write(make([]byte, 8)); err != nil { // an all-keys-up report types nothing
			fail("keyboard", "%v\n                 Is the USB gadget set up (deploy/install.sh), the Pi rebooted since, and the Pi's\n                 USB-C or data port connected to the computer? Use -dry-run to test without it.", err)
		} else {
			ok("keyboard", "%s accepted a report, so a computer appears to be connected", cfg.HID.Device)
		}
		_ = dev.Close()
	}

	// Buttons.
	switch f.keys {
	case "stdin", "none":
		ok("buttons", "skipped (-keys %s)", f.keys)
	default:
		checkButtons(cfg, f.buttons, ok, fail)
	}

	if failures > 0 {
		fmt.Printf("\n%d check(s) failed.\n", failures)
		return 1
	}
	fmt.Println("\nAll checks passed.")
	return 0
}

type report func(part, msg string, args ...any)

func checkMic(cfg config.Config, ok, warn, fail report) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	blocks := make(chan []float32, 64)
	done := make(chan error, 1)
	go func() { done <- audio.RunCommand(ctx, cfg.Audio.Command, cfg.Audio.Gain, blocks) }()
	fmt.Println("      microphone recording for 3 seconds: say something...")
	var samples []float32
	var err error
	for len(samples) < 3*audio.SampleRate && err == nil {
		select {
		case b := <-blocks:
			samples = append(samples, b...)
		case err = <-done:
			if err == nil {
				err = fmt.Errorf("the capture command produced only %.1f s of audio in 4 s", float64(len(samples))/audio.SampleRate)
			}
		}
	}
	cancel()
	if err != nil {
		fail("microphone", "%v\n                 command: %s\n                 List capture devices with: arecord -l", err, cfg.Audio.Command)
		return
	}
	rms, peak := audio.Level(samples)
	switch {
	case peak <= -119:
		fail("microphone", "pure digital silence. The device opens but delivers nothing: check the wiring or the chosen device.")
	case peak > -0.5:
		warn("microphone", "level %.0f dB, peak %.0f dB: clipping. Lower audio.gain or move back from the microphone.", rms, peak)
	case peak < -40:
		warn("microphone", "level %.0f dB, peak %.0f dB: very quiet. If you were speaking, raise audio.gain (try %.0f).", rms, peak, min(64, float64(cfg.Audio.Gain)*8))
	default:
		ok("microphone", "level %.0f dB, peak %.0f dB (speech normally peaks between -30 and -3)", rms, peak)
	}
}

func checkButtons(cfg config.Config, wait time.Duration, ok, fail report) {
	chip := cfg.GPIO.Chip
	if chip == "" {
		var err error
		if chip, err = gpio.FindChip(); err != nil {
			fail("buttons", "%v", err)
			return
		}
	}
	events := make(chan core.KeyEvent, 64)
	pins := buttonPins(cfg)
	pad, err := gpio.Open(chip, pins, cfg.GPIO.LED, time.Duration(cfg.GPIO.DebounceMs)*time.Millisecond, events)
	if err != nil {
		fail("buttons", "%v", err)
		return
	}
	defer pad.Close()
	var names []string
	for k, pin := range pins {
		names = append(names, fmt.Sprintf("%s=GPIO%d", k, pin))
	}
	sort.Strings(names)
	ok("buttons", "%s: %v", chip, names)
	if wait <= 0 {
		fmt.Println("      (add -buttons 20s to test each button by pressing it)")
		return
	}
	fmt.Printf("      press each button within %v; the LED lights while one is held...\n", wait)
	seen := map[core.Key]bool{}
	deadline := time.After(wait)
	for len(seen) < len(pins) {
		select {
		case ev := <-events:
			pad.SetLED(ev.Down)
			if ev.Down {
				fmt.Printf("      %s pressed\n", ev.Key)
				seen[ev.Key] = true
			}
		case <-deadline:
			var missing []string
			for k, pin := range pins {
				if !seen[k] {
					missing = append(missing, fmt.Sprintf("%s (GPIO%d)", k, pin))
				}
			}
			sort.Strings(missing)
			fail("buttons", "no press seen from: %v. Each switch should connect its GPIO pin to a ground pin.", missing)
			return
		}
	}
	ok("buttons", "all %d buttons respond", len(pins))
}
