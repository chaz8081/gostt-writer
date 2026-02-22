// internal/ble/protocol/chunk_test.go
package protocol

import (
	"strings"
	"testing"
)

const testMaxBytes = 50 // small limit for easy testing

func TestChunkTextFitsInOne(t *testing.T) {
	chunks := ChunkText("hello world", testMaxBytes)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if chunks[0] != "hello world" {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], "hello world")
	}
}

func TestChunkTextEmpty(t *testing.T) {
	chunks := ChunkText("", testMaxBytes)
	if len(chunks) != 0 {
		t.Errorf("got %d chunks for empty string, want 0", len(chunks))
	}
}

func TestChunkTextSplitsAtWordBoundary(t *testing.T) {
	// 60 chars, should split into 2 chunks at a word boundary
	text := "the quick brown fox jumps over the lazy dog sleeping today"
	chunks := ChunkText(text, testMaxBytes)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	// Each chunk must be <= testMaxBytes
	for i, c := range chunks {
		if len(c) > testMaxBytes {
			t.Errorf("chunk[%d] len=%d exceeds max=%d", i, len(c), testMaxBytes)
		}
	}
	// Reassembled text must equal original
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Errorf("reassembled = %q, want %q", reassembled, text)
	}
}

func TestChunkTextUTF8NeverSplitsMidChar(t *testing.T) {
	// Each emoji is 4 bytes. With max=10, can fit 2 emojis per chunk.
	text := "\U0001F600\U0001F601\U0001F602\U0001F603\U0001F604" // 5 emojis = 20 bytes
	chunks := ChunkText(text, 10)
	for i, c := range chunks {
		if len(c) > 10 {
			t.Errorf("chunk[%d] len=%d exceeds max=10", i, len(c))
		}
		// Each chunk must be valid UTF-8 (Go strings are valid by construction,
		// but verify no partial runes by checking rune count)
		for _, r := range c {
			if r == '\uFFFD' {
				t.Errorf("chunk[%d] contains replacement character (split mid-rune)", i)
			}
		}
	}
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Errorf("reassembled = %q, want %q", reassembled, text)
	}
}

func TestChunkTextExactFit(t *testing.T) {
	text := strings.Repeat("a", testMaxBytes)
	chunks := ChunkText(text, testMaxBytes)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], text)
	}
}

func TestChunkTextOneByteOver(t *testing.T) {
	text := strings.Repeat("a", testMaxBytes+1)
	chunks := ChunkText(text, testMaxBytes)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
}

func TestChunkTextLongWordForced(t *testing.T) {
	// A single word longer than maxBytes must be split mid-word (but not mid-rune)
	text := strings.Repeat("x", testMaxBytes+10)
	chunks := ChunkText(text, testMaxBytes)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want >= 2", len(chunks))
	}
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Errorf("reassembled length = %d, want %d", len(reassembled), len(text))
	}
}

func TestChunkTextZeroMax(t *testing.T) {
	chunks := ChunkText("hello", 0)
	if chunks != nil {
		t.Errorf("ChunkText with maxBytes=0 should return nil, got %v", chunks)
	}
}

func TestChunkTextMaxSmallerThanRune(t *testing.T) {
	// 4-byte emoji with maxBytes=1 should still make forward progress
	text := "\U0001F600" // 4 bytes
	chunks := ChunkText(text, 1)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (single rune forced)", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], text)
	}
}
