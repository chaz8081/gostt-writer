package transcribe

import (
	"fmt"
	"log/slog"
	"math"
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

	// Cached I/O names discovered via model introspection.
	// Preprocessor
	prepInputNames  []string
	prepOutputNames []string
	// Encoder
	encInputNames  []string
	encOutputNames []string
	// Decoder
	decInputNames  []string
	decOutputNames []string
	// Joint
	jointInputNames  []string
	jointOutputNames []string
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

	p := &ParakeetTranscriber{
		preprocessor: preprocessor,
		encoder:      encoder,
		decoder:      decoder,
		joint:        joint,
		vocab:        vocab,
	}

	// Cache I/O names from model introspection
	p.prepInputNames = modelInputNames(preprocessor)
	p.prepOutputNames = modelOutputNames(preprocessor)
	p.encInputNames = modelInputNames(encoder)
	p.encOutputNames = modelOutputNames(encoder)
	p.decInputNames = modelInputNames(decoder)
	p.decOutputNames = modelOutputNames(decoder)
	p.jointInputNames = modelInputNames(joint)
	p.jointOutputNames = modelOutputNames(joint)

	// Log model I/O for debugging
	introspectModel("Preprocessor", preprocessor)
	introspectModel("Encoder", encoder)
	introspectModel("Decoder", decoder)
	introspectModel("JointDecision", joint)

	return p, nil
}

// Close releases all CoreML model resources.
func (p *ParakeetTranscriber) Close() error {
	if p.preprocessor != nil {
		p.preprocessor.Close()
	}
	if p.encoder != nil {
		p.encoder.Close()
	}
	if p.decoder != nil {
		p.decoder.Close()
	}
	if p.joint != nil {
		p.joint.Close()
	}
	return nil
}

// Process transcribes mono 16kHz float32 audio samples to text.
func (p *ParakeetTranscriber) Process(samples []float32) (string, error) {
	// Pad or truncate to maxModelSamples
	padded := padAudio(samples, parakeetMaxSamples)

	// Step 1: Preprocessor (audio → mel features)
	prepOutputs, err := p.runPreprocessor(padded)
	if err != nil {
		return "", fmt.Errorf("parakeet: preprocessor: %w", err)
	}
	defer func() {
		for _, t := range prepOutputs {
			t.Close()
		}
	}()

	// Step 2: Encoder (mel features → encoder hidden states)
	encOutputs, err := p.runEncoder(prepOutputs)
	if err != nil {
		return "", fmt.Errorf("parakeet: encoder: %w", err)
	}
	defer func() {
		for _, t := range encOutputs {
			t.Close()
		}
	}()

	// Extract encoder output and length
	encoderOutput, encoderLength, err := p.extractEncoderOutput(encOutputs)
	if err != nil {
		return "", fmt.Errorf("parakeet: %w", err)
	}

	slog.Debug("parakeet encoder", "frames", encoderLength, "totalFloats", len(encoderOutput))

	// Step 3+4: TDT decode loop (decoder + joint)
	tokens, err := tdtDecode(encoderOutput, encoderLength, p, p)
	if err != nil {
		return "", fmt.Errorf("parakeet: decode: %w", err)
	}

	// Step 5: Convert tokens to text
	text := decodeTokens(tokens, p.vocab)
	return text, nil
}

