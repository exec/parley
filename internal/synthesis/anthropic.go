package synthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider calls the Anthropic Messages API directly. Built but
// intentionally unwired in cmd/api/main.go for V1 — see Phase A spec §5.2.
// When the time comes to flip the provider, wire this in alongside (or in
// place of) OllamaProvider in main.go.
type AnthropicProvider struct {
	APIKey  string
	Model   string
	BaseURL string // defaults to https://api.anthropic.com
	client  *http.Client
}

// NewAnthropicProvider builds a provider against api.anthropic.com.
// Recommended model: claude-sonnet-4-6 (cheap, fast, plenty smart for
// CLAUDE.md synthesis). Pass an empty BaseURL to use the default.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.anthropic.com",
		client:  &http.Client{Timeout: 80 * time.Second},
	}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

// Anthropic Messages API request/response shapes (subset we need).
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
}

// SynthesizeClaudeMD calls POST /v1/messages.
func (p *AnthropicProvider) SynthesizeClaudeMD(ctx context.Context, in Input) (string, error) {
	if p.APIKey == "" {
		return "", errors.New("anthropic: API key not set")
	}

	payload := anthropicRequest{
		Model:     p.Model,
		MaxTokens: 2048,
		System:    SystemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: buildUserPrompt(in)},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, snippet)
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("anthropic: decode response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", errors.New("anthropic: empty response content")
	}

	// Concatenate all text blocks (the API can stream multiple).
	var out bytes.Buffer
	for _, c := range result.Content {
		if c.Type == "text" {
			out.WriteString(c.Text)
		}
	}
	return cleanFences(out.String()), nil
}
