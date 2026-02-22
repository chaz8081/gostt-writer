package transcribe

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// loadVocabulary reads parakeet_vocab.json and returns a token ID -> string mapping.
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
