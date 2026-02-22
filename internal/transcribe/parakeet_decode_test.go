package transcribe

import (
	"fmt"
	"testing"
)

// mockDecoder returns predetermined decoder outputs for testing.
type mockDecoder struct {
	calls   int
	outputs []mockDecoderOutput
}

type mockDecoderOutput struct {
	decoderOut []float32
	hOut       []float32
	cOut       []float32
}

func (m *mockDecoder) runDecoder(targetID int32, hIn, cIn []float32) (decoderOut, hOut, cOut []float32, err error) {
	if m.calls >= len(m.outputs) {
		// Return zeros for any extra calls
		size := parakeetDecoderHidden
		return make([]float32, size), make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), nil
	}
	out := m.outputs[m.calls]
	m.calls++
	return out.decoderOut, out.hOut, out.cOut, nil
}

// mockJoint returns predetermined joint decisions for testing.
type mockJoint struct {
	calls   int
	results []mockJointResult
}

type mockJointResult struct {
	tokenID  int32
	duration int32
}

func (m *mockJoint) runJoint(encoderStep, decoderStep []float32) (tokenID, duration int32, err error) {
	if m.calls >= len(m.results) {
		return parakeetBlankID, 1, nil // default: blank, advance 1
	}
	r := m.results[m.calls]
	m.calls++
	return r.tokenID, r.duration, nil
}

func TestTDTDecodeBasic(t *testing.T) {
	// 3 encoder frames, each 1024 floats
	encoderOutput := make([]float32, 3*parakeetEncoderHidden)

	// Mock joint: frame 0 emits token 5 (dur 1), frame 1 emits token 10 (dur 1), frame 2 blank (dur 1)
	joint := &mockJoint{results: []mockJointResult{
		{tokenID: 5, duration: 1},               // frame 0: emit 5, advance 1
		{tokenID: 10, duration: 1},              // frame 1: emit 10, advance 1
		{tokenID: parakeetBlankID, duration: 1}, // frame 2: blank, advance 1
	}}

	// Mock decoder: return dummy outputs for each call
	dec := &mockDecoder{outputs: []mockDecoderOutput{
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
	}}

	tokens, err := tdtDecode(encoderOutput, 3, dec, joint)
	if err != nil {
		t.Fatalf("tdtDecode: %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2", len(tokens))
	}
	if tokens[0] != 5 || tokens[1] != 10 {
		t.Errorf("tokens = %v, want [5, 10]", tokens)
	}
}

func TestTDTDecodeBlankSkip(t *testing.T) {
	// 5 encoder frames
	encoderOutput := make([]float32, 5*parakeetEncoderHidden)

	// Frame 0: blank with duration 3 (skip to frame 3)
	// Frame 3: emit token 7 (dur 1), advance to frame 4
	// Frame 4: blank (dur 1)
	joint := &mockJoint{results: []mockJointResult{
		{tokenID: parakeetBlankID, duration: 3}, // frame 0: skip 3 frames
		{tokenID: 7, duration: 1},               // frame 3: emit 7
		{tokenID: parakeetBlankID, duration: 1}, // frame 4: blank
	}}

	dec := &mockDecoder{outputs: []mockDecoderOutput{
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
	}}

	tokens, err := tdtDecode(encoderOutput, 5, dec, joint)
	if err != nil {
		t.Fatalf("tdtDecode: %v", err)
	}

	if len(tokens) != 1 || tokens[0] != 7 {
		t.Errorf("tokens = %v, want [7]", tokens)
	}
}

func TestTDTDecodeMaxSymbolsGuard(t *testing.T) {
	// 1 encoder frame, joint keeps emitting non-blank tokens with duration 0
	encoderOutput := make([]float32, 1*parakeetEncoderHidden)

	// Emit 15 tokens with duration 0 — should be capped at parakeetMaxSymsPerStep (10)
	results := make([]mockJointResult, 15)
	for i := range results {
		results[i] = mockJointResult{tokenID: int32(i), duration: 0}
	}
	joint := &mockJoint{results: results}

	// Need enough decoder outputs for 10 calls (initial + 10 token emissions)
	outputs := make([]mockDecoderOutput, 12)
	for i := range outputs {
		outputs[i] = mockDecoderOutput{
			decoderOut: make([]float32, parakeetDecoderHidden),
			hOut:       make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden),
			cOut:       make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden),
		}
	}
	dec := &mockDecoder{outputs: outputs}

	tokens, err := tdtDecode(encoderOutput, 1, dec, joint)
	if err != nil {
		t.Fatalf("tdtDecode: %v", err)
	}

	if len(tokens) > parakeetMaxSymsPerStep {
		t.Errorf("got %d tokens, want at most %d (max symbols per step)", len(tokens), parakeetMaxSymsPerStep)
	}
}

func TestTDTDecodeBlankDurationZeroForceAdvance(t *testing.T) {
	// If blank with duration 0, should advance by 1 to prevent infinite loop
	encoderOutput := make([]float32, 2*parakeetEncoderHidden)

	joint := &mockJoint{results: []mockJointResult{
		{tokenID: parakeetBlankID, duration: 0}, // frame 0: blank, dur 0 -> should force advance to 1
		{tokenID: parakeetBlankID, duration: 1}, // frame 1: blank, advance 1
	}}

	dec := &mockDecoder{outputs: []mockDecoderOutput{
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
	}}

	tokens, err := tdtDecode(encoderOutput, 2, dec, joint)
	if err != nil {
		t.Fatalf("tdtDecode: %v", err)
	}

	// Should emit no tokens (both were blanks)
	if len(tokens) != 0 {
		t.Errorf("got %d tokens, want 0", len(tokens))
	}
}

func TestTDTDecodeEmptyEncoder(t *testing.T) {
	tokens, err := tdtDecode(nil, 0, &mockDecoder{outputs: []mockDecoderOutput{
		{decoderOut: make([]float32, parakeetDecoderHidden), hOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden), cOut: make([]float32, parakeetLSTMLayers*1*parakeetDecoderHidden)},
	}}, &mockJoint{})
	if err != nil {
		t.Fatalf("tdtDecode: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("got %d tokens, want 0 for empty encoder", len(tokens))
	}
}

func TestTDTDecodeDecoderError(t *testing.T) {
	encoderOutput := make([]float32, 1*parakeetEncoderHidden)

	// Initial decoder call fails
	dec := &errorDecoder{err: fmt.Errorf("decoder failed")}
	joint := &mockJoint{}

	_, err := tdtDecode(encoderOutput, 1, dec, joint)
	if err == nil {
		t.Error("expected error from decoder failure")
	}
}

// errorDecoder always returns an error.
type errorDecoder struct {
	err error
}

func (e *errorDecoder) runDecoder(targetID int32, hIn, cIn []float32) ([]float32, []float32, []float32, error) {
	return nil, nil, nil, e.err
}
