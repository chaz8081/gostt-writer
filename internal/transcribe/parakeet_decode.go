package transcribe

import "fmt"

const (
	parakeetBlankID        = 1024 // blank token index for v2 CoreML model (FluidInference conversion)
	parakeetMaxSymsPerStep = 10
	parakeetEncoderHidden  = 1024
	parakeetDecoderHidden  = 640
	parakeetLSTMLayers     = 2
)

var parakeetDurationBins = []int32{0, 1, 2, 3, 4}

// decoderRunner runs the LSTM decoder for one step.
type decoderRunner interface {
	runDecoder(targetID int32, hIn, cIn []float32) (decoderOut, hOut, cOut []float32, err error)
}

// jointRunner runs the joint decision network for one step.
type jointRunner interface {
	runJoint(encoderStep, decoderStep []float32) (tokenID, duration int32, err error)
}

// tdtDecode runs the TDT greedy decode algorithm over encoder output frames.
// encoderOutput shape: [T, encoderHidden] flattened.
// encoderLength: number of valid frames.
// Returns decoded token IDs (excluding blank tokens).
func tdtDecode(
	encoderOutput []float32,
	encoderLength int,
	dec decoderRunner,
	joint jointRunner,
) ([]int32, error) {
	// Initialize LSTM state (zeros)
	lstmStateSize := parakeetLSTMLayers * 1 * parakeetDecoderHidden
	hState := make([]float32, lstmStateSize)
	cState := make([]float32, lstmStateSize)

	// Initial decoder run with blank token
	decoderOut, hState, cState, err := dec.runDecoder(int32(parakeetBlankID), hState, cState)
	if err != nil {
		return nil, fmt.Errorf("initial decoder run: %w", err)
	}

	var tokens []int32
	t := 0

	for t < encoderLength {
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
				break
			}

			symCount++
		}

		if symCount >= parakeetMaxSymsPerStep {
			t++
		}
	}

	return tokens, nil
}
