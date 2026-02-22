# Parakeet TDT CoreML Backend — Design Document

**Date:** 2026-02-22
**Status:** Approved
**Author:** AI-assisted (Claude + chaz8081)

## Goal

Add Parakeet TDT 0.6B v2 as an optional transcription backend, running via CoreML on Apple Neural Engine, while keeping whisper.cpp as the default. Stay entirely in the Go ecosystem by vendoring a minimal CoreML bridge (~500 lines of ObjC/Go CGo code).

## Motivation

- **Accuracy**: Parakeet TDT 0.6B averages 6.05% WER across 8 benchmarks vs whisper base.en at ~10-12% WER
- **Speed**: ~110x real-time on Apple Silicon (vs ~26x for whisper base.en on M4 Max)
- **On-device**: CoreML runs on the Apple Neural Engine — no GPU contention, low power
- **No ecosystem change**: Vendored bridge keeps the entire project in Go

## Architecture

### Transcriber Interface

Extract a `Transcriber` interface from the existing concrete type. The existing 2-method surface (`Process` + `Close`) becomes the interface contract:

```go
type Transcriber interface {
    Process(samples []float32) (string, error)
    Close() error
}
```

A factory function `New(cfg)` returns the appropriate backend based on config.

### Package Layout

```
internal/
    transcribe/
        transcriber.go         # Interface + New() factory
        whisper.go             # Whisper backend (renamed from transcribe.go)
        whisper_test.go        # Existing whisper tests (renamed)
        parakeet.go            # Parakeet backend: loads 4 CoreML models, orchestrates pipeline
        parakeet_decode.go     # TDT greedy decode algorithm (~150 lines)
        parakeet_vocab.go      # Vocabulary loader + SentencePiece token → text
        parakeet_test.go       # Parakeet integration + unit tests
    coreml/
        bridge.go              # Go CGo wrapper (vendored from go-coreml, ~200 lines)
        bridge.h               # C header (~50 lines)
        bridge.m               # ObjC implementation (~250 lines)
```

### Config Changes

New `transcribe` section in config. Old top-level `model_path` maps to `transcribe.model_path` for backward compatibility.

```yaml
transcribe:
  backend: whisper # "whisper" or "parakeet"
  model_path: models/ggml-base.en.bin # whisper only
  parakeet_model_dir: models/parakeet-tdt-v2/ # parakeet only
```

Default backend: `whisper` (existing behavior unchanged).

### CoreML Bridge (Vendored)

~500 lines copied from github.com/gomlx/go-coreml (Apache 2.0). Provides:

- `LoadModel(path string) (*Model, error)` — loads `.mlmodelc`
- `Model.Predict(inputNames, inputs, outputNames, outputs)` — MLMultiArray I/O
- `SetComputeUnits(units)` — select ANE/GPU/CPU
- Tensor creation/access for float32, float16, int32

Build: `-framework Foundation -framework CoreML` via CGo directives (ships with macOS).

### Parakeet Inference Pipeline

4 CoreML models called in sequence:

| Step | Model         | Called                | Inputs                                                      | Outputs                                        |
| ---- | ------------- | --------------------- | ----------------------------------------------------------- | ---------------------------------------------- |
| 1    | Preprocessor  | 1x per utterance      | `audio_signal` [1,N] f32, `audio_length` [1] i32            | mel features                                   |
| 2    | Encoder       | 1x per utterance      | preprocessor outputs                                        | `encoder` [1,T,1024] f32, `encoder_length` i32 |
| 3    | Decoder       | N times (decode loop) | `targets` [1,1] i32, `h_in`/`c_in` [2,1,640] f32            | `decoder` f32, `h_out`/`c_out` [2,1,640] f32   |
| 4    | JointDecision | N times (decode loop) | `encoder_step` [1,1024,1] f32, `decoder_step` [1,640,1] f32 | `token_id` [1,1,1] i32, `duration` [1,1,1] i32 |

### TDT Greedy Decode Algorithm

Ported from FluidAudio's TdtDecoderV3.swift:

1. Walk encoder frames 0..T
2. At each frame: call JointDecision with encoder frame + decoder state
3. If blank token (id=1024): advance by duration frames, no decoder update
4. If non-blank: emit token, call Decoder to update LSTM state, advance by duration
5. Duration bins: [0, 1, 2, 3, 4] — maps joint output to frame advance count
6. Guard: max 10 symbols per timestep

### Audio Chunking

For audio >15s (240,000 samples at 16kHz), split into chunks and stitch results. Deferred to a follow-up task if the initial implementation targets ≤15s utterances (typical for dictation).

### Key Constants

| Constant          | Value           |
| ----------------- | --------------- |
| sampleRate        | 16000           |
| maxModelSamples   | 240000 (15s)    |
| encoderHiddenSize | 1024            |
| decoderHiddenSize | 640             |
| blankId           | 1024 (v2)       |
| durationBins      | [0, 1, 2, 3, 4] |
| maxSymbolsPerStep | 10              |

## Model Distribution

Makefile target `make parakeet-model` downloads from HuggingFace:

```
models/parakeet-tdt-v2/
    Preprocessor.mlmodelc/    (compiled CoreML model bundle)
    Encoder.mlmodelc/
    Decoder.mlmodelc/
    JointDecision.mlmodelc/
    parakeet_vocab.json
```

Source: `FluidInference/parakeet-tdt-0.6b-v2-coreml` on HuggingFace.

## Risks

| Risk                              | Severity | Mitigation                                                               |
| --------------------------------- | -------- | ------------------------------------------------------------------------ |
| go-coreml bridge bugs             | Medium   | Bridge is ~500 lines, fully auditable; we vendor and own it              |
| CGo overhead in decode loop       | Low      | ~1000 calls x 200ns = 0.2ms — negligible vs model inference time         |
| CoreML model I/O shape mismatches | Medium   | Write integration tests that load real models and check I/O names/shapes |
| Long audio (>15s) handling        | Low      | Defer chunking to follow-up; typical dictation is <15s                   |
| Float16 output handling           | Medium   | Verify with real models; may need to force Float32 compute               |
