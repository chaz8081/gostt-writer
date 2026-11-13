package audio

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tone(n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = 0.5 * float32(math.Sin(float64(i)/10))
	}
	return out
}

func TestWAVRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.wav")
	in := tone(2205)
	if err := WriteWAV(path, in, 22050); err != nil {
		t.Fatal(err)
	}
	out, rate, err := ReadWAV(path)
	if err != nil || rate != 22050 || len(out) != len(in) {
		t.Fatalf("rate %d, %d samples, err %v", rate, len(out), err)
	}
	for i := range in {
		if math.Abs(float64(in[i]-out[i])) > 1e-3 {
			t.Fatalf("sample %d: %v became %v", i, in[i], out[i])
		}
	}
	if _, _, err := ReadWAV(filepath.Join(t.TempDir(), "missing.wav")); err == nil {
		t.Error("no error for a missing file")
	}
}

func TestEncode16AppliesVolumeAndClips(t *testing.T) {
	b := Encode16([]float32{0.25, 0.9, -0.9}, 2)
	got := Decode16(b, 1)
	want := []float32{0.5, 1, -1}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-3 {
			t.Errorf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPlayCommandSendsAudioAndRate(t *testing.T) {
	dir := t.TempDir()
	cmd := "cat > " + filepath.Join(dir, "out-{rate}.raw")
	if err := PlayCommand(context.Background(), cmd, tone(100), 16000, 1); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "out-16000.raw"))
	if err != nil || len(b) != 200 {
		t.Fatalf("%d bytes, err %v", len(b), err)
	}
	if err := PlayCommand(context.Background(), "exit 3", tone(10), 16000, 1); err == nil {
		t.Error("no error from a failing command")
	}
}

func TestPlayCommandStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	start := time.Now()
	err := PlayCommand(ctx, "sleep 30", tone(10), 16000, 1)
	if err != context.Canceled || time.Since(start) > 3*time.Second {
		t.Errorf("err %v after %v", err, time.Since(start))
	}
}

func TestRunCommandDeliversBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.raw")
	if err := os.WriteFile(path, Encode16(tone(2*BlockSamples), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	out := make(chan []float32, 8)
	err := RunCommand(context.Background(), "cat "+path, 1, out)
	if err == nil {
		t.Error("a capture command that ends should be an error")
	}
	if len(out) != 2 || len(<-out) != BlockSamples {
		t.Errorf("got %d blocks", len(out)+1)
	}
}
