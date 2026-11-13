// Package audio moves sound in and out of the device: microphone audio from
// a capture command such as arecord, speech out through a playback command
// such as aplay, and WAV files for the voice clips and for testing.
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

// SampleRate is the microphone rate the speech model needs.
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

// ReadWAV loads a mono 16-bit PCM WAV file and returns its sample rate.
func ReadWAV(path string) (samples []float32, rate int, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("audio: %w", err)
	}
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("audio: %s is not a WAV file", path)
	}
	var channels, bits, format int
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
				return nil, 0, fmt.Errorf("audio: %s has a short format chunk", path)
			}
			format = int(binary.LittleEndian.Uint16(body))
			channels = int(binary.LittleEndian.Uint16(body[2:]))
			rate = int(binary.LittleEndian.Uint32(body[4:]))
			bits = int(binary.LittleEndian.Uint16(body[14:]))
		case "data":
			if format != 1 || channels != 1 || bits != 16 || rate <= 0 {
				return nil, 0, fmt.Errorf("audio: %s must be mono 16-bit PCM (it is format %d, %d channel(s), %d-bit)", path, format, channels, bits)
			}
			return Decode16(body[:size-size%2], 1), rate, nil
		}
		p += 8 + size + size%2
	}
	return nil, 0, fmt.Errorf("audio: %s has no audio data", path)
}

// WriteWAV saves samples as a mono 16-bit PCM WAV file. It writes a
// temporary file first, so a clip is never left half written.
func WriteWAV(path string, samples []float32, rate int) error {
	data := Encode16(samples, 1)
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(2*rate))
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(h, data...), 0o644); err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("audio: %w", err)
	}
	return nil
}

// Encode16 converts floats to 16-bit little-endian samples, scaled by
// volume and clipped.
func Encode16(samples []float32, volume float32) []byte {
	out := make([]byte, 2*len(samples))
	for i, s := range samples {
		v := max(-1, min(1, s*volume))
		binary.LittleEndian.PutUint16(out[2*i:], uint16(int16(v*32767)))
	}
	return out
}

// PlayCommand plays samples through a shell command that reads raw 16-bit
// little-endian mono audio on its standard input. "{rate}" in the command
// is replaced by the sample rate. It returns when playback has finished, or
// stops the sound and returns the context's error when ctx is cancelled.
func PlayCommand(ctx context.Context, command string, samples []float32, rate int, volume float32) error {
	command = strings.ReplaceAll(command, "{rate}", fmt.Sprint(rate))
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdin = bytes.NewReader(Encode16(samples, volume))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("audio: playback command %q: %w: %s", command, err, strings.TrimSpace(stderr.String()))
	}
	return nil
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