// runPreprocessor runs the preprocessor model on raw audio.
// Returns output tensors (caller must close them).
func (p *ParakeetTranscriber) runPreprocessor(audio []float32) ([]*coreml.Tensor, error) {
	// Create audio_signal tensor [1, N]
	audioTensor, err := coreml.NewTensorWithData(
		[]int64{1, int64(len(audio))},
		coreml.DTypeFloat32,
		unsafe.Pointer(&audio[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("create audio tensor: %w", err)
	}
	defer audioTensor.Close()

	// Create audio_length tensor [1] with value N
	audioLen := []int32{int32(len(audio))}
	audioLenTensor, err := coreml.NewTensorWithData(
		[]int64{1},
		coreml.DTypeInt32,
		unsafe.Pointer(&audioLen[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("create audio_length tensor: %w", err)
	}
	defer audioLenTensor.Close()

	// Allocate output tensors — we need to introspect the model to know exact shapes.
	// The preprocessor outputs mel features. We allocate generously and let CoreML fill them.
	// For Parakeet TDT, the preprocessor output shape depends on audio length.
	// With 240000 samples (15s), we expect ~1500 mel frames × 80 features = [1, 80, ~1500].
	// However, the exact shape is model-specific. We'll pre-allocate and the bridge will
	// overwrite with actual output data.
	numOutputs := p.preprocessor.OutputCount()
	outputs := make([]*coreml.Tensor, numOutputs)
	for i := range outputs {
		// Allocate a placeholder that will be populated by Predict.
		// The CoreML bridge copies output data into these tensors.
		// We need a reasonable allocation — use a large mel spectrogram buffer.
		outputs[i], err = coreml.NewTensor([]int64{1, 80, 1500}, coreml.DTypeFloat32)
		if err != nil {
			for j := 0; j < i; j++ {
				outputs[j].Close()
			}
			return nil, fmt.Errorf("create output tensor %d: %w", i, err)
		}
	}

	err = p.preprocessor.Predict(
		p.prepInputNames,
		[]*coreml.Tensor{audioTensor, audioLenTensor},
		p.prepOutputNames,
		outputs,
	)
	if err != nil {
		for _, t := range outputs {
			t.Close()
		}
		return nil, fmt.Errorf("predict: %w", err)
	}

	return outputs, nil
}

// runEncoder runs the encoder model on preprocessor outputs.
// Returns output tensors (caller must close them).
func (p *ParakeetTranscriber) runEncoder(prepOutputs []*coreml.Tensor) ([]*coreml.Tensor, error) {
	// The encoder takes the preprocessor outputs as inputs.
	// We pass them through using the encoder's input names.
	numOutputs := p.encoder.OutputCount()
	outputs := make([]*coreml.Tensor, numOutputs)
	var err error
	for i := range outputs {
		// Encoder output: [1, T, 1024] for encoder hidden states, plus encoder_length.
		// Allocate generously for the encoder output.
		if i == 0 {
			// Main encoder output: [1, T, 1024] — T ≈ 1500 for 15s audio
			outputs[i], err = coreml.NewTensor([]int64{1, 1500, int64(parakeetEncoderHidden)}, coreml.DTypeFloat32)
		} else {
			// encoder_length: scalar or [1]
			outputs[i], err = coreml.NewTensor([]int64{1}, coreml.DTypeInt32)
		}
		if err != nil {
			for j := 0; j < i; j++ {
				outputs[j].Close()
			}
			return nil, fmt.Errorf("create encoder output tensor %d: %w", i, err)
		}
	}

	err = p.encoder.Predict(
		p.encInputNames,
		prepOutputs,
		p.encOutputNames,
		outputs,
	)
	if err != nil {
		for _, t := range outputs {
			t.Close()
		}
		return nil, fmt.Errorf("predict: %w", err)
	}

	return outputs, nil
}

// extractEncoderOutput extracts the flattened encoder hidden states and length from encoder outputs.
func (p *ParakeetTranscriber) extractEncoderOutput(encOutputs []*coreml.Tensor) ([]float32, int, error) {
	if len(encOutputs) < 2 {
		return nil, 0, fmt.Errorf("encoder returned %d outputs, need at least 2", len(encOutputs))
	}

	// Find the encoder output tensor (3D: [1, T, 1024]) and length tensor (1D or scalar)
	var encoderTensor, lengthTensor *coreml.Tensor
	for _, t := range encOutputs {
		if t.Rank() == 3 {
			encoderTensor = t
		} else if t.Rank() <= 1 {
			lengthTensor = t
		}
	}

	if encoderTensor == nil {
		return nil, 0, fmt.Errorf("no 3D encoder output tensor found")
	}

	// Extract encoder length
	var encoderLength int
	if lengthTensor != nil && lengthTensor.DType() == coreml.DTypeInt32 {
		data := (*int32)(lengthTensor.DataPtr())
		encoderLength = int(*data)
	} else {
		// Fall back to tensor shape
		encoderLength = int(encoderTensor.Dim(1))
	}

	// Copy encoder output to Go slice
	totalFloats := int(encoderTensor.Dim(1)) * int(encoderTensor.Dim(2))
	encoderData := make([]float32, totalFloats)

	if encoderTensor.DType() == coreml.DTypeFloat16 {
		// Convert float16 to float32
		src := unsafe.Slice((*uint16)(encoderTensor.DataPtr()), totalFloats)
		for i, v := range src {
			encoderData[i] = float16ToFloat32(v)
		}
	} else {
		// Direct copy for float32
		src := unsafe.Slice((*float32)(encoderTensor.DataPtr()), totalFloats)
		copy(encoderData, src)
	}

	return encoderData, encoderLength, nil
}

// Ensure ParakeetTranscriber implements decoderRunner and jointRunner.
var _ decoderRunner = (*ParakeetTranscriber)(nil)
var _ jointRunner = (*ParakeetTranscriber)(nil)

// runDecoder runs the LSTM decoder for one step via CoreML.
func (p *ParakeetTranscriber) runDecoder(targetID int32, hIn, cIn []float32) (decoderOut, hOut, cOut []float32, err error) {
	// Create targets tensor [1, 1]
	targets := []int32{targetID}
	targetsTensor, err := coreml.NewTensorWithData(
		[]int64{1, 1},
		coreml.DTypeInt32,
		unsafe.Pointer(&targets[0]),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create targets tensor: %w", err)
	}
	defer targetsTensor.Close()

	// Create h_in tensor [2, 1, 640]
	hInTensor, err := coreml.NewTensorWithData(
		[]int64{int64(parakeetLSTMLayers), 1, int64(parakeetDecoderHidden)},
		coreml.DTypeFloat32,
		unsafe.Pointer(&hIn[0]),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create h_in tensor: %w", err)
	}
	defer hInTensor.Close()

	// Create c_in tensor [2, 1, 640]
	cInTensor, err := coreml.NewTensorWithData(
		[]int64{int64(parakeetLSTMLayers), 1, int64(parakeetDecoderHidden)},
		coreml.DTypeFloat32,
		unsafe.Pointer(&cIn[0]),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create c_in tensor: %w", err)
	}
	defer cInTensor.Close()

	// Allocate output tensors
	// decoder output: [1, 640] or [1, 1, 640]
	decOutTensor, err := coreml.NewTensor([]int64{1, int64(parakeetDecoderHidden)}, coreml.DTypeFloat32)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create decoder output tensor: %w", err)
	}
	defer decOutTensor.Close()

	// h_out: [2, 1, 640]
	hOutTensor, err := coreml.NewTensor([]int64{int64(parakeetLSTMLayers), 1, int64(parakeetDecoderHidden)}, coreml.DTypeFloat32)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create h_out tensor: %w", err)
	}
	defer hOutTensor.Close()

	// c_out: [2, 1, 640]
	cOutTensor, err := coreml.NewTensor([]int64{int64(parakeetLSTMLayers), 1, int64(parakeetDecoderHidden)}, coreml.DTypeFloat32)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create c_out tensor: %w", err)
	}
	defer cOutTensor.Close()

	err = p.decoder.Predict(
		p.decInputNames,
		[]*coreml.Tensor{targetsTensor, hInTensor, cInTensor},
		p.decOutputNames,
		[]*coreml.Tensor{decOutTensor, hOutTensor, cOutTensor},
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("predict: %w", err)
	}

	// Copy outputs to Go slices
	decoderOut = copyFloat32FromTensor(decOutTensor, parakeetDecoderHidden)
	lstmStateSize := parakeetLSTMLayers * 1 * parakeetDecoderHidden
	hOut = copyFloat32FromTensor(hOutTensor, lstmStateSize)
	cOut = copyFloat32FromTensor(cOutTensor, lstmStateSize)

	return decoderOut, hOut, cOut, nil
}

// runJoint runs the joint decision network for one step via CoreML.
func (p *ParakeetTranscriber) runJoint(encoderStep, decoderStep []float32) (tokenID, duration int32, err error) {
	// Create encoder_step tensor [1, 1024, 1]
	encStep := make([]float32, parakeetEncoderHidden)
	copy(encStep, encoderStep)
	encStepTensor, err := coreml.NewTensorWithData(
		[]int64{1, int64(parakeetEncoderHidden), 1},
		coreml.DTypeFloat32,
		unsafe.Pointer(&encStep[0]),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("create encoder_step tensor: %w", err)
	}
	defer encStepTensor.Close()

	// Create decoder_step tensor [1, 640, 1]
	decStep := make([]float32, parakeetDecoderHidden)
	copy(decStep, decoderStep)
	decStepTensor, err := coreml.NewTensorWithData(
		[]int64{1, int64(parakeetDecoderHidden), 1},
		coreml.DTypeFloat32,
		unsafe.Pointer(&decStep[0]),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("create decoder_step tensor: %w", err)
	}
	defer decStepTensor.Close()

	// Allocate output tensors
	// token_id: [1, 1, 1] int32
	tokenOutTensor, err := coreml.NewTensor([]int64{1, 1, 1}, coreml.DTypeInt32)
	if err != nil {
		return 0, 0, fmt.Errorf("create token_id output tensor: %w", err)
	}
	defer tokenOutTensor.Close()

	// duration: [1, 1, 1] int32
	durOutTensor, err := coreml.NewTensor([]int64{1, 1, 1}, coreml.DTypeInt32)
	if err != nil {
		return 0, 0, fmt.Errorf("create duration output tensor: %w", err)
	}
	defer durOutTensor.Close()

	err = p.joint.Predict(
		p.jointInputNames,
		[]*coreml.Tensor{encStepTensor, decStepTensor},
		p.jointOutputNames,
		[]*coreml.Tensor{tokenOutTensor, durOutTensor},
	)
	if err != nil {
		return 0, 0, fmt.Errorf("predict: %w", err)
	}

	// Extract token_id and duration
	tokenPtr := (*int32)(tokenOutTensor.DataPtr())
	tokenID = *tokenPtr

	durPtr := (*int32)(durOutTensor.DataPtr())
	duration = *durPtr

	// Clamp duration to valid range
	if duration < 0 {
		duration = 0
	}
	if int(duration) >= len(parakeetDurationBins) {
		duration = int32(len(parakeetDurationBins) - 1)
	}

	return tokenID, duration, nil
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

// copyFloat32FromTensor copies n float32 values from a tensor's data pointer.
// Handles float16 → float32 conversion if needed.
func copyFloat32FromTensor(t *coreml.Tensor, n int) []float32 {
	result := make([]float32, n)
	if t.DType() == coreml.DTypeFloat16 {
		src := unsafe.Slice((*uint16)(t.DataPtr()), n)
		for i, v := range src {
			result[i] = float16ToFloat32(v)
		}
	} else {
		src := unsafe.Slice((*float32)(t.DataPtr()), n)
		copy(result, src)
	}
	return result
}

// float16ToFloat32 converts a IEEE 754 half-precision float to float32.
func float16ToFloat32(h uint16) float32 {
	// Extract components
	sign := uint32(h>>15) & 1
	exp := uint32(h>>10) & 0x1f
	frac := uint32(h) & 0x3ff

	var f uint32
	switch {
	case exp == 0:
		if frac == 0 {
			// Zero
			f = sign << 31
		} else {
			// Subnormal: normalize
			exp = 1
			for frac&0x400 == 0 {
				frac <<= 1
				exp--
			}
			frac &= 0x3ff
			f = (sign << 31) | ((exp + 127 - 15) << 23) | (frac << 13)
		}
	case exp == 0x1f:
		// Inf/NaN
		f = (sign << 31) | (0xff << 23) | (frac << 13)
	default:
		// Normal
		f = (sign << 31) | ((exp + 127 - 15) << 23) | (frac << 13)
	}

	return math.Float32frombits(f)
}

// modelInputNames returns all input names for a model.
func modelInputNames(m *coreml.Model) []string {
	names := make([]string, m.InputCount())
	for i := range names {
		names[i] = m.InputName(i)
	}
	return names
}

// modelOutputNames returns all output names for a model.
func modelOutputNames(m *coreml.Model) []string {
	names := make([]string, m.OutputCount())
	for i := range names {
		names[i] = m.OutputName(i)
	}
	return names
}

// introspectModel logs the input/output names of a CoreML model.
func introspectModel(name string, m *coreml.Model) {
	slog.Debug("CoreML model introspection",
		"name", name,
		"inputs", m.InputCount(),
		"outputs", m.OutputCount())
	for i := 0; i < m.InputCount(); i++ {
		slog.Debug("  input", "model", name, "index", i, "name", m.InputName(i))
	}
	for i := 0; i < m.OutputCount(); i++ {
		slog.Debug("  output", "model", name, "index", i, "name", m.OutputName(i))
	}
}
