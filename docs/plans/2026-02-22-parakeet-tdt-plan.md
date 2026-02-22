# Parakeet TDT CoreML Backend — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Parakeet TDT 0.6B v2 as an optional CoreML-based transcription backend alongside the existing whisper.cpp backend.

**Architecture:** Extract a `Transcriber` interface from the existing concrete whisper type, vendor a minimal CoreML bridge from go-coreml (~500 lines), implement the TDT greedy decode loop and vocabulary loader in pure Go, wire everything through config with `whisper` as the default backend.

**Tech Stack:** Go, CoreML (via vendored ObjC bridge), Parakeet TDT 0.6B v2 CoreML models from FluidInference/HuggingFace.

**Design doc:** `.opencode/plans/2026-02-22-parakeet-tdt-design.md`

---

### Task 1: Extract Transcriber Interface + Rename Files

Refactor the existing transcribe package to use an interface. No behavior changes — pure refactoring.

**Files:**
- Create: `internal/transcribe/transcriber.go`
- Rename: `internal/transcribe/transcribe.go` → `internal/transcribe/whisper.go`
- Rename: `internal/transcribe/transcribe_test.go` → `internal/transcribe/whisper_test.go`
- Modify: `cmd/gostt-writer/main.go`

**Step 1: Create the interface file**

Create `internal/transcribe/transcriber.go`:

```go
// Package transcribe provides speech-to-text backends.
package transcribe

import "github.com/chaz8081/gostt-writer/internal/config"

// Transcriber converts audio samples to text.
type Transcriber interface {
    // Process transcribes mono 16kHz float32 audio samples to text.
    Process(samples []float32) (string, error)
    // Close releases backend resources.
    Close() error
}

// New creates a Transcriber based on the config backend setting.
func New(cfg *config.TranscribeConfig) (Transcriber, error) {
    switch cfg.Backend {
    case "parakeet":
        return NewParakeetTranscriber(cfg.ParakeetModelDir)
    default: // "whisper" or empty
        return NewWhisperTranscriber(cfg.ModelPath)
    }
}
```

**Step 2: Rename transcribe.go → whisper.go**

Rename the file. Change `NewTranscriber` to `NewWhisperTranscriber`. Update the type to `WhisperTranscriber`. Keep it implementing the `Transcriber` interface. Remove the package doc comment (it moved to transcriber.go).

Old: `type Transcriber struct` → New: `type WhisperTranscriber struct`
Old: `func NewTranscriber(` → New: `func NewWhisperTranscriber(`

```go
package transcribe

import (
    "fmt"
    "io"
    "strings"

    whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// WhisperTranscriber wraps a whisper.cpp model for speech-to-text.
type WhisperTranscriber struct {
    model whisper.Model
}

// NewWhisperTranscriber loads a whisper model from the given path.
func NewWhisperTranscriber(modelPath string) (*WhisperTranscriber, error) {
    model, err := whisper.New(modelPath)
    if err != nil {
        return nil, fmt.Errorf("transcribe: load whisper model %q: %w", modelPath, err)
    }
    return &WhisperTranscriber{model: model}, nil
}

// Close releases the whisper model resources.
func (t *WhisperTranscriber) Close() error {
    if t.model != nil {
        return t.model.Close()
    }
    return nil
}

// Process transcribes mono 16kHz float32 audio samples to text.
func (t *WhisperTranscriber) Process(samples []float32) (string, error) {
    ctx, err := t.model.NewContext()
    if err != nil {
        return "", fmt.Errorf("transcribe: create context: %w", err)
    }

    if err := ctx.Process(samples, nil, nil, nil); err != nil {
        return "", fmt.Errorf("transcribe: process: %w", err)
    }

    var segments []string
    for {
        seg, err := ctx.NextSegment()
        if err == io.EOF {
            break
        }
        if err != nil {
            return "", fmt.Errorf("transcribe: next segment: %w", err)
        }
        segments = append(segments, seg.Text)
    }

    return strings.TrimSpace(strings.Join(segments, " ")), nil
}
```

