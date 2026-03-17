package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient calls the native Ollama chat API.
type OllamaClient struct {
	URL    string
	Key    string
	Model  string
	client *http.Client
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

// NewOllamaClient creates a new OllamaClient with an 80-second HTTP timeout.
func NewOllamaClient(url, key, model string) *OllamaClient {
	return &OllamaClient{
		URL:   url,
		Key:   key,
		Model: model,
		client: &http.Client{
			Timeout: 80 * time.Second,
		},
	}
}

// Generate sends a chat request to the Ollama API and returns the assistant's reply.
// It calls POST {URL}/chat with a system message and a user message.
func (c *OllamaClient) Generate(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	payload := ollamaRequest{
		Model: c.Model,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
		Stream: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ollama: status %d: %s", resp.StatusCode, snippet)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: decode response: %w", err)
	}

	return result.Message.Content, nil
}
