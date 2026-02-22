# gostt-writer Design Document

**Date:** 2026-02-21
**Status:** Approved

## Overview

gostt-writer (Go Speech-To-Text Writer) is a single-binary macOS CLI application that provides real-time local dictation. It listens for a configurable global hotkey, records audio from the default microphone, transcribes it locally using whisper.cpp, and types the result into the currently focused application via keystroke simulation.

## Core Data Flow

```
Global Hotkey (gohook) -> Audio Capture (malgo/CoreAudio) -> []float32 buffer
  -> Whisper Transcription (whisper.cpp Go bindings) -> text string
    -> Text Injection (robotgo.Type) -> active app cursor
```

## Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Audio library | malgo (miniaudio) | Zero external deps on macOS, uses CoreAudio natively |
| Whisper integration | Official Go bindings (in-process) | Native performance, no temp files or process overhead |
| Text injection | robotgo.Type() (keystroke simulation) | Preserves clipboard content; paste mode available as config option |
| Hotkey library | gohook (robotn/gohook) | Mature global event hook, supports key down/up events |
| Configuration | YAML file (~/.config/gostt-writer/config.yaml) | Human-readable, easy to edit |
| Transcription model | Async post-capture | Hold key -> release -> transcribe in goroutine -> type result. Sub-second on M4 Max with base.en model |
| Hotkey mode | Configurable: hold-to-talk (default) or toggle | User preference |
| Streaming transcription | Deferred to v2 | Complexity of partial text injection outweighs benefit for v1 |

## Package Structure

```
gostt-writer/
  cmd/
    gostt-writer/
      main.go              # Entry point, wires everything together
  internal/
    config/
      config.go            # YAML config loading, defaults, validation
    audio/
      recorder.go          # malgo-based mic capture -> []float32
    hotkey/
      hotkey.go            # gohook-based global hotkey listener
    transcribe/
      transcribe.go        # whisper.cpp Go bindings wrapper
    inject/
      inject.go            # robotgo text injection
  config.example.yaml      # Example config file
  go.mod
  go.sum
  Makefile                 # Build whisper.cpp, download models, build app
  third_party/
    whisper.cpp/           # git submodule
```

## Component Interfaces

### config

```go
type Config struct {
    ModelPath string       `yaml:"model_path"`
    Hotkey    HotkeyConfig `yaml:"hotkey"`
    Audio     AudioConfig  `yaml:"audio"`
    Inject    InjectConfig `yaml:"inject"`
    LogLevel  string       `yaml:"log_level"`
}

type HotkeyConfig struct {
    Keys []string `yaml:"keys"`
    Mode string   `yaml:"mode"` // "hold" or "toggle"
}

type AudioConfig struct {
    SampleRate uint32 `yaml:"sample_rate"`
    Channels   uint32 `yaml:"channels"`
}

type InjectConfig struct {
    Method string `yaml:"method"` // "type" or "paste"
}

func Load(path string) (*Config, error)
func Default() *Config
```

### audio

```go
type Recorder struct { ... }

func NewRecorder(sampleRate, channels uint32) (*Recorder, error)
func (r *Recorder) Start() error        // Begin capturing to internal buffer
func (r *Recorder) Stop() []float32     // Stop and return captured samples
func (r *Recorder) Close() error        // Release malgo resources
```

### hotkey

```go
type EventType int
const (
    EventStart EventType = iota  // Key pressed (hold) or first press (toggle)
    EventStop                     // Key released (hold) or second press (toggle)
)

type Event struct {
    Type EventType
}

func Listen(keys []string, mode string) (<-chan Event, func())
// Returns event channel and a cancel function
```

### transcribe

```go
type Transcriber struct { ... }

func NewTranscriber(modelPath string) (*Transcriber, error)
func (t *Transcriber) Process(samples []float32) (string, error)
func (t *Transcriber) Close() error
```

### inject

```go
type Injector struct { ... }

func NewInjector(method string) *Injector
func (i *Injector) Inject(text string) error
```

## Main Loop

```go
func main() {
    cfg := config.Load(configPath)

    transcriber := transcribe.NewTranscriber(cfg.ModelPath)
    defer transcriber.Close()

    recorder := audio.NewRecorder(cfg.Audio.SampleRate, cfg.Audio.Channels)
    defer recorder.Close()

    injector := inject.NewInjector(cfg.Inject.Method)

    events, cancel := hotkey.Listen(cfg.Hotkey.Keys, cfg.Hotkey.Mode)
    defer cancel()

    // Handle SIGINT/SIGTERM
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    for {
        select {
        case ev := <-events:
            switch ev.Type {
            case hotkey.EventStart:
                recorder.Start()
                log.Info("Recording...")
            case hotkey.EventStop:
                samples := recorder.Stop()
                if len(samples) < minSamples {
                    log.Warn("Recording too short, skipping")
                    continue
                }
                log.Info("Transcribing...")
                go func() {
                    text, err := transcriber.Process(samples)
                    if err != nil {
                        log.Error("Transcription failed", "err", err)
                        return
                    }
                    if text == "" {
                        log.Info("Empty transcription, skipping")
                        return
                    }
                    injector.Inject(text)
                    log.Info("Injected", "text", text)
                }()
            }
        case <-sigCh:
            log.Info("Shutting down...")
            return
        }
    }
}
```

## Configuration

Default config at `~/.config/gostt-writer/config.yaml`:

```yaml
model_path: ~/.local/share/gostt-writer/models/ggml-base.en.bin

hotkey:
  keys: ["ctrl", "shift", "r"]
  mode: hold  # "hold" or "toggle"

audio:
  sample_rate: 16000
  channels: 1

inject:
  method: type  # "type" or "paste"

log_level: info
```

## Build System

Makefile targets:
- `make whisper` - Clone/build whisper.cpp with Metal + Accelerate
- `make model` - Download ggml-base.en.bin
- `make build` - Build Go binary with CGO flags
- `make run` - Build and run
- `make clean` - Remove build artifacts

whisper.cpp build flags:
```bash
cmake -B build -DGGML_METAL=ON -DGGML_ACCELERATE=ON
cmake --build build --config Release
```

Go build:
```bash
CGO_ENABLED=1 \
C_INCLUDE_PATH=third_party/whisper.cpp/include \
LIBRARY_PATH=third_party/whisper.cpp/build/src \
go build -o bin/gostt-writer ./cmd/gostt-writer
```

## macOS Permissions

Required permissions (System Settings > Privacy & Security):
1. **Accessibility** - For robotgo (global hotkey + keystroke injection)
2. **Microphone** - For malgo audio capture

The app should detect missing permissions and print clear instructions.

## Error Handling

| Scenario | Handling |
|----------|----------|
| Model file missing | Fatal error with download instructions |
| Microphone permission denied | Detect from malgo error, print permission instructions |
| Accessibility not granted | Detect from gohook failure, print instructions |
| Very short recording (<0.5s) | Skip transcription, log warning |
| Very long recording (>30s) | Whisper processes first 30s, warn user |
| Empty transcription result | Skip injection, log info |
| Config file parse error | Fatal with line number |

## Future (v2)

- Streaming transcription with partial results
- macOS menu bar app with status indicator
- System notification support
- Multiple model support / model switching
- Custom vocabulary / prompt hints for technical terms
