package transcribe

import "fmt"

// NewParakeetTranscriber creates a Parakeet TDT CoreML transcriber.
// This is a stub that will be replaced in Task 5 with the real implementation.
func NewParakeetTranscriber(modelDir string) (*parakeetStub, error) {
	return nil, fmt.Errorf("parakeet backend not yet implemented (model dir: %s)", modelDir)
}

// parakeetStub is a placeholder type satisfying the Transcriber interface.
// It will be replaced by ParakeetTranscriber in Task 5.
type parakeetStub struct{}

func (p *parakeetStub) Process(samples []float32) (string, error) {
	return "", fmt.Errorf("parakeet backend not yet implemented")
}

func (p *parakeetStub) Close() error {
	return nil
}
