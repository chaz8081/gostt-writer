package transcribe

import (
	"fmt"
	"log/slog"
	"math"
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

	// Cached I/O names discovered via model introspection (sorted alphabetically).
	prepInputNames  []string
	encInputNames   []string
	decInputNames   []string
	jointInputNames []string
}

// NewParakeetTranscriber loads the 4 CoreML models and vocabulary from modelDir.
func NewParakeetTranscriber(modelDir string) (*ParakeetTranscriber, error) {
	// Load vocabulary
	vocabPath := modelDir + "/parakeet_vocab.json"
	vocab, err := loadVocabulary(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("parakeet: %w", err)
	}

	// Load CoreML models
	// Preprocessor runs on CPU (mel spectrogram is faster on CPU)
	coreml.SetComputeUnits(coreml.ComputeCPUOnly)
	preprocessor, err := coreml.LoadModel(modelDir + "/Preprocessor.mlmodelc")
	if err != nil {
		return nil, fmt.Errorf("parakeet: load preprocessor: %w", err)
	}

	// Encoder, decoder, joint run on all units (ANE preferred)
	coreml.SetComputeUnits(coreml.ComputeAll)
	encoder, err := coreml.LoadModel(modelDir + "/Encoder.mlmodelc")
	if err != nil {
		preprocessor.Close()
		return nil, fmt.Errorf("parakeet: load encoder: %w", err)
	}

	decoder, err := coreml.LoadModel(modelDir + "/Decoder.mlmodelc")
	if err != nil {
		preprocessor.Close()
		encoder.Close()
		return nil, fmt.Errorf("parakeet: load decoder: %w", err)
	}

	joint, err := coreml.LoadModel(modelDir + "/JointDecision.mlmodelc")
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

	// Cache sorted input names from model introspection
	p.prepInputNames = modelInputNames(preprocessor)
	p.encInputNames = modelInputNames(encoder)
	p.decInputNames = modelInputNames(decoder)
	p.jointInputNames = modelInputNames(joint)

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
	prepResult, err := p.runPreprocessor(padded)
	if err != nil {
		return "", fmt.Errorf("parakeet: preprocessor: %w", err)
	}
	defer prepResult.Close()

	// Step 2: Encoder (mel features → encoder hidden states)
	encResult, err := p.runEncoder(prepResult)
	if err != nil {
		return "", fmt.Errorf("parakeet: encoder: %w", err)
	}
	defer encResult.Close()

	// Extract encoder output and length
	encoderOutput, encoderLength, err := p.extractEncoderOutput(encResult)
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
func (p *ParakeetTranscriber) runPreprocessor(audio []float32) (*coreml.PredictAllocResult, error) {
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

	// Map tensors to sorted input names
	inputMap := map[string]*coreml.Tensor{
		"audio_signal": audioTensor,
		"audio_length": audioLenTensor,
	}
	inputs, err := orderInputs(p.prepInputNames, inputMap)
	if err != nil {
		return nil, err
	}

	return p.preprocessor.PredictAlloc(p.prepInputNames, inputs)
}

// runEncoder runs the encoder model on preprocessor outputs.
func (p *ParakeetTranscriber) runEncoder(prepResult *coreml.PredictAllocResult) (*coreml.PredictAllocResult, error) {
	// Map preprocessor outputs to encoder input names
	inputMap := make(map[string]*coreml.Tensor)
	for i, name := range prepResult.Names {
		inputMap[name] = prepResult.Tensors[i]
	}
	inputs, err := orderInputs(p.encInputNames, inputMap)
	if err != nil {
		return nil, err
	}

	return p.encoder.PredictAlloc(p.encInputNames, inputs)
}

// extractEncoderOutput extracts the flattened encoder hidden states and length from encoder outputs.
// The encoder output shape is [1, encoderHidden, T] (not [1, T, encoderHidden]).
func (p *ParakeetTranscriber) extractEncoderOutput(encResult *coreml.PredictAllocResult) ([]float32, int, error) {
	encoderTensor := encResult.Tensor("encoder")
	lengthTensor := encResult.Tensor("encoder_length")

	if encoderTensor == nil {
		return nil, 0, fmt.Errorf("no 'encoder' output tensor found in result (got %v)", encResult.Names)
	}

	// Encoder output shape: [1, H, T] where H=1024 (or similar)
	if encoderTensor.Rank() != 3 {
		return nil, 0, fmt.Errorf("encoder output has rank %d, expected 3", encoderTensor.Rank())
	}

	H := int(encoderTensor.Dim(1)) // encoder hidden size
	T := int(encoderTensor.Dim(2)) // number of frames

	// Extract encoder length
	var encoderLength int
	if lengthTensor != nil && lengthTensor.DType() == coreml.DTypeInt32 {
		data := (*int32)(lengthTensor.DataPtr())
		encoderLength = int(*data)
	} else {
		encoderLength = T
	}

	slog.Debug("parakeet encoder output", "shape", encoderTensor.Shape(), "H", H, "T", T, "encoderLength", encoderLength)

	// The decode loop expects encoderOutput as a flat array indexed by [t*H + h].
	// CoreML stores the data in row-major order as [1, H, T] meaning memory layout is H×T.
	// We need to transpose to [T, H] so the decode loop can index by frame.
	totalFloats := H * T
	srcData := unsafe.Slice((*float32)(encoderTensor.DataPtr()), totalFloats)

	encoderData := make([]float32, totalFloats)
	if encoderTensor.DType() == coreml.DTypeFloat16 {
		src16 := unsafe.Slice((*uint16)(encoderTensor.DataPtr()), totalFloats)
		// Transpose [H, T] → [T, H] with float16→float32 conversion
		for h := 0; h < H; h++ {
			for t := 0; t < T; t++ {
				encoderData[t*H+h] = float16ToFloat32(src16[h*T+t])
			}
		}
	} else {
		// Transpose [H, T] → [T, H]
		for h := 0; h < H; h++ {
			for t := 0; t < T; t++ {
				encoderData[t*H+h] = srcData[h*T+t]
			}
		}
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

	// Create target_length tensor [1] with value 1 (always decoding 1 target at a time)
	targetLen := []int32{1}
	targetLenTensor, err := coreml.NewTensorWithData(
		[]int64{1},
		coreml.DTypeInt32,
		unsafe.Pointer(&targetLen[0]),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create target_length tensor: %w", err)
	}
	defer targetLenTensor.Close()

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

	// Map tensors to sorted input names
	inputMap := map[string]*coreml.Tensor{
		"targets":       targetsTensor,
		"target_length": targetLenTensor,
		"h_in":          hInTensor,
		"c_in":          cInTensor,
	}
	inputs, err := orderInputs(p.decInputNames, inputMap)
	if err != nil {
		return nil, nil, nil, err
	}

	result, err := p.decoder.PredictAlloc(p.decInputNames, inputs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("predict: %w", err)
	}
	defer result.Close()

	// Extract outputs by name
	decTensor := result.Tensor("decoder")
	hOutTensor := result.Tensor("h_out")
	cOutTensor := result.Tensor("c_out")

	if decTensor == nil || hOutTensor == nil || cOutTensor == nil {
		return nil, nil, nil, fmt.Errorf("missing decoder outputs (got %v)", result.Names)
	}

	// Copy outputs to Go slices
	decoderOut = copyFloat32FromTensor(decTensor, parakeetDecoderHidden)
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

	// Map tensors to sorted input names
	inputMap := map[string]*coreml.Tensor{
		"encoder_step": encStepTensor,
		"decoder_step": decStepTensor,
	}
	inputs, err := orderInputs(p.jointInputNames, inputMap)
	if err != nil {
		return 0, 0, err
	}

	result, err := p.joint.PredictAlloc(p.jointInputNames, inputs)
	if err != nil {
		return 0, 0, fmt.Errorf("predict: %w", err)
	}
	defer result.Close()

	// Extract outputs by name
	tokenTensor := result.Tensor("token_id")
	durTensor := result.Tensor("duration")

	if tokenTensor == nil || durTensor == nil {
		return 0, 0, fmt.Errorf("missing joint outputs (got %v)", result.Names)
	}

	// Extract token_id and duration
	tokenPtr := (*int32)(tokenTensor.DataPtr())
	tokenID = *tokenPtr

	durPtr := (*int32)(durTensor.DataPtr())
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

// orderInputs arranges tensors to match the sorted input name order.
func orderInputs(names []string, tensorMap map[string]*coreml.Tensor) ([]*coreml.Tensor, error) {
	result := make([]*coreml.Tensor, len(names))
	for i, name := range names {
		t, ok := tensorMap[name]
		if !ok {
			return nil, fmt.Errorf("missing input tensor for %q", name)
		}
		result[i] = t
	}
	return result, nil
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
	switch exp {
	case 0:
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
	case 0x1f:
		// Inf/NaN
		f = (sign << 31) | (0xff << 23) | (frac << 13)
	default:
		// Normal
		f = (sign << 31) | ((exp + 127 - 15) << 23) | (frac << 13)
	}

	return math.Float32frombits(f)
}

// modelInputNames returns all input names for a model (sorted alphabetically).
func modelInputNames(m *coreml.Model) []string {
	names := make([]string, m.InputCount())
	for i := range names {
		names[i] = m.InputName(i)
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