**Step 3: Rename transcribe_test.go → whisper_test.go**

Rename the file. Update all references from `NewTranscriber` to `NewWhisperTranscriber`.

**Step 4: Update config to add TranscribeConfig**

Modify `internal/config/config.go`:

Add a `TranscribeConfig` struct. The top-level `Config` gains a `Transcribe TranscribeConfig` field. For backward compatibility, the `Load` function checks: if `Transcribe.Backend` is empty, default to `"whisper"`. If old top-level `ModelPath` is set and `Transcribe.ModelPath` is empty, copy it over.

```go
// TranscribeConfig holds transcription backend settings.
type TranscribeConfig struct {
    Backend          string `yaml:"backend"`            // "whisper" or "parakeet"
    ModelPath        string `yaml:"model_path"`         // whisper: path to ggml model file
    ParakeetModelDir string `yaml:"parakeet_model_dir"` // parakeet: dir with .mlmodelc files + vocab
}
```

The `Config` struct changes:
- Keep `ModelPath` for backward compat (old YAML files still work)
- Add `Transcribe TranscribeConfig`
- In `Default()`: set `Transcribe.Backend = "whisper"`, `Transcribe.ModelPath = "models/ggml-base.en.bin"`, `Transcribe.ParakeetModelDir = "models/parakeet-tdt-v2"`
- In `Load()`: after unmarshal, if `cfg.ModelPath != ""` and `cfg.Transcribe.ModelPath == ""`, copy `cfg.ModelPath` to `cfg.Transcribe.ModelPath` (backward compat)
- In `Validate()`: validate based on backend — whisper requires ModelPath, parakeet requires ParakeetModelDir
- Expand tilde on both `Transcribe.ModelPath` and `Transcribe.ParakeetModelDir`

**Step 5: Update main.go to use factory**

Change main.go from:
```go
transcriber, err := transcribe.NewTranscriber(cfg.ModelPath)
```
To:
```go
transcriber, err := transcribe.New(&cfg.Transcribe)
```

Update the error message and banner to show the backend name.

**Step 6: Update config_test.go**

Add tests for:
- Default TranscribeConfig values
- Loading old-style config (top-level model_path) maps to Transcribe.ModelPath
- Loading new-style config with transcribe section
- Validation: whisper backend requires model_path, parakeet requires parakeet_model_dir

**Step 7: Run tests**

Run: `make test`
Expected: All existing tests pass. The interface extraction is a pure refactoring — no behavior change.

**Step 8: Update config.example.yaml**

Add the new `transcribe` section with comments explaining both backends.

**Step 9: Commit**

```bash
git add -A
git commit -m "refactor: extract Transcriber interface, add backend config

Prepare for multiple transcription backends by extracting a Transcriber
interface from the concrete whisper type. Add TranscribeConfig with
backend selection (whisper/parakeet). Backward compatible with old
top-level model_path config."
```

---

### Task 2: Vendor CoreML Bridge

Copy the minimal CoreML bridge from go-coreml into `internal/coreml/`. This is a self-contained package with no external Go dependencies — just CGo bindings to Foundation + CoreML frameworks.

**Files:**
- Create: `internal/coreml/bridge.go`
- Create: `internal/coreml/bridge.h`
- Create: `internal/coreml/bridge.m`
- Create: `internal/coreml/bridge_test.go`

**Step 1: Create bridge.h**

Copy from `https://raw.githubusercontent.com/gomlx/go-coreml/main/internal/bridge/bridge.h` verbatim, adding a license header noting the Apache 2.0 origin from gomlx/go-coreml.

**Step 2: Create bridge.m**

Copy from `https://raw.githubusercontent.com/gomlx/go-coreml/main/internal/bridge/bridge.m` verbatim, adding the license header.

**Step 3: Create bridge.go**

