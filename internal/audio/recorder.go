package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/gen2brain/malgo"
)

// Recorder captures audio from the default microphone into a float32 buffer.
type Recorder struct {
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	sampleRate uint32
	channels   uint32

	mu        sync.Mutex
	buf       []float32
	recording bool
}

// NewRecorder creates a new audio recorder. Call Close() when done.
func NewRecorder(sampleRate, channels uint32) (*Recorder, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initializing audio context: %w", err)
	}

	r := &Recorder{
		ctx:        ctx,
		sampleRate: sampleRate,
		channels:   channels,
	}

	return r, nil
}

// Start begins capturing audio from the default microphone.
// Audio samples are accumulated in an internal buffer as float32 values.
func (r *Recorder) Start() error {
	r.mu.Lock()
	if r.recording {
		r.mu.Unlock()
		return fmt.Errorf("already recording")
	}
	r.buf = r.buf[:0] // reset buffer but keep capacity
	r.recording = true
	r.mu.Unlock()

	deviceCfg := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceCfg.Capture.Format = malgo.FormatF32
	deviceCfg.Capture.Channels = r.channels
	deviceCfg.SampleRate = r.sampleRate

	callbacks := malgo.DeviceCallbacks{
		Data: r.onData,
	}

	device, err := malgo.InitDevice(r.ctx.Context, deviceCfg, callbacks)
	if err != nil {
		r.mu.Lock()
		r.recording = false
		r.mu.Unlock()
		return fmt.Errorf("initializing capture device: %w", err)
	}

	if err := device.Start(); err != nil {
		device.Uninit()
		r.mu.Lock()
		r.recording = false
		r.mu.Unlock()
		return fmt.Errorf("starting capture device: %w", err)
	}

	r.mu.Lock()
	r.device = device
	r.mu.Unlock()

	return nil
}

// Stop ends the audio capture and returns the recorded samples as float32.
// The returned slice can be passed directly to whisper.cpp for transcription.
func (r *Recorder) Stop() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.recording {
		return nil
	}

	if r.device != nil {
		r.device.Uninit()
		r.device = nil
	}
	r.recording = false

	// Return a copy of the buffer
	result := make([]float32, len(r.buf))
	copy(result, r.buf)

	return result
}

// IsRecording returns whether the recorder is currently capturing audio.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Close releases all audio resources.
func (r *Recorder) Close() error {
	r.mu.Lock()
	if r.device != nil {
		r.device.Uninit()
		r.device = nil
	}
	r.recording = false
	r.mu.Unlock()

	if r.ctx != nil {
		if err := r.ctx.Uninit(); err != nil {
			return fmt.Errorf("uninitializing audio context: %w", err)
		}
		r.ctx.Free()
	}

	return nil
}

// onData is the malgo callback invoked when audio data is available.
// pSample contains the captured audio frames as raw bytes (float32 format).
func (r *Recorder) onData(_, pSample []byte, frameCount uint32) {
	sampleCount := frameCount * r.channels
	samples := bytesToFloat32(pSample, sampleCount)

	r.mu.Lock()
	r.buf = append(r.buf, samples...)
	r.mu.Unlock()
}

// bytesToFloat32 converts raw bytes (little-endian float32) to a float32 slice.
func bytesToFloat32(data []byte, sampleCount uint32) []float32 {
	samples := make([]float32, 0, sampleCount)
	for i := uint32(0); i < sampleCount; i++ {
		offset := i * 4
		if offset+4 > uint32(len(data)) {
			break
		}
		bits := binary.LittleEndian.Uint32(data[offset : offset+4])
		samples = append(samples, math.Float32frombits(bits))
	}
	return samples
}
