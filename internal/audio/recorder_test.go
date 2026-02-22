package audio

import (
	"testing"
)

func TestNewRecorderAndClose(t *testing.T) {
	r, err := NewRecorder(16000, 1)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	if r.sampleRate != 16000 {
		t.Errorf("sampleRate = %d, want 16000", r.sampleRate)
	}
	if r.channels != 1 {
		t.Errorf("channels = %d, want 1", r.channels)
	}
}

func TestRecorderNotRecordingByDefault(t *testing.T) {
	r, err := NewRecorder(16000, 1)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer r.Close()

	if r.IsRecording() {
		t.Error("IsRecording() should be false after creation")
	}
}

func TestStopWithoutStart(t *testing.T) {
	r, err := NewRecorder(16000, 1)
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}
	defer r.Close()

	samples := r.Stop()
	if samples != nil {
		t.Errorf("Stop() without Start() should return nil, got %d samples", len(samples))
	}
}

func TestBytesToFloat32(t *testing.T) {
	// Test with known float32 value: 1.0 = 0x3F800000
	data := []byte{0x00, 0x00, 0x80, 0x3F} // 1.0 in little-endian float32
	samples := bytesToFloat32(data, 1)

	if len(samples) != 1 {
		t.Fatalf("bytesToFloat32() returned %d samples, want 1", len(samples))
	}
	if samples[0] != 1.0 {
		t.Errorf("bytesToFloat32() = %f, want 1.0", samples[0])
	}
}

func TestBytesToFloat32Multiple(t *testing.T) {
	// Two samples: 0.0 and -1.0
	// 0.0 = 0x00000000, -1.0 = 0xBF800000
	data := []byte{
		0x00, 0x00, 0x00, 0x00, // 0.0
		0x00, 0x00, 0x80, 0xBF, // -1.0
	}
	samples := bytesToFloat32(data, 2)

	if len(samples) != 2 {
		t.Fatalf("bytesToFloat32() returned %d samples, want 2", len(samples))
	}
	if samples[0] != 0.0 {
		t.Errorf("samples[0] = %f, want 0.0", samples[0])
	}
	if samples[1] != -1.0 {
		t.Errorf("samples[1] = %f, want -1.0", samples[1])
	}
}
