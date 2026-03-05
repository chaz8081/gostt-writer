package rewrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chaz8081/gostt-writer/internal/config"
)

func newTestRewriter(url string, timeoutSecs int) *Rewriter {
	cfg := &config.RewriteConfig{
		Enabled:     true,
		OllamaURL:   url,
		Model:       "test-model",
		Prompt:      "Clean up this text.",
		TimeoutSecs: timeoutSecs,
	}
	return New(cfg)
}

func TestRewriteSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if req.Model != "test-model" {
			t.Errorf("model = %q, want %q", req.Model, "test-model")
		}
		if req.Stream {
			t.Errorf("stream = true, want false")
		}
		if len(req.Messages) != 2 {
			t.Errorf("messages count = %d, want 2", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("messages[0].role = %q, want %q", req.Messages[0].Role, "system")
		}
		if req.Messages[1].Role != "user" {
			t.Errorf("messages[1].role = %q, want %q", req.Messages[1].Role, "user")
		}

		resp := chatResponse{
			Message: chatMessage{Role: "assistant", Content: "Cleaned up text."},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rw := newTestRewriter(srv.URL, 10)
	result, err := rw.Rewrite(context.Background(), "um so like cleaned up text")
	if err != nil {
		t.Fatalf("Rewrite() error = %v", err)
	}
	if result != "Cleaned up text." {
		t.Errorf("Rewrite() = %q, want %q", result, "Cleaned up text.")
	}
}

func TestRewriteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		resp := chatResponse{Message: chatMessage{Content: "late"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rw := newTestRewriter(srv.URL, 1)
	result, err := rw.Rewrite(context.Background(), "hello")
	if err == nil {
		t.Fatal("Rewrite() should return error on timeout")
	}
	if result != "hello" {
		t.Errorf("Rewrite() fallback = %q, want %q", result, "hello")
	}
}

func TestRewriteServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	rw := newTestRewriter(srv.URL, 10)
	result, err := rw.Rewrite(context.Background(), "hello")
	if err == nil {
		t.Fatal("Rewrite() should return error on HTTP 500")
	}
	if result != "hello" {
		t.Errorf("Rewrite() fallback = %q, want %q", result, "hello")
	}
}

func TestRewriteUnreachableServer(t *testing.T) {
	rw := newTestRewriter("http://127.0.0.1:1", 2)
	result, err := rw.Rewrite(context.Background(), "hello")
	if err == nil {
		t.Fatal("Rewrite() should return error for unreachable server")
	}
	if result != "hello" {
		t.Errorf("Rewrite() fallback = %q, want %q", result, "hello")
	}
}

func TestRewriteEmptyLLMResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{Message: chatMessage{Content: ""}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rw := newTestRewriter(srv.URL, 10)
	result, err := rw.Rewrite(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Rewrite() error = %v", err)
	}
	// Empty LLM response => falls back to raw text
	if result != "hello" {
		t.Errorf("Rewrite() = %q, want %q (raw fallback)", result, "hello")
	}
}