Adapt from `https://raw.githubusercontent.com/gomlx/go-coreml/main/internal/bridge/bridge.go`:
- Change package name from `bridge` to `coreml`
- Make types exported: `Model`, `Tensor`, `DType`, `ComputeUnits`
- Add license header noting origin
- Keep the full API surface: `CompileModel`, `LoadModel`, `SetComputeUnits`, tensor operations, `Predict`

**Step 4: Write a basic smoke test**

Create `internal/coreml/bridge_test.go`:
- Test `NewTensor` creates tensors with correct shapes
- Test `NewTensorWithData` round-trips data correctly
- Test `LoadModel` with a nonexistent path returns an error
- Skip any model-dependent tests if no test model is available

```go
func TestNewTensor(t *testing.T) {
    tensor, err := NewTensor([]int64{2, 3}, DTypeFloat32)
    if err != nil {
        t.Fatalf("NewTensor: %v", err)
    }
    defer tensor.Close()

    if tensor.Rank() != 2 {
        t.Errorf("Rank = %d, want 2", tensor.Rank())
    }
    shape := tensor.Shape()
    if shape[0] != 2 || shape[1] != 3 {
        t.Errorf("Shape = %v, want [2, 3]", shape)
    }
}

func TestLoadModelBadPath(t *testing.T) {
    _, err := LoadModel("/nonexistent/model.mlmodelc")
    if err == nil {
        t.Error("LoadModel with bad path should return error")
    }
}
```

**Step 5: Verify it compiles**

Run: `go build ./internal/coreml/`
Expected: Compiles successfully with CGo linking Foundation + CoreML.

**Step 6: Run bridge tests**

Run: `go test -v ./internal/coreml/`
Expected: Tensor tests pass. LoadModel error test passes.

**Step 7: Commit**

```bash
git add internal/coreml/
git commit -m "feat: vendor CoreML bridge from gomlx/go-coreml

Minimal ObjC/CGo bridge for loading .mlmodelc files and running
inference via MLMultiArray tensors. ~500 lines, Apache 2.0 licensed
from gomlx/go-coreml."
```

---

### Task 3: Vocabulary Loader

Implement loading `parakeet_vocab.json` and converting token IDs to text.

**Files:**
- Create: `internal/transcribe/parakeet_vocab.go`
- Create: `internal/transcribe/parakeet_vocab_test.go`

**Step 1: Write failing vocabulary tests**

```go
func TestLoadVocabulary(t *testing.T) {
    // Create a temp vocab file
    vocabJSON := `{"0": "▁the", "1": "▁a", "2": "s", "1024": "<blank>"}`
    tmpDir := t.TempDir()
    vocabPath := filepath.Join(tmpDir, "parakeet_vocab.json")
    os.WriteFile(vocabPath, []byte(vocabJSON), 0644)

    vocab, err := loadVocabulary(vocabPath)
    if err != nil {
        t.Fatalf("loadVocabulary: %v", err)
    }
    if vocab[0] != "▁the" {
        t.Errorf("vocab[0] = %q, want %q", vocab[0], "▁the")
    }
    if vocab[1024] != "<blank>" {
        t.Errorf("vocab[1024] = %q, want %q", vocab[1024], "<blank>")
    }
}

func TestDecodeTokens(t *testing.T) {
    vocab := []string{"▁the", "▁a", "s", "k"}
    // tokens: "▁the" + "▁a" + "s" + "k" = " the ask"
    tokens := []int32{0, 1, 2, 3}
    text := decodeTokens(tokens, vocab)
    if text != "the ask" {
        t.Errorf("decodeTokens = %q, want %q", text, "the ask")
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `make test` (or `go test -v ./internal/transcribe/ -run TestLoadVocab -run TestDecodeTokens`)
Expected: FAIL — functions not defined.

**Step 3: Implement vocabulary loader**

```go
package transcribe

import (
    "encoding/json"
    "fmt"
    "os"
    "strconv"
    "strings"
)

