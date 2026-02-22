package transcribe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVocabulary(t *testing.T) {
	vocabJSON := `{"0": "▁the", "1": "▁a", "2": "s", "1024": "<blank>"}`
	tmpDir := t.TempDir()
	vocabPath := filepath.Join(tmpDir, "parakeet_vocab.json")
	os.WriteFile(vocabPath, []byte(vocabJSON), 0644)

	vocab, err := loadVocabulary(vocabPath)
	if err != nil {
		t.Fatalf("loadVocabulary: %v", err)
	}
	if len(vocab) != 1025 {
		t.Errorf("len(vocab) = %d, want 1025", len(vocab))
	}
	if vocab[0] != "▁the" {
		t.Errorf("vocab[0] = %q, want %q", vocab[0], "▁the")
	}
	if vocab[1024] != "<blank>" {
		t.Errorf("vocab[1024] = %q, want %q", vocab[1024], "<blank>")
	}
}

func TestLoadVocabularyBadPath(t *testing.T) {
	_, err := loadVocabulary("/nonexistent/vocab.json")
	if err == nil {
		t.Error("loadVocabulary should fail for nonexistent file")
	}
}

func TestLoadVocabularyBadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	vocabPath := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(vocabPath, []byte("not json"), 0644)

	_, err := loadVocabulary(vocabPath)
	if err == nil {
		t.Error("loadVocabulary should fail for invalid JSON")
	}
}

func TestDecodeTokens(t *testing.T) {
	vocab := []string{"▁the", "▁a", "s", "k"}
	tokens := []int32{0, 1, 2, 3}
	text := decodeTokens(tokens, vocab)
	if text != "the ask" {
		t.Errorf("decodeTokens = %q, want %q", text, "the ask")
	}
}

func TestDecodeTokensEmpty(t *testing.T) {
	vocab := []string{"▁hello"}
	text := decodeTokens(nil, vocab)
	if text != "" {
		t.Errorf("decodeTokens(nil) = %q, want empty", text)
	}
}

func TestDecodeTokensOutOfRange(t *testing.T) {
	vocab := []string{"▁hi"}
	tokens := []int32{0, 999} // 999 is out of range
	text := decodeTokens(tokens, vocab)
	if text != "hi" {
		t.Errorf("decodeTokens with OOB = %q, want %q", text, "hi")
	}
}
