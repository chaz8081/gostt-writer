//go:build linux

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/chaz8081/gostt-writer/pad/internal/audio"
	"github.com/chaz8081/gostt-writer/pad/internal/config"
)

// runBench transcribes WAV files as fast as the CPU allows and reports the
// speed, which shows whether a board can keep up with live speech.
func runBench(cfg config.Config, files []string) int {
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "bench needs at least one WAV file (16 kHz mono 16-bit)")
		return 2
	}
	rec, took, err := loadModel(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "the speech model did not load:", err)
		return 1
	}
	defer rec.Close()
	fmt.Printf("model loaded in %v (%d threads, %s)\n", took.Round(time.Millisecond), cfg.Model.Threads, cfg.Model.Decoding)

	var audioTotal, cpuTotal time.Duration
	failed := false
	for _, path := range files {
		samples, err := audio.ReadWAV(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}
		rec.Reset()
		start := time.Now()
		for i := 0; i < len(samples); i += audio.BlockSamples {
			rec.Accept(samples[i:min(i+audio.BlockSamples, len(samples))])
		}
		text := rec.Finish()
		cpu := time.Since(start)
		dur := time.Duration(len(samples)) * time.Second / audio.SampleRate
		audioTotal += dur
		cpuTotal += cpu
		fmt.Printf("\n%s\n  audio %.1f s, processed in %.1f s, speed %.2f\n  %s\n", path, dur.Seconds(), cpu.Seconds(), cpu.Seconds()/dur.Seconds(), text)
	}
	if audioTotal > 0 {
		speed := cpuTotal.Seconds() / audioTotal.Seconds()
		verdict := "fast enough for live dictation"
		switch {
		case speed >= 1:
			verdict = "TOO SLOW for live dictation: it would fall further behind the longer you speak"
		case speed >= 0.7:
			verdict = "only just fast enough; expect delays on long sentences"
		}
		fmt.Printf("\noverall speed %.2f (processing time / audio time; must stay below 1.0): %s\n", speed, verdict)
	}
	if failed {
		return 1
	}
	return 0
}
