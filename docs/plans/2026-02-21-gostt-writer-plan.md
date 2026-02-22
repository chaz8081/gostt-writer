# gostt-writer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a local real-time dictation app that captures speech via global hotkey, transcribes with whisper.cpp, and injects text via keystroke simulation.

**Architecture:** Single Go binary. malgo for audio capture, gohook for global hotkey, whisper.cpp Go bindings for transcription, robotgo for text injection. YAML config file. Async post-capture transcription.

**Tech Stack:** Go 1.25, whisper.cpp (Metal + Accelerate), malgo, robotgo/gohook, gopkg.in/yaml.v3

---

## Dependency Graph

```
gostt-writer-l03: Project scaffolding & build system     [READY]
    |
    +---> gostt-writer-blw: Configuration system (YAML)      [blocked by l03]
    +---> gostt-writer-dc7: Audio recorder (malgo)            [blocked by l03]
    +---> gostt-writer-ult: Whisper transcription wrapper     [blocked by l03]
    +---> gostt-writer-3ic: Global hotkey listener (gohook)   [blocked by l03]
    +---> gostt-writer-dub: Text injection (robotgo)          [blocked by l03]
              |
              +---> gostt-writer-6rg: Main integration        [blocked by all above]
                        |
                        +---> gostt-writer-3ao: Polish & edge cases  [blocked by 6rg]
```

---

### Task 1: Project Scaffolding & Build System (gostt-writer-l03)

**Files:**
- Create: `go.mod`, `Makefile`, `cmd/gostt-writer/main.go`, `config.example.yaml`
- Create: `.gitmodules`

**Step 1:** Initialize Go module
```bash
go mod init github.com/chaz8081/gostt-writer
```

**Step 2:** Add whisper.cpp as git submodule
```bash
git submodule add https://github.com/ggml-org/whisper.cpp.git third_party/whisper.cpp
```

**Step 3:** Create Makefile with targets: `whisper`, `model`, `build`, `run`, `clean`

The Makefile should:
- `whisper`: Build whisper.cpp with cmake, Metal + Accelerate enabled
- `model`: Download ggml-base.en.bin to models/ directory
- `build`: Compile Go binary with CGO flags pointing to whisper.cpp build artifacts
- `run`: Build and run the binary
- `clean`: Remove build artifacts
- `test`: Run all tests

**Step 4:** Build whisper.cpp with Metal + Accelerate
```bash
make whisper
```

**Step 5:** Download ggml-base.en.bin model
```bash
make model
```

**Step 6:** Create minimal `cmd/gostt-writer/main.go` that prints "gostt-writer starting..."

**Step 7:** Create `config.example.yaml` with documented defaults

**Step 8:** Verify build compiles
```bash
make build
```

**Step 9:** Commit

---

### Task 2: Configuration System (gostt-writer-blw)

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

**Step 1:** Write test for `Default()` returning correct default values
```go
func TestDefault(t *testing.T) {
    cfg := Default()
    assert ModelPath contains "ggml-base.en.bin"
    assert Hotkey.Keys == ["ctrl", "shift", "r"]
    assert Hotkey.Mode == "hold"
    assert Audio.SampleRate == 16000
    assert Audio.Channels == 1
    assert Inject.Method == "type"
    assert LogLevel == "info"
}
```

**Step 2:** Run test, verify it fails

**Step 3:** Implement Config struct and Default() function

**Step 4:** Run test, verify it passes

**Step 5:** Write test for Load() from YAML file (create temp YAML, load it, check values)

**Step 6:** Run test, verify it fails

**Step 7:** Implement Load(path) with file reading, YAML parsing, tilde expansion

**Step 8:** Run test, verify it passes

**Step 9:** Write test for validation (invalid mode, invalid inject method, bad sample rate)

**Step 10:** Implement Validate() method

**Step 11:** Run all config tests

**Step 12:** Commit

---

### Task 3: Audio Recorder (gostt-writer-dc7)

**Files:**
- Create: `internal/audio/recorder.go`, `internal/audio/recorder_test.go`
- Create: `cmd/test-audio/main.go` (manual test)

**Step 1:** Add malgo dependency
```bash
go get github.com/gen2brain/malgo
```

**Step 2:** Write unit test for NewRecorder (creates without panic, can Close)

**Step 3:** Implement NewRecorder and Close

**Step 4:** Implement Start() - initializes malgo capture device with callback that appends float32 samples to internal buffer. Key details:
- malgo delivers raw bytes in its callback
- Need to convert from native format (likely float32 on CoreAudio) to []float32
- Sample rate: 16000 Hz, mono, float32
- Use sync.Mutex to protect buffer access from callback goroutine

**Step 5:** Implement Stop() - stops capture device, returns copy of buffer, resets buffer

**Step 6:** Create cmd/test-audio/main.go that:
- Creates recorder at 16kHz mono
- Records for 3 seconds
- Prints buffer length and duration
- Writes raw PCM to stdout for verification