// loadVocabulary reads parakeet_vocab.json and returns a token ID → string mapping.
// The JSON format is {"0": "▁the", "1": "▁a", ...} where keys are string token IDs.
func loadVocabulary(path string) ([]string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("reading vocabulary: %w", err)
    }

    var raw map[string]string
    if err := json.Unmarshal(data, &raw); err != nil {
        return nil, fmt.Errorf("parsing vocabulary JSON: %w", err)
    }

    // Find max ID to size the slice
    maxID := 0
    for k := range raw {
        id, err := strconv.Atoi(k)
        if err != nil {
            return nil, fmt.Errorf("invalid token ID %q: %w", k, err)
        }
        if id > maxID {
            maxID = id
        }
    }

    vocab := make([]string, maxID+1)
    for k, v := range raw {
        id, _ := strconv.Atoi(k)
        vocab[id] = v
    }

    return vocab, nil
}

// decodeTokens converts a sequence of token IDs to text using the vocabulary.
// SentencePiece "▁" markers are replaced with spaces, then the result is trimmed.
func decodeTokens(tokens []int32, vocab []string) string {
    var b strings.Builder
    for _, id := range tokens {
        if int(id) < len(vocab) {
            b.WriteString(vocab[id])
        }
    }
    text := b.String()
    text = strings.ReplaceAll(text, "▁", " ")
    return strings.TrimSpace(text)
}
```

**Step 4: Run tests to verify they pass**

Run: `make test`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/transcribe/parakeet_vocab.go internal/transcribe/parakeet_vocab_test.go
git commit -m "feat: add Parakeet vocabulary loader and token decoder"
```

---

### Task 4: TDT Greedy Decode Loop

Implement the core TDT greedy decode algorithm in pure Go. This is the heart of Parakeet inference — it orchestrates the Decoder and JointDecision models.

**Files:**
- Create: `internal/transcribe/parakeet_decode.go`
- Create: `internal/transcribe/parakeet_decode_test.go`

**Step 1: Define the decode interface for model calls**

The decode loop needs to call two models (Decoder and JointDecision). Define interfaces so the decode loop can be unit tested with mocks:

```go
// decoderRunner runs the LSTM decoder for one step.
type decoderRunner interface {
    // runDecoder runs one decoder step. Returns decoder output and updated LSTM state.
    runDecoder(targetID int32, hIn, cIn []float32) (decoderOut, hOut, cOut []float32, err error)
}

// jointRunner runs the joint decision network for one step.
type jointRunner interface {
    // runJoint runs the joint network with one encoder frame and decoder output.
    // Returns token ID, duration (frames to advance), and error.
    runJoint(encoderStep, decoderStep []float32) (tokenID, duration int32, err error)
}
```

**Step 2: Write failing decode loop tests with mocks**

Create test mocks that return predetermined sequences, then verify the decode loop produces correct token sequences:

```go
func TestTDTDecodeBasic(t *testing.T) {
    // Mock: encoder has 3 frames, each emits one token then blank
    // Frame 0: token 5 (duration 1), then blank (duration 1)
    // Frame 1: token 10 (duration 1), then blank (duration 1)
    // Frame 2: blank (duration 1)
    // Expected tokens: [5, 10]
    ...
}

func TestTDTDecodeBlankSkip(t *testing.T) {
    // Test that blank tokens with duration > 1 skip multiple frames
    ...
}

func TestTDTDecodeMaxSymbolsGuard(t *testing.T) {
    // Test that max 10 symbols per timestep is enforced
    ...
}
```

**Step 3: Run tests to verify they fail**

Expected: FAIL — tdtDecode not defined.

**Step 4: Implement the TDT decode loop**

