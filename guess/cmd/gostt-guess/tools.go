package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/chaz8081/gostt-writer/guess/internal/audio"
	"github.com/chaz8081/gostt-writer/guess/internal/gpio"
	"github.com/chaz8081/gostt-writer/guess/internal/session"
	"github.com/chaz8081/gostt-writer/guess/internal/speech"
)

// voiceCmd makes every clip the game can need, so nothing has to be made
// while a child is waiting. With -say it speaks one line instead.
func voiceCmd(ctx context.Context, args []string) error {
	c := newCommon("voice")
	say := c.fs.String("say", "", "speak this line through the speaker and stop")
	cfg, pack, err := c.load(args)
	if err != nil {
		return err
	}
	voice, err := speech.NewVoice(speech.VoiceConfig{Dir: cfg.Voice.Dir, Threads: cfg.Voice.Threads, Speed: cfg.Voice.Speed, Cache: cfg.Voice.Cache})
	if err != nil {
		return err
	}
	defer voice.Close()
	if *say != "" {
		samples, rate, err := voice.Clip(*say)
		if err != nil {
			return err
		}
		return audio.PlayCommand(ctx, cfg.Audio.Play, samples, rate, cfg.Audio.Volume)
	}
	lines := session.Lines(pack)
	made, seconds := 0, 0.0
	start := time.Now()
	for i, line := range lines {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if voice.Cached(line) {
			continue
		}
		samples, rate, err := voice.Clip(line)
		if err != nil {
			return err
		}
		made++
		seconds += float64(len(samples)) / float64(rate)
		fmt.Printf("\r%d of %d", i+1, len(lines))
	}
	if made == 0 {
		fmt.Printf("all %d clips were already made\n", len(lines))
		return nil
	}
	took := time.Since(start).Seconds()
	fmt.Printf("\rmade %d clips (%d already there): %.0f s of speech in %.0f s\n", made, len(lines)-made, seconds, took)
	return nil
}

