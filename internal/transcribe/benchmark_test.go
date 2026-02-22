package transcribe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-audio/wav"
)

// benchSample holds a test audio sample and its reference transcript.
type benchSample struct {
	Label      string  `json:"label"`
	File       string  `json:"file"`
	Transcript string  `json:"transcript"`
	DurationS  float64 `json:"duration_sec"`
}

// benchReferences is the top-level structure of testdata/references.json.
type benchReferences struct {
	Samples []benchSample `json:"samples"`
}

// loadBenchSamples reads references.json and loads all audio samples.
// benchSampleWithAudio pairs a benchSample with its decoded audio data.
type benchSampleWithAudio struct {
	benchSample
	audio []float32
}

// loadBenchSamples reads references.json and loads all audio samples.
func loadBenchSamples(b *testing.B) []benchSampleWithAudio {
	b.Helper()

	refPath := filepath.Join("testdata", "references.json")
	data, err := os.ReadFile(refPath)
	if err != nil {
		b.Fatalf("read references.json: %v", err)
	}

	var refs benchReferences
	if err := json.Unmarshal(data, &refs); err != nil {
		b.Fatalf("parse references.json: %v", err)
	}

	results := make([]benchSampleWithAudio, 0, len(refs.Samples))

	for _, s := range refs.Samples {
		wavPath := filepath.Join("testdata", s.File)
		f, err := os.Open(wavPath)
		if err != nil {
			b.Skipf("WAV file not found at %s: %v", wavPath, err)
		}

		dec := wavDecode(f)
		_ = f.Close()
		if dec == nil {
			b.Fatalf("failed to decode WAV %s", wavPath)
		}

		results = append(results, benchSampleWithAudio{
			benchSample: s,
			audio:       dec,
		})
	}

	return results
}

// wavDecode opens and decodes a WAV file from an os.File, returning float32
// samples normalized to [-1.0, 1.0]. Returns nil on error.
func wavDecode(f *os.File) []float32 {
	dec := wav.NewDecoder(f)
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil
	}
	samples := make([]float32, len(buf.Data))
	for i, s := range buf.Data {
		samples[i] = float32(s) / 32768.0
	}
	return samples
}

func BenchmarkWhisperProcess(b *testing.B) {
	modelPath := filepath.Join("..", "..", "models", "ggml-base.en.bin")
	if _, err := os.Stat(modelPath); err != nil {
		b.Skipf("whisper model not found at %s (run 'task whisper-model')", modelPath)
	}

	samples := loadBenchSamples(b)

	tr, err := NewWhisperTranscriber(modelPath)
	if err != nil {
		b.Fatalf("NewWhisperTranscriber: %v", err)
	}
	defer func() { _ = tr.Close() }()

	for _, s := range samples {
		s := s // capture
		b.Run(s.Label, func(b *testing.B) {
			// Report audio duration as a custom metric
			b.ReportMetric(s.DurationS*1000, "audio-ms")

			// Warm up: single run outside the loop
			_, _ = tr.Process(s.audio)

			var lastText string
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				text, err := tr.Process(s.audio)
				if err != nil {
					b.Fatalf("Process: %v", err)
				}
				lastText = text
			}
			b.StopTimer()

			// Compute and report RTF and WER after the benchmark loop
			elapsed := b.Elapsed()
			rtf := (elapsed.Seconds() / float64(b.N)) / s.DurationS
			b.ReportMetric(rtf, "rtf")

			wer := ComputeWER(s.Transcript, lastText)
			b.ReportMetric(wer.WER, "wer")
		})
	}
}

func BenchmarkParakeetProcess(b *testing.B) {
	modelDir := filepath.Join("..", "..", "models", "parakeet-tdt-v2")
	if _, err := os.Stat(filepath.Join(modelDir, "Encoder.mlmodelc")); err != nil {
		b.Skipf("parakeet models not found at %s (run 'task parakeet-model')", modelDir)
	}

	samples := loadBenchSamples(b)

	tr, err := NewParakeetTranscriber(modelDir)
	if err != nil {
		b.Fatalf("NewParakeetTranscriber: %v", err)
	}
	defer func() { _ = tr.Close() }()

	for _, s := range samples {
		s := s // capture
		b.Run(s.Label, func(b *testing.B) {
			// Report audio duration as a custom metric
			b.ReportMetric(s.DurationS*1000, "audio-ms")

			// Warm up: single run outside the loop
			_, _ = tr.Process(s.audio)

			var lastText string
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				text, err := tr.Process(s.audio)
				if err != nil {
					b.Fatalf("Process: %v", err)
				}
				lastText = text
			}
			b.StopTimer()

			// Compute and report RTF and WER after the benchmark loop
			elapsed := b.Elapsed()
			rtf := (elapsed.Seconds() / float64(b.N)) / s.DurationS
			b.ReportMetric(rtf, "rtf")

			wer := ComputeWER(s.Transcript, lastText)
			b.ReportMetric(wer.WER, "wer")
		})
	}
}

// BenchmarkWhisperLatency measures first-call latency (cold start after model load).
func BenchmarkWhisperLatency(b *testing.B) {
	modelPath := filepath.Join("..", "..", "models", "ggml-base.en.bin")
	if _, err := os.Stat(modelPath); err != nil {
		b.Skipf("whisper model not found at %s (run 'task whisper-model')", modelPath)
	}

	// Use short sample only for latency measurement
	wavPath := filepath.Join("testdata", "short.wav")
	f, err := os.Open(wavPath)
	if err != nil {
		b.Skipf("short.wav not found: %v", err)
	}
	audio := wavDecode(f)
	_ = f.Close()
	if audio == nil {
		b.Fatal("failed to decode short.wav")
	}

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tr, err := NewWhisperTranscriber(modelPath)
		if err != nil {
			b.Fatalf("NewWhisperTranscriber: %v", err)
		}
		b.StartTimer()

		start := time.Now()
		_, err = tr.Process(audio)
		latency := time.Since(start)
		if err != nil {
			_ = tr.Close()
			b.Fatalf("Process: %v", err)
		}

		b.StopTimer()
		tr.Close()
		b.ReportMetric(float64(latency.Milliseconds()), "first-call-ms")
	}
}

// BenchmarkParakeetLatency measures first-call latency (cold start after model load).
func BenchmarkParakeetLatency(b *testing.B) {
	modelDir := filepath.Join("..", "..", "models", "parakeet-tdt-v2")
	if _, err := os.Stat(filepath.Join(modelDir, "Encoder.mlmodelc")); err != nil {
		b.Skipf("parakeet models not found at %s (run 'task parakeet-model')", modelDir)
	}

	// Use short sample only for latency measurement
	wavPath := filepath.Join("testdata", "short.wav")
	f, err := os.Open(wavPath)
	if err != nil {
		b.Skipf("short.wav not found: %v", err)
	}
	audio := wavDecode(f)
	_ = f.Close()
	if audio == nil {
		b.Fatal("failed to decode short.wav")
	}

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tr, err := NewParakeetTranscriber(modelDir)
		if err != nil {
			b.Fatalf("NewParakeetTranscriber: %v", err)
		}
		b.StartTimer()

		start := time.Now()
		_, err = tr.Process(audio)
		latency := time.Since(start)
		if err != nil {
			tr.Close()
			b.Fatalf("Process: %v", err)
		}

		b.StopTimer()
		tr.Close()
		b.ReportMetric(float64(latency.Milliseconds()), "first-call-ms")
	}
}