```go
package transcribe

const (
    parakeetBlankID         = 1024
    parakeetMaxSymsPerStep  = 10
    parakeetEncoderHidden   = 1024
    parakeetDecoderHidden   = 640
    parakeetLSTMLayers      = 2
)

var parakeetDurationBins = []int32{0, 1, 2, 3, 4}

// tdtDecode runs the TDT greedy decode algorithm over encoder output frames.
// encoderOutput shape: [T, encoderHidden] (flattened from [1, T, encoderHidden])
// encoderLength: number of valid frames in encoderOutput
// Returns the decoded token IDs (excluding blank tokens).
func tdtDecode(
    encoderOutput []float32,
    encoderLength int,
    dec decoderRunner,
    joint jointRunner,
) ([]int32, error) {
    // Initialize LSTM state (zeros)
    lstmStateSize := parakeetLSTMLayers * 1 * parakeetDecoderHidden // [2, 1, 640]
    hState := make([]float32, lstmStateSize)
    cState := make([]float32, lstmStateSize)

    // Initial decoder run with blank token
    decoderOut, hState, cState, err := dec.runDecoder(parakeetBlankID, hState, cState)
    if err != nil {
        return nil, fmt.Errorf("initial decoder run: %w", err)
    }

    var tokens []int32
    t := 0 // current encoder frame index

    for t < encoderLength {
        // Extract encoder frame at position t: encoderOutput[t*1024 : (t+1)*1024]
        frameStart := t * parakeetEncoderHidden
        encoderFrame := encoderOutput[frameStart : frameStart+parakeetEncoderHidden]

        symCount := 0
        for symCount < parakeetMaxSymsPerStep {
            tokenID, durIdx, err := joint.runJoint(encoderFrame, decoderOut)
            if err != nil {
                return nil, fmt.Errorf("joint at frame %d: %w", t, err)
            }

            dur := parakeetDurationBins[durIdx]

            if tokenID == parakeetBlankID {
                // Blank: advance without updating decoder
                if dur == 0 {
                    dur = 1 // prevent infinite loop
                }
                t += int(dur)
                break
            }

            // Non-blank: emit token, update decoder state
            tokens = append(tokens, tokenID)
            decoderOut, hState, cState, err = dec.runDecoder(tokenID, hState, cState)
            if err != nil {
                return nil, fmt.Errorf("decoder at frame %d: %w", t, err)
            }

            if dur > 0 {
                t += int(dur)
                break // advance to next frame
            }

            symCount++
        }

        // If we hit max symbols without advancing, force advance by 1
        if symCount >= parakeetMaxSymsPerStep {
            t++
        }
    }

    return tokens, nil
}
```

**Step 5: Run tests to verify they pass**

Run: `make test`
Expected: PASS — decode logic works with mocks.

**Step 6: Commit**

```bash
git add internal/transcribe/parakeet_decode.go internal/transcribe/parakeet_decode_test.go
git commit -m "feat: implement TDT greedy decode loop

Pure Go port of FluidAudio's TdtDecoderV3. Handles blank/non-blank
tokens, duration bins, LSTM state management, and max-symbols-per-step
guard. Fully unit tested with mock model runners."
```

---

### Task 5: Parakeet Transcriber (Wiring It All Together)

Connect the CoreML bridge, vocabulary, and decode loop into the `ParakeetTranscriber` struct.

**Files:**
- Create: `internal/transcribe/parakeet.go`
- Create: `internal/transcribe/parakeet_test.go`

**Step 1: Implement ParakeetTranscriber**

