// Package rewrite provides LLM-based post-processing of transcribed text
// via a local Ollama instance.
package rewrite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chaz8081/gostt-writer/internal/config"
)

// Rewriter sends transcribed text to a local Ollama LLM for transformation.
// On any error, Rewrite returns the original text so callers can gracefully
// fall back to raw transcription.
type Rewriter struct {
	url     string
	model   string
	prompt  string
	timeout time.Duration
	client  *http.Client
}

// New creates a Rewriter from the given config.
func New(cfg *config.RewriteConfig) *Rewriter {
	timeout := time.Duration(cfg.TimeoutSecs) * time.Second
	return &Rewriter{
		url:     cfg.OllamaURL,
		model:   cfg.Model,
		prompt:  cfg.Prompt,
		timeout: timeout,
		client:  &http.Client{},
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// Rewrite sends rawText to the Ollama LLM for transformation using the
// configured system prompt. On any error it returns (rawText, err) so
// callers can log the error and use the original text.
func (r *Rewriter) Rewrite(ctx context.Context, rawText string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	req := chatRequest{
		Model: r.model,
		Messages: []chatMessage{
			{Role: "system", Content: r.prompt},
			{Role: "user", Content: rawText},
		},
		Stream: false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return rawText, fmt.Errorf("rewrite: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return rawText, fmt.Errorf("rewrite: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return rawText, fmt.Errorf("rewrite: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rawText, fmt.Errorf("rewrite: ollama returned HTTP %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return rawText, fmt.Errorf("rewrite: read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return rawText, fmt.Errorf("rewrite: unmarshal response: %w", err)
	}

	rewritten := chatResp.Message.Content
	if rewritten == "" {
		return rawText, nil
	}

	return rewritten, nil
}