**Step 7:** Test manually: run test-audio, speak into mic, verify non-zero buffer

**Step 8:** Commit

---

### Task 4: Whisper Transcription (gostt-writer-ult)

**Files:**
- Create: `internal/transcribe/transcribe.go`, `internal/transcribe/transcribe_test.go`

**Step 1:** Set up Go module to reference whisper.cpp bindings:
- Add replace directive in go.mod: `github.com/ggerganov/whisper.cpp/bindings/go => ./third_party/whisper.cpp/bindings/go`
- Add require directive

**Step 2:** Write test for NewTranscriber (loads model successfully)
- Uses models/ggml-base.en.bin
- Test that model loads without error
- Test that Close() works

**Step 3:** Implement NewTranscriber using whisper.New(modelPath)

**Step 4:** Write test for Process:
- Load the JFK sample WAV from third_party/whisper.cpp/samples/jfk.wav
- Convert WAV to []float32 (read WAV header, extract PCM data)
- Pass to Process()
- Assert output contains "ask not what your country"

**Step 5:** Implement Process(samples):
- Create whisper context from model
- Set language to "en", set translate to false
- Call context.Process(samples, nil, nil, nil)
- Iterate segments, concatenate text with spaces
- Return trimmed result

**Step 6:** Run tests (need CGO flags: C_INCLUDE_PATH and LIBRARY_PATH)

**Step 7:** Commit

---

### Task 5: Global Hotkey Listener (gostt-writer-3ic)

**Files:**
- Create: `internal/hotkey/hotkey.go`
- Create: `cmd/test-hotkey/main.go` (manual test)

**Step 1:** Add gohook dependency
```bash
go get github.com/robotn/gohook
```

**Step 2:** Implement Listen(keys, mode) for "hold" mode:
- Use hook.Register for KeyDown and KeyUp events
- KeyDown with matching keys -> send EventStart
- KeyUp with matching keys -> send EventStop
- Return channel and cancel function (calls hook.End())

**Step 3:** Implement "toggle" mode:
- Track state (recording/not recording)
- KeyDown with matching keys while not recording -> send EventStart, set recording=true
- KeyDown with matching keys while recording -> send EventStop, set recording=false

**Step 4:** Create cmd/test-hotkey/main.go that:
- Listens for Ctrl+Shift+R
- Prints "START" on press, "STOP" on release
- Ctrl+C to exit

**Step 5:** Test manually (requires Accessibility permission granted)

**Step 6:** Commit

---

### Task 6: Text Injection (gostt-writer-dub)

**Files:**
- Create: `internal/inject/inject.go`
- Create: `cmd/test-inject/main.go` (manual test)

**Step 1:** Add robotgo dependency
```bash
go get github.com/go-vgo/robotgo
```

**Step 2:** Implement NewInjector(method) and Inject(text):
- "type" mode: robotgo.Type(text)
- "paste" mode: robotgo.WriteAll(text) then robotgo.KeyTap("v", "cmd")

**Step 3:** Create cmd/test-inject/main.go that:
- Waits 3 seconds (time to focus a text editor)
- Types "Hello from gostt-writer!"
- Tests both "type" and "paste" modes

**Step 4:** Test manually (requires Accessibility permission)

**Step 5:** Commit

---

### Task 7: Main Application Integration (gostt-writer-6rg)

**Files:**
- Modify: `cmd/gostt-writer/main.go`

**Step 1:** Implement CLI flag parsing (--config path)

**Step 2:** Wire up config loading with defaults and validation

**Step 3:** Initialize all components:
- Load Whisper model (log time taken)
- Create audio recorder
- Create text injector
- Start hotkey listener

**Step 4:** Implement main event loop:
```
for event from hotkey channel:
  EventStart -> recorder.Start(), log "Recording..."
  EventStop -> samples = recorder.Stop()
    if too short: skip
    go func: transcribe -> inject -> log
```

**Step 5:** Add signal handling (SIGINT/SIGTERM) for graceful shutdown

**Step 6:** Add startup banner with config summary

**Step 7:** Add permission error detection with clear instructions

**Step 8:** Manual end-to-end test: build, run, press hotkey, speak, verify text

**Step 9:** Commit

---

### Task 8: Polish & Edge Cases (gostt-writer-3ao)

**Files:**
- Modify: various internal/ files and cmd/gostt-writer/main.go

**Step 1:** Add minimum recording duration check (<0.5s = skip)

**Step 2:** Add maximum recording duration handling (>30s = warn, process first 30s)

**Step 3:** Add first-run config file creation (write default config if file missing)

**Step 4:** Switch to slog for structured logging with configurable level

**Step 5:** Add --version flag

**Step 6:** Clean up test binaries in cmd/test-*/

**Step 7:** Final end-to-end test

**Step 8:** Commit