```go
package transcribe

import (
    "fmt"
    "path/filepath"
    "unsafe"

    "github.com/chaz8081/gostt-writer/internal/coreml"
)

const parakeetMaxSamples = 240000 // 15s at 16kHz

// ParakeetTranscriber uses Parakeet TDT 0.6B v2 via CoreML for speech-to-text.
type ParakeetTranscriber struct {
    preprocessor *coreml.Model
    encoder      *coreml.Model
    decoder      *coreml.Model
    joint        *coreml.Model
    vocab        []string
}

// NewParakeetTranscriber loads the 4 CoreML models and vocabulary from modelDir.
func NewParakeetTranscriber(modelDir string) (*ParakeetTranscriber, error) {
    // Load vocabulary
    vocabPath := filepath.Join(modelDir, "parakeet_vocab.json")
    vocab, err := loadVocabulary(vocabPath)
    if err != nil {
        return nil, fmt.Errorf("parakeet: %w", err)
    }

    // Load CoreML models
    // Preprocessor runs on CPU (mel spectrogram is faster on CPU)
    coreml.SetComputeUnits(coreml.ComputeCPUOnly)
    preprocessor, err := coreml.LoadModel(filepath.Join(modelDir, "Preprocessor.mlmodelc"))
    if err != nil {
        return nil, fmt.Errorf("parakeet: load preprocessor: %w", err)
    }

    // Encoder, decoder, joint run on all units (ANE preferred)
    coreml.SetComputeUnits(coreml.ComputeAll)
    encoder, err := coreml.LoadModel(filepath.Join(modelDir, "Encoder.mlmodelc"))
    if err != nil {
        preprocessor.Close()
        return nil, fmt.Errorf("parakeet: load encoder: %w", err)
    }

    decoder, err := coreml.LoadModel(filepath.Join(modelDir, "Decoder.mlmodelc"))
    if err != nil {
        preprocessor.Close()
        encoder.Close()
        return nil, fmt.Errorf("parakeet: load decoder: %w", err)
    }

    joint, err := coreml.LoadModel(filepath.Join(modelDir, "JointDecision.mlmodelc"))
    if err != nil {
        preprocessor.Close()
        encoder.Close()
        decoder.Close()
        return nil, fmt.Errorf("parakeet: load joint: %w", err)
    }

    return &ParakeetTranscriber{
        preprocessor: preprocessor,
        encoder:      encoder,
        decoder:      decoder,
        joint:        joint,
        vocab:        vocab,
    }, nil
}

// Close releases all CoreML model resources.
func (p *ParakeetTranscriber) Close() error {
    if p.preprocessor != nil { p.preprocessor.Close() }
    if p.encoder != nil { p.encoder.Close() }
    if p.decoder != nil { p.decoder.Close() }
    if p.joint != nil { p.joint.Close() }
    return nil
}

// Process transcribes mono 16kHz float32 audio samples to text.
func (p *ParakeetTranscriber) Process(samples []float32) (string, error) {
    // Pad or truncate to maxModelSamples
    padded := padAudio(samples, parakeetMaxSamples)

    // Step 1: Preprocessor (mel spectrogram)
    // ... (CoreML model calls using p.preprocessor)

    // Step 2: Encoder
    // ... (CoreML model call using p.encoder)

    // Step 3+4: TDT decode loop (decoder + joint)
    // ... (calls tdtDecode with p as the decoderRunner and jointRunner)

    // Step 5: Convert tokens to text
    // text := decodeTokens(tokens, p.vocab)

    return text, nil
}

// padAudio pads or truncates audio to exactly maxSamples.
func padAudio(samples []float32, maxSamples int) []float32 {
    if len(samples) >= maxSamples {
        return samples[:maxSamples]
    }
    padded := make([]float32, maxSamples)
    copy(padded, samples)
    return padded
}
```

The `Process` method will contain the actual CoreML tensor creation and `Predict` calls for each model. The exact input/output tensor names and shapes need to be discovered by introspecting the loaded models (using `InputName`/`OutputName`/`InputCount`/`OutputCount`), and may need adjustment based on what the real models expect.

**Note:** The exact tensor I/O wiring (Step 1-4 inside Process) requires running against the real models to verify input/output names and shapes. The implementation should:
1. First introspect each model to log its input/output names and shapes
2. Wire up the tensor creation and predict calls based on what we find
3. Handle any float16 vs float32 conversions if needed

The `ParakeetTranscriber` also implements `decoderRunner` and `jointRunner` interfaces by wrapping the CoreML model calls:

