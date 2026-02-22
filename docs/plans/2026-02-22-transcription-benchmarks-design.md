# Transcription Benchmarks Design

## Goal

Add Go benchmarks measuring transcription speed (RTF) and accuracy (WER) for both Whisper and Parakeet backends, enabling objective comparison between models.

## Context

gostt-writer has two transcription backends:
- **Whisper** (whisper.cpp via Go bindings) - CPU/GPU with Metal, ~26x real-time on M4 Max
- **Parakeet TDT 0.6B v2** (CoreML on ANE) - ~110x real-time on M4 Max

Currently these numbers are anecdotal. No benchmarks exist in the project's own code.

## Design

### WER (Word Error Rate)

Standard speech-to-text accuracy metric:

```
WER = (Substitutions + Insertions + Deletions) / Reference Word Count
```

Implemented via minimum edit distance (dynamic programming) on normalized word arrays. Normalization: lowercase, strip punctuation, collapse whitespace.

Exported as `transcribe.ComputeWER()` with a `WERResult` struct containing WER, substitution/insertion/deletion counts, and reference word count.

### Test Audio

Multiple audio samples from LibriSpeech (public domain, industry standard):

| Sample | Duration | Purpose |
|--------|----------|---------|
| JFK (existing) | ~11s | Baseline |
| Short phrase | ~3s | Short-utterance latency |
| Medium passage | ~8s | Typical dictation length |
| Long passage | ~14s | Near Parakeet's 15s cap |

Reference transcripts stored in `testdata/references.json`.

### Benchmark Functions

Standard Go `testing.B` benchmarks with sub-benchmarks per audio sample:

```
BenchmarkWhisperProcess/jfk
BenchmarkWhisperProcess/short
BenchmarkParakeetProcess/jfk
BenchmarkParakeetProcess/short
```

Custom metrics reported per benchmark:
- **RTF** (Real-Time Factor) = processing time / audio duration (lower = faster)
- **WER** = word error rate against reference transcript (lower = more accurate)
- **audio-ms** = audio duration in milliseconds for context

Models loaded once outside the benchmark loop. Backends skip if models not downloaded.

### Constraints

- Parakeet has a hard 15s / 240k sample cap - long sample stays under this
- Both backends skip gracefully if model files missing (via `b.Skip`)
- Audio helper extracted from existing `jfkSamples()` into generic `loadWAVSamples()`
