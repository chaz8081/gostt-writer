// Package audio gets 16 kHz mono audio into the pad: from a capture command
// such as arecord, or from a WAV file for testing.
package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// SampleRate is the only rate the pad accepts.
const SampleRate = 16000

// BlockSamples is how much audio is delivered at a time: 100 ms.
const BlockSamples = SampleRate / 10

// RunCommand runs a shell command that writes raw 16-bit little-endian mono
// 16 kHz audio to its standard output, and sends it to out in 100 ms blocks
// scaled by gain. It returns when ctx is cancelled or the command ends; a
// command that ends on its own is an error.
func RunCommand(ctx context.Context, command string, gain float32, out chan<- []float32) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	// The shell usually starts the recorder as a child. Stop the whole
	// group, or the child would keep running and hold the pipe open.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("audio: pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("audio: start %q: %w", command, err)
	}
	readErr := readPCM(ctx, bufio.NewReaderSize(stdout, 4*BlockSamples), gain, out)
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil
	}
	msg := strings.TrimSpace(stderr.String())
	if len(msg) > 400 {
		msg = msg[len(msg)-400:]
	}
	if msg == "" {
		msg = "no error output"
	}
	return fmt.Errorf("audio: capture command ended (%v): %s", errors.Join(readErr, waitErr), msg)
}

// readPCM converts a raw PCM16 stream into float blocks until it ends.
func readPCM(ctx context.Context, r io.Reader, gain float32, out chan<- []float32) error {
	buf := make([]byte, 2*BlockSamples)
	for {
		n, err := io.ReadFull(r, buf)
		if n >= 2 {
			block := Decode16(buf[:n-n%2], gain)
			select {
			case out <- block:
			case <-ctx.Done():
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
	}
}

// Decode16 converts 16-bit little-endian samples to floats in [-1, 1],
// scaled by gain and clipped.
func Decode16(b []byte, gain float32) []float32 {
	out := make([]float32, len(b)/2)
	for i := range out {
		v := float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / 32768 * gain
		out[i] = max(-1, min(1, v))
	}
	return out
}

// ReadWAV loads a 16 kHz mono 16-bit PCM WAV file.
func ReadWAV(path string) ([]float32, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("audio: %w", err)
	}
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, fmt.Errorf("audio: %s is not a WAV file", path)
	}
	var channels, rate, bits, format int
	for p := 12; p+8 <= len(b); {
		id := string(b[p : p+4])
		size := int(binary.LittleEndian.Uint32(b[p+4:]))
		body := b[p+8:]
		if size > len(body) {
			size = len(body)
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, fmt.Errorf("audio: %s has a short format chunk", path)
			}
			format = int(binary.LittleEndian.Uint16(body))
			channels = int(binary.LittleEndian.Uint16(body[2:]))
			rate = int(binary.LittleEndian.Uint32(body[4:]))
			bits = int(binary.LittleEndian.Uint16(body[14:]))
		case "data":
			if format != 1 || channels != 1 || rate != SampleRate || bits != 16 {
				return nil, fmt.Errorf("audio: %s must be 16 kHz mono 16-bit PCM (it is format %d, %d channel(s), %d Hz, %d-bit); "+
					"convert it with: ffmpeg -i in.wav -ar 16000 -ac 1 -c:a pcm_s16le out.wav", path, format, channels, rate, bits)
			}
			return Decode16(body[:size-size%2], 1), nil
		}
		p += 8 + size + size%2
	}
	return nil, fmt.Errorf("audio: %s has no audio data", path)
}

// Play sends samples to out in 100 ms blocks. With pace it waits between
// blocks so the audio arrives at speaking speed, as from a microphone.
func Play(ctx context.Context, samples []float32, pace bool, out chan<- []float32) {
	var tick *time.Ticker
	if pace {
		tick = time.NewTicker(time.Second * BlockSamples / SampleRate)
		defer tick.Stop()
	}
	for i := 0; i < len(samples); i += BlockSamples {
		block := samples[i:min(i+BlockSamples, len(samples))]
		select {
		case out <- block:
		case <-ctx.Done():
			return
		}
		if tick != nil {
			select {
			case <-tick.C:
			case <-ctx.Done():
				return
			}
		}
	}
}

// Level returns the RMS and peak of samples in dBFS (0 is full scale).
// Silence is reported as -120.
func Level(samples []float32) (rms, peak float64) {
	var sum, pk float64
	for _, s := range samples {
		v := math.Abs(float64(s))
		sum += v * v
		pk = max(pk, v)
	}
	db := func(v float64) float64 {
		if v <= 1e-6 {
			return -120
		}
		return 20 * math.Log10(v)
	}
	if len(samples) == 0 {
		return -120, -120
	}
	return db(math.Sqrt(sum / float64(len(samples)))), db(pk)
}