```go
func (p *ParakeetTranscriber) runDecoder(targetID int32, hIn, cIn []float32) ([]float32, []float32, []float32, error) {
    // Create input tensors, call p.decoder.Predict(), extract outputs
    ...
}

func (p *ParakeetTranscriber) runJoint(encoderStep, decoderStep []float32) (int32, int32, error) {
    // Create input tensors, call p.joint.Predict(), extract outputs
    ...
}
```

**Step 2: Write integration test (requires real models)**

```go
func parakeetModelDir(t *testing.T) string {
    t.Helper()
    dir := filepath.Join("..", "..", "models", "parakeet-tdt-v2")
    if _, err := os.Stat(filepath.Join(dir, "Encoder.mlmodelc")); err != nil {
        t.Skipf("Parakeet models not found at %s (run 'make parakeet-model' first)", dir)
    }
    return dir
}

func TestNewParakeetTranscriber(t *testing.T) {
    dir := parakeetModelDir(t)
    tr, err := NewParakeetTranscriber(dir)
    if err != nil {
        t.Fatalf("NewParakeetTranscriber: %v", err)
    }
    defer tr.Close()
}

func TestParakeetProcessJFK(t *testing.T) {
    dir := parakeetModelDir(t)
    samples := jfkSamples(t) // reuse from whisper_test.go

    tr, err := NewParakeetTranscriber(dir)
    if err != nil {
        t.Fatalf("NewParakeetTranscriber: %v", err)
    }
    defer tr.Close()

    text, err := tr.Process(samples)
    if err != nil {
        t.Fatalf("Process: %v", err)
    }

    lower := strings.ToLower(text)
    if !strings.Contains(lower, "ask not what your country") {
        t.Errorf("expected transcript to contain 'ask not what your country', got: %q", text)
    }
}
```

**Step 3: Add model introspection helper**

Add a debug function that loads each model and prints its input/output names and expected shapes. This will be critical during initial development to verify our tensor shapes match:

```go
// introspectModel logs the input/output names of a CoreML model.
func introspectModel(name string, m *coreml.Model) {
    slog.Debug("CoreML model introspection",
        "name", name,
        "inputs", m.InputCount(),
        "outputs", m.OutputCount())
    for i := 0; i < m.InputCount(); i++ {
        slog.Debug("  input", "index", i, "name", m.InputName(i))
    }
    for i := 0; i < m.OutputCount(); i++ {
        slog.Debug("  output", "index", i, "name", m.OutputName(i))
    }
}
```

**Step 4: Iteratively implement and test Process**

This step will likely require some back-and-forth as we discover the exact tensor shapes from the real models. The general approach:

1. Load models, introspect to find exact input/output names
2. Implement preprocessor call (audio → mel features)
3. Implement encoder call (mel features → encoder hidden states)
4. Implement decoder + joint calls via the existing decode loop
5. Run the JFK test to validate end-to-end

**Step 5: Run tests**

Run: `make test`
Expected: All existing whisper tests pass. New parakeet tests pass if models are present, skip if not.

**Step 6: Commit**

```bash
git add internal/transcribe/parakeet.go internal/transcribe/parakeet_test.go
git commit -m "feat: add Parakeet TDT CoreML transcription backend

Loads 4 CoreML models (Preprocessor, Encoder, Decoder, JointDecision),
runs the TDT greedy decode pipeline, and converts tokens to text via
the SentencePiece vocabulary. Tested with JFK sample audio."
```

---

### Task 6: Makefile + Model Download

Add model download target and update build/test targets.

**Files:**
- Modify: `Makefile`

**Step 1: Add parakeet-model target**

```makefile
# Parakeet TDT model
PARAKEET_DIR := $(MODELS_DIR)/parakeet-tdt-v2
PARAKEET_REPO := FluidInference/parakeet-tdt-0.6b-v2-coreml
PARAKEET_HF_BASE := https://huggingface.co/$(PARAKEET_REPO)/resolve/main

.PHONY: parakeet-model

parakeet-model:
	@echo "Downloading Parakeet TDT v2 CoreML models..."
	@mkdir -p $(PARAKEET_DIR)
	@# Download vocab
	curl -L -o $(PARAKEET_DIR)/parakeet_vocab.json $(PARAKEET_HF_BASE)/parakeet_vocab.json
	@# Download model bundles (each is a directory)
	@for model in Preprocessor Encoder Decoder JointDecision; do \
		echo "Downloading $$model.mlmodelc..."; \
		# HuggingFace model bundles need special handling — may need huggingface-cli or git lfs
	done
	@echo "Parakeet models downloaded to $(PARAKEET_DIR)"
```