// checkCmd tests one part of the device at a time.
func checkCmd(ctx context.Context, args []string) error {
	c := newCommon("check")
	buttons := c.fs.Duration("buttons", 0, "also wait this long for each button to be pressed, for example 20s")
	cfg, pack, err := c.load(args)
	if err != nil {
		return err
	}
	failed := false
	report := func(part string, err error, okText string) {
		if err != nil {
			failed = true
			fmt.Printf("FAIL  %-10s %v\n", part, err)
			return
		}
		fmt.Printf("ok    %-10s %s\n", part, okText)
	}
	report("pack", nil, fmt.Sprintf("%d items, %d questions", len(pack.Items), len(pack.Questions)))

	asr, err := speech.NewASR(speech.ASRConfig{Dir: cfg.Hear.Dir, Threads: cfg.Hear.Threads, Decoding: cfg.Hear.Decoding,
		TailPadding: time.Duration(cfg.Hear.TailPaddingMs) * time.Millisecond})
	report("hearing", err, cfg.Hear.Dir)
	if asr != nil {
		defer asr.Close()
	}
	voice, err := speech.NewVoice(speech.VoiceConfig{Dir: cfg.Voice.Dir, Threads: cfg.Voice.Threads, Speed: cfg.Voice.Speed, Cache: cfg.Voice.Cache})
	var clip []float32
	var rate int
	if err == nil {
		defer voice.Close()
		clip, rate, err = voice.Clip("Say yes after the beep. Beep!")
	}
	report("voice", err, cfg.Voice.Dir)
	missing := 0
	for _, line := range session.Lines(pack) {
		if voice != nil && !voice.Cached(line) {
			missing++
		}
	}
	if missing > 0 {
		fmt.Printf("note  %-10s %d clips are not made yet; run: gostt-guess voice\n", "voice", missing)
	}

	// Speaker, then three seconds of microphone: one of quiet to measure
	// the room, and the rest for the word "yes".
	if clip != nil {
		report("speaker", audio.PlayCommand(ctx, cfg.Audio.Play, clip, rate, cfg.Audio.Volume), "played a line; if you did not hear it, check audio.play")
	}
	recorded, err := record(ctx, cfg.Audio.Capture, cfg.Audio.Gain, 3*time.Second)
	if err == nil && len(recorded) < audio.SampleRate {
		err = fmt.Errorf("the capture command produced only %d samples in 3 seconds", len(recorded))
	}
	if err != nil {
		report("microphone", err, "")
	} else {
		quietest, loudest := 1.0, 0.0
		for i := 0; i+audio.BlockSamples <= len(recorded); i += audio.BlockSamples {
			rms, _ := audio.Level(recorded[i : i+audio.BlockSamples])
			v := math.Pow(10, rms/20)
			quietest, loudest = min(quietest, v), max(loudest, v)
		}
		report("microphone", nil, fmt.Sprintf("room %.4f, loudest %.4f (listen.level is %.4f)", quietest, loudest, cfg.Listen.Level))
		switch {
		case loudest < cfg.Listen.Level:
			fmt.Printf("note  %-10s nothing was loud enough to count as speech; raise audio.gain to about %.0f, or speak closer\n",
				"microphone", math.Ceil(float64(cfg.Audio.Gain)*4*cfg.Listen.Level/max(loudest, 1e-4)))
		case quietest*3 > cfg.Listen.Level:
			fmt.Printf("note  %-10s the room is noisy; the device will adapt, but replies may take longer to register\n", "microphone")
		case loudest > 0.7:
			fmt.Printf("note  %-10s very loud; lower audio.gain\n", "microphone")
		}
		if asr != nil {
			asr.Accept(recorded)
			heard := asr.Finish()
			if heard == "" {
				fmt.Printf("note  %-10s no words were recognised in the recording\n", "hearing")
			} else {
				fmt.Printf("ok    %-10s heard %q\n", "hearing", heard)
			}
		}
	}

	if *buttons > 0 {
		report("buttons", checkButtons(ctx, cfg.GPIO.Chip, cfg.GPIO.Buttons(), cfg.GPIO.Light, *buttons), "every button responds")
	}
	if failed {
		return fmt.Errorf("some checks failed")
	}
	return nil
}

// record captures audio for d.
func record(ctx context.Context, command string, gain float32, d time.Duration) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	out := make(chan []float32, 64)
	errc := make(chan error, 1)
	go func() { errc <- audio.RunCommand(ctx, command, gain, out); close(out) }()
	var all []float32
	for block := range out {
		all = append(all, block...)
	}
	if err := <-errc; err != nil {
		if _, lookErr := exec.LookPath(strings.Fields(command)[0]); lookErr != nil {
			return nil, fmt.Errorf("%s is not installed", strings.Fields(command)[0])
		}
		return nil, err
	}
	return all, nil
}

// checkButtons waits for each configured button to be pressed once.
func checkButtons(ctx context.Context, chip string, pins map[string]int, light int, wait time.Duration) error {
	want := map[string]bool{}
	for name, pin := range pins {
		if pin >= 0 {
			want[name] = true
		}
	}
	presses := make(chan string, 16)
	panel, err := gpio.Open(chip, pins, light, 10*time.Millisecond, func(name string) { presses <- name })
	if err != nil {
		return err
	}
	defer panel.Close()
	panel.SetLight(true)
	fmt.Printf("press each button once (%d buttons, %v)\n", len(want), wait)
	timeout := time.After(wait)
	for len(want) > 0 {
		select {
		case name := <-presses:
			fmt.Printf("      %s pressed\n", name)
			delete(want, name)
		case <-timeout:
			var missing []string
			for name := range want {
				missing = append(missing, name)
			}
			return fmt.Errorf("no press seen from: %s", strings.Join(missing, ", "))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
