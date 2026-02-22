// internal/ble/protocol/chunk.go
package protocol

import "unicode/utf8"

// MaxPayloadBytes is the usable text bytes per BLE packet after
// protobuf framing + AES-GCM overhead (253 - ~40 bytes overhead).
const MaxPayloadBytes = 213

// ChunkText splits text into chunks that each fit within maxBytes.
// It prefers splitting at word boundaries (spaces) and never splits
// in the middle of a UTF-8 character. Returns nil for empty text.
func ChunkText(text string, maxBytes int) []string {
	if len(text) == 0 {
		return nil
	}
	if len(text) <= maxBytes {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxBytes {
			chunks = append(chunks, text)
			break
		}

		// Find the split point: start at maxBytes and walk back to find
		// a space. If no space found, split at the last valid UTF-8 boundary.
		split := maxBytes

		// Ensure we don't split in the middle of a UTF-8 character.
		// Walk back until we're at the start of a rune.
		for split > 0 && !utf8.RuneStart(text[split]) {
			split--
		}

		// Try to find a word boundary (space) by walking back from split.
		bestSpace := -1
		for i := split; i > 0; i-- {
			if text[i-1] == ' ' {
				bestSpace = i
				break
			}
		}

		if bestSpace > 0 {
			// Split at word boundary — include the space in the first chunk
			// so reassembly is exact.
			chunks = append(chunks, text[:bestSpace])
			text = text[bestSpace:]
		} else {
			// No space found — forced split at UTF-8 boundary
			chunks = append(chunks, text[:split])
			text = text[split:]
		}
	}
	return chunks
}