**Note:** The `.mlmodelc` bundles on HuggingFace may require `git lfs` or `huggingface-cli download` rather than plain curl, since they're directory trees. The exact download mechanism needs to be determined:
- Option 1: `huggingface-cli download FluidInference/parakeet-tdt-0.6b-v2-coreml --local-dir $(PARAKEET_DIR)` (requires huggingface-cli installed)
- Option 2: `git clone` with LFS from the HuggingFace repo
- Option 3: Direct curl if HuggingFace provides tar/zip downloads

The simplest approach is likely `huggingface-cli download` with an `--include` filter.

**Step 2: Update build target to include CoreML frameworks**

The existing build target may need `-framework CoreML` added to the linker flags since we're now always compiling the CoreML bridge.

**Step 3: Update help target**

Add `parakeet-model` to the help output.

**Step 4: Test the download**

Run: `make parakeet-model`
Expected: Models download to `models/parakeet-tdt-v2/`

**Step 5: Commit**

```bash
git add Makefile
git commit -m "feat: add parakeet-model download target"
```

---

### Task 7: Update config.example.yaml, README, and Polish

**Files:**
- Modify: `config.example.yaml`
- Modify: `README.md`
- Modify: `cmd/gostt-writer/main.go` (banner update)

**Step 1: Update config.example.yaml**

Add the `transcribe` section with documentation for both backends.

**Step 2: Update README.md**

Add a section on using the Parakeet backend:
- How to download models (`make parakeet-model`)
- How to switch backends in config
- Performance comparison notes

**Step 3: Update main.go banner**

Show the active backend in the startup banner:
```
=== gostt-writer ===
  Version: v1.1.0
  Backend: parakeet (CoreML/ANE)
  Model:   models/parakeet-tdt-v2/
  ...
```

**Step 4: Run full test suite**

Run: `make test`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add -A
git commit -m "docs: update config example and README for Parakeet backend"
```

---

### Task 8: End-to-End Validation

Final integration testing — verify the full pipeline works with both backends.

**Step 1: Test whisper backend (default)**

```bash
make build && ./bin/gostt-writer
# Press Ctrl+Shift+R, speak, verify transcription
```

**Step 2: Test parakeet backend**

```bash
# Update config: transcribe.backend: parakeet
make build && ./bin/gostt-writer
# Press Ctrl+Shift+R, speak, verify transcription
```

**Step 3: Verify backward compatibility**

Test with an old-style config (top-level `model_path`, no `transcribe` section) — should default to whisper.

**Step 4: Run full test suite one final time**

Run: `make test`
Expected: All tests pass.

**Step 5: Final commit if any fixes needed**

---

## Summary

| Task | Description | Estimated Effort |
|------|-------------|-----------------|
| 1 | Extract Transcriber interface + config refactor | ~30 min |
| 2 | Vendor CoreML bridge | ~20 min |
| 3 | Vocabulary loader | ~15 min |
| 4 | TDT greedy decode loop | ~45 min |
| 5 | Parakeet transcriber (wire everything) | ~60 min |
| 6 | Makefile + model download | ~20 min |
| 7 | Config example, README, polish | ~15 min |
| 8 | End-to-end validation | ~15 min |
| **Total** | | **~3.5 hours** |

**Critical path:** Task 5 is the riskiest — it requires matching the exact CoreML model I/O tensor shapes. The model introspection helper will be essential. Tasks 1-4 are low-risk since they're either pure refactoring or pure Go logic.
