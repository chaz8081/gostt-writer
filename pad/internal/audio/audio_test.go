package audio

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pcm(samples ...int16) []byte {
	b := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(s))
	}
	return b
}

func writeWAV(t *testing.T, channels, rate, bits int, data []byte, extraChunk bool) string {
	t.Helper()
	var b []byte
	le16 := func(v int) { b = binary.LittleEndian.AppendUint16(b, uint16(v)) }
	le32 := func(v int) { b = binary.LittleEndian.AppendUint32(b, uint32(v)) }
	b = append(b, "RIFF"...)
	le32(0)
	b = append(b, "WAVE"...)
	if extraChunk { // an odd-sized chunk before fmt, as some tools write
		b = append(b, "LIST"...)
		le32(3)
		b = append(b, 'a', 'b', 'c', 0)
	}
	b = append(b, "fmt "...)
	le32(16)
	le16(1)
	le16(channels)
	le32(rate)
	le32(rate * channels * bits / 8)
	le16(channels * bits / 8)
	le16(bits)
	b = append(b, "data"...)
	le32(len(data))
	b = append(b, data...)
	path := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDecode16(t *testing.T) {
	got := Decode16(pcm(0, 16384, -32768, 32767), 1)
	want := []float32{0, 0.5, -1, 32767.0 / 32768}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
	if got := Decode16(pcm(16384, -16384), 4); got[0] != 1 || got[1] != -1 {
		t.Errorf("gain must clip to [-1, 1]; got %v", got)
	}
}

func TestReadWAV(t *testing.T) {
	for _, extra := range []bool{false, true} {
		got, err := ReadWAV(writeWAV(t, 1, 16000, 16, pcm(0, 16384, -16384), extra))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[1] != 0.5 || got[2] != -0.5 {
			t.Errorf("extra chunk %v: got %v", extra, got)
		}
	}
	bad := map[string]string{
		"stereo":      writeWAV(t, 2, 16000, 16, pcm(0, 0), false),
		"wrong rate":  writeWAV(t, 1, 44100, 16, pcm(0, 0), false),
		"wrong depth": writeWAV(t, 1, 16000, 8, []byte{0, 0}, false),
	}
	for name, path := range bad {
		if _, err := ReadWAV(path); err == nil || !strings.Contains(err.Error(), "ffmpeg") {
			t.Errorf("%s: want an error that says how to convert, got %v", name, err)
		}
	}
	notWAV := filepath.Join(t.TempDir(), "x.wav")
	_ = os.WriteFile(notWAV, []byte("hello world, not audio"), 0o600)
	if _, err := ReadWAV(notWAV); err == nil {
		t.Error("want an error for a non-WAV file")
	}
	if _, err := ReadWAV(filepath.Join(t.TempDir(), "missing.wav")); err == nil {
		t.Error("want an error for a missing file")
	}
}

func TestRunCommand(t *testing.T) {
	raw := filepath.Join(t.TempDir(), "a.raw")
	samples := make([]int16, BlockSamples*2+10) // two full blocks and a short one
	for i := range samples {
		samples[i] = 8192
	}
	if err := os.WriteFile(raw, pcm(samples...), 0o600); err != nil {
		t.Fatal(err)
	}
	out := make(chan []float32, 8)
	err := RunCommand(context.Background(), "cat "+raw+"; echo mic unplugged >&2; exit 3", 2, out)
	if err == nil || !strings.Contains(err.Error(), "mic unplugged") {
		t.Fatalf("want the command's own message in the error, got %v", err)
	}
	close(out)
	var n int
	for b := range out {
		n += len(b)
		if b[0] != 0.5 {
			t.Fatalf("gain not applied: %v", b[0])
		}
	}
	if n != len(samples) {
		t.Errorf("got %d samples, want %d", n, len(samples))
	}
}

func TestRunCommandCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan []float32, 1)
	done := make(chan error, 1)
	go func() { done <- RunCommand(ctx, "cat /dev/zero", 1, out) }()
	<-out
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("cancel must not be an error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunCommand did not stop after cancel")
	}
}

func TestRunCommandMissing(t *testing.T) {
	err := RunCommand(context.Background(), "definitely-not-a-command-xyz", 1, make(chan []float32, 1))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want a not-found error, got %v", err)
	}
}

func TestPlay(t *testing.T) {
	out := make(chan []float32, 8)
	Play(context.Background(), make([]float32, BlockSamples*2+5), false, out)
	close(out)
	var sizes []int
	for b := range out {
		sizes = append(sizes, len(b))
	}
	if len(sizes) != 3 || sizes[2] != 5 {
		t.Errorf("block sizes = %v", sizes)
	}

	start := time.Now()
	out = make(chan []float32, 8)
	Play(context.Background(), make([]float32, BlockSamples*3), true, out)
	if d := time.Since(start); d < 250*time.Millisecond {
		t.Errorf("paced playback of 300 ms took %v", d)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Play(ctx, make([]float32, BlockSamples*100), true, make(chan []float32)) // must return, not block
}

func TestLevel(t *testing.T) {
	if rms, peak := Level(nil); rms != -120 || peak != -120 {
		t.Errorf("empty: %v %v", rms, peak)
	}
	if rms, peak := Level(make([]float32, 100)); rms != -120 || peak != -120 {
		t.Errorf("silence: %v %v", rms, peak)
	}
	rms, peak := Level([]float32{0.5, -0.5, 0.5, -0.5})
	if rms > -6 || rms < -6.1 || peak > -6 || peak < -6.1 {
		t.Errorf("half scale should be about -6 dBFS: rms %v peak %v", rms, peak)
	}
}
