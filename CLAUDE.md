# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gostt-writer is a local real-time dictation tool for macOS. Press a hotkey, speak, and words are typed into the active application. All processing is on-device — no cloud services. Built with CGO (required for whisper.cpp bindings and CoreML bridge). macOS/Apple Silicon only.

Two transcription backends: **whisper** (whisper.cpp via Metal GPU) and **parakeet** (CoreML on Apple Neural Engine). Three text injection methods: `type` (keystroke simulation), `paste` (clipboard+Cmd+V), `ble` (encrypted Bluetooth to ESP32-S3).

## Build System

Uses [Task](https://taskfile.dev) (`go-task`), NOT `make`. All commands go through `Taskfile.yml`.

```bash
task                # Build everything (whisper.cpp + model + binary)
task build          # Build binary only (requires whisper already built)
task test           # Run all tests: go test -v ./...
task bench          # Transcription benchmarks: go test -bench=. -benchtime=3x ./internal/transcribe/
task run            # Build and run
task clean          # Remove bin/ and whisper.cpp build dir
task install        # Build, download models, install to /usr/local/bin
task models         # Interactive model download (whisper, parakeet, or both)
task config         # Interactive configuration editor (all settings)
task ollama-setup   # Install Ollama, pull model for LLM rewriting
task ollama-check   # Verify Ollama is running and model is available
```

Submodules required before first build:
```bash
git submodule update --init --recursive
```

### CGO Environment

The Taskfile sets CGO env vars automatically for build/test/run. When running `go test` or `golangci-lint` directly outside of Task, you need:
- `CGO_ENABLED=1`
- `C_INCLUDE_PATH` pointing to whisper.cpp include dirs
- `LIBRARY_PATH` pointing to whisper.cpp static library dirs
- macOS linker flags: `-framework Foundation -framework Metal -framework MetalKit -framework CoreML -lggml-metal -lggml-blas`

### Firmware (ESP32-S3)

```bash
task fw-build       # Build firmware
task fw-flash       # Flash to device
task fw-test        # Host-side C protobuf tests (firmware/esp32/test/)
task ble-pair       # ECDH key exchange pairing
```

## Architecture

Entry point: `cmd/gostt-writer/main.go`. All application packages are under `internal/`.

### Main flow
1. Parse CLI flags → load/validate YAML config → init slog
2. Init `transcribe.Transcriber` (whisper or parakeet)
3. Init `audio.Recorder` (miniaudio/malgo)
4. Init `inject.TextInjector` (type, paste, or BLE)
5. Init `rewrite.Rewriter` (optional, if `rewrite.enabled`)
6. Init `hotkey.Listener` (gohook/CGEventTap)
7. Event loop runs in a goroutine; hotkey listener runs on **main OS thread** via `runtime.LockOSThread()` (required by CGEventTap/CFRunLoop on macOS — moving it off the main thread causes deadlock)

### Key packages

| Package | Purpose |
|---|---|
| `internal/audio` | Microphone capture via malgo/miniaudio |
| `internal/transcribe` | `Transcriber` interface + whisper/parakeet backends |
| `internal/inject` | `TextInjector` interface — keystroke, clipboard, or BLE |
| `internal/hotkey` | Global hotkey listener (hold and toggle modes) |
| `internal/ble` | BLE client, ECDH pairing, AES-256-GCM crypto, hand-written protobuf |
| `internal/config` | YAML config loading, defaults, validation |
| `internal/rewrite` | LLM post-processing via local Ollama (stdlib net/http) |
| `internal/models` | Model download from HuggingFace (stdlib net/http) |
| `internal/coreml` | CGO bridge to Apple CoreML (Objective-C in bridge.m) |

### Key interfaces
- `transcribe.Transcriber` — `Process(samples []float32) (string, error)` + `Close() error`
- `inject.TextInjector` — `Inject(text string) error`
- `ble.Adapter`, `ble.Connection`, `ble.Characteristic` — abstracted for testing with mocks

### Parakeet pipeline (4-stage CoreML)
Preprocessor (mel spectrogram) → Encoder (acoustic features) → Decoder (RNNT step) → JointDecision (token prediction)

### BLE protocol
Hand-written protobuf (no .proto files). AES-256-GCM encryption per packet. ECDH P-256 pairing with HKDF-SHA256 (info=`"toothpaste"`). MTU chunking at 213 bytes with word-boundary/UTF-8 safe splits.

## Code Conventions

- Error wrapping: `fmt.Errorf("package: context: %w", err)` with package name prefix
- Logging: `log/slog` with structured key-value pairs to stderr
- Interfaces for testability with compile-time checks: `var _ TextInjector = (*Injector)(nil)`
- Concurrency: `sync.Mutex` for shared state, `sync.Once` for one-shot ops, `sync/atomic` for flags
- Commit prefixes: `feat:`, `fix:`, `docs:`, `chore:`, `ci:`, `deps:`, `test:`

## Config

Default path: `~/.config/gostt-writer/config.yaml` (auto-created on first run)
Models path: `~/.local/share/gostt-writer/models/`
Reference: `config.example.yaml`

## CI

All CI runs on `macos-15` (Apple Silicon). Three workflows:
- `build-test.yml` — build + test on push/PR to main
- `lint.yml` — golangci-lint on all branches/PRs
- `release.yml` — GoReleaser on `v*` tags (darwin/arm64 only)

## Issue Tracker

Uses **bd** (beads) for local issue tracking (`.beads/` directory). See `AGENTS.md` for workflow.
