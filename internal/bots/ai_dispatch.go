// internal/bots/ai_dispatch.go
package bots

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	dispatchTimeout = 30 * time.Second
	replyChainHops  = 10
	replyCharBudget = 4000
)

// httpClient has an explicit timeout to guard against providers that accept the
// connection but never complete the response body, which would not be caught by
// the request context alone in all Go transport implementations.
var httpClient = &http.Client{Timeout: dispatchTimeout + 5*time.Second}

// PostFunc sends a message to a channel as the bot user.
type PostFunc func(ctx context.Context, channelID, botUserIDStr, content string) error

// Dispatcher holds dependencies for AI dispatch.
type Dispatcher struct {
	svc       *Service
	repo      *Repository
	ollamaURL string
	ollamaKey string
	botUserID int64
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(svc *Service, repo *Repository, ollamaURL, ollamaKey string, botUserID int64) *Dispatcher {
	return &Dispatcher{
		svc:       svc,
		repo:      repo,
		ollamaURL: ollamaURL,
		ollamaKey: ollamaKey,
		botUserID: botUserID,
	}
}

// BuildTrigger returns a BotTriggerFunc (to be set on MessageService once at startup).
// postFn is called to send the bot's reply.
func (d *Dispatcher) BuildTrigger(postFn PostFunc) func(ctx context.Context, msgID, channelID, serverID, authorID, content, parentID string) {
	return func(ctx context.Context, msgID, channelID, serverID, authorID, content, parentID string) {
		// Don't respond to the bot's own messages
		authorIDInt, _ := strconv.ParseInt(authorID, 10, 64)
		if authorIDInt == d.botUserID {
			return
		}

		// Only fire if the bot is in this server
		if serverID == "" {
			return
		}
		serverIDInt, _ := strconv.ParseInt(serverID, 10, 64)

		inServer, err := d.repo.IsBotInServer(ctx, serverIDInt, d.botUserID)
		if err != nil || !inServer {
			return
		}

		// Only respond if mentioned (@ai-chatbot or <@botUserID>) or this is a reply to the bot
		botUserIDStr := strconv.FormatInt(d.botUserID, 10)
		mentioned := strings.Contains(content, "@ai-chatbot") || strings.Contains(content, "<@"+botUserIDStr+">")

		isReplyToBot := false
		if parentID != "" && !mentioned {
			parentIDInt, _ := strconv.ParseInt(parentID, 10, 64)
			chain, err := d.repo.GetReplyChain(ctx, parentIDInt, 1, 100)
			if err == nil && len(chain) > 0 {
				isReplyToBot = chain[len(chain)-1].AuthorID == d.botUserID
			}
		}

		if !mentioned && !isReplyToBot {
			return
		}

		// Dispatch asynchronously so we never block the HTTP response
		go func() {
			dispCtx, cancel := context.WithTimeout(context.Background(), dispatchTimeout)
			defer cancel()

			if err := d.dispatch(dispCtx, serverIDInt, channelID, botUserIDStr, msgID, parentID, content, postFn); err != nil {
				log.Printf("bot dispatch error: %v", err)
			}
		}()
	}
}

// dispatch retrieves config, builds context, calls the provider, and posts the reply.
func (d *Dispatcher) dispatch(ctx context.Context, serverIDInt int64, channelID, botUserIDStr, msgIDStr, parentID, content string, postFn PostFunc) error {
	// Get AI config directly from repo — no auth check needed for internal dispatch path.
	cfg, rawKeyEnc, err := d.repo.GetAIConfig(ctx, serverIDInt)
	if err != nil {
		return fmt.Errorf("get ai config: %w", err)
	}

	provider := "parley"
	model := "ministral-3:14b"
	systemPrompt := ""
	var apiKey string

	if cfg != nil {
		provider = cfg.Provider
		model = cfg.Model
		systemPrompt = cfg.SystemPrompt
		if rawKeyEnc != "" {
			apiKey, err = d.svc.DecryptAPIKey(rawKeyEnc)
			if err != nil {
				return fmt.Errorf("decrypt api key: %w", err)
			}
		}
	}

	// Check monthly allowance for parley provider
	if provider == "parley" {
		used, err := d.repo.GetMonthlyUsage(ctx, serverIDInt)
		if err != nil {
			return fmt.Errorf("get monthly usage: %w", err)
		}
		limit := ParleyModelAllowances[model]
		if limit == 0 {
			limit = 100_000
		}
		if used >= limit {
			return nil // silently skip — over quota
		}
	}

	// Build conversation context from reply chain
	messages := d.buildMessages(ctx, msgIDStr, parentID, content, systemPrompt)

	// Call provider
	var reply string
	var tokensUsed int64
	switch provider {
	case "parley":
		reply, tokensUsed, err = d.callOllama(ctx, model, messages, d.ollamaURL, d.ollamaKey)
	case "anthropic":
		reply, tokensUsed, err = d.callAnthropic(ctx, model, messages, apiKey)
	case "openai", "xai", "mistral":
		baseURL := openAICompatURL(provider)
		reply, tokensUsed, err = d.callOpenAICompat(ctx, model, messages, apiKey, baseURL)
	case "google":
		reply, tokensUsed, err = d.callGoogle(ctx, model, messages, apiKey)
	default:
		return fmt.Errorf("unknown provider: %s", provider)
	}
	if err != nil {
		return fmt.Errorf("provider %s: %w", provider, err)
	}
	if reply == "" {
		return nil
	}

	// Track usage for parley provider
	if provider == "parley" && tokensUsed > 0 {
		_ = d.repo.AddTokenUsage(ctx, serverIDInt, tokensUsed)
	}

	// Post reply to channel
	return postFn(ctx, channelID, botUserIDStr, reply)
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// buildMessages constructs the message array for the LLM, walking the reply chain.
func (d *Dispatcher) buildMessages(ctx context.Context, msgIDStr, parentID, content, systemPrompt string) []chatMessage {
	var msgs []chatMessage
	if systemPrompt != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: systemPrompt})
	}

	// Walk reply chain for context
	if parentID != "" {
		parentIDInt, _ := strconv.ParseInt(parentID, 10, 64)
		chain, err := d.repo.GetReplyChain(ctx, parentIDInt, replyChainHops, replyCharBudget)
		if err == nil {
			for _, cm := range chain {
				role := "user"
				if cm.IsBot {
					role = "assistant"
				}
				msgs = append(msgs, chatMessage{Role: role, Content: cm.Content})
			}
		}
	}

	msgs = append(msgs, chatMessage{Role: "user", Content: content})
	return msgs
}

// --- Ollama (Parley provider) ---

type ollamaRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	PromptEvalCount int64 `json:"prompt_eval_count"`
	EvalCount       int64 `json:"eval_count"`
}

func (d *Dispatcher) callOllama(ctx context.Context, model string, messages []chatMessage, baseURL, apiKey string) (string, int64, error) {
	// Strip :cloud suffix (UI display only, not sent to Ollama)
	ollamaModel := strings.TrimSuffix(model, ":cloud")

	reqBody, _ := json.Marshal(ollamaRequest{Model: ollamaModel, Messages: messages, Stream: false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("ollama %d: %s", resp.StatusCode, body)
	}

	var out ollamaResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, err
	}
	tokens := out.PromptEvalCount + out.EvalCount
	return strings.TrimSpace(out.Message.Content), tokens, nil
}

// --- Anthropic ---

type anthropicRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []chatMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

func (d *Dispatcher) callAnthropic(ctx context.Context, model string, messages []chatMessage, apiKey string) (string, int64, error) {
	// Extract system message if present
	system := ""
	var userMsgs []chatMessage
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
		} else {
			userMsgs = append(userMsgs, m)
		}
	}

	reqBody, _ := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: 1024,
		System:    system,
		Messages:  userMsgs,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("anthropic %d: %s", resp.StatusCode, body)
	}

	var out anthropicResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, err
	}
	if len(out.Content) == 0 {
		return "", 0, errors.New("empty anthropic response")
	}
	tokens := out.Usage.InputTokens + out.Usage.OutputTokens
	return strings.TrimSpace(out.Content[0].Text), tokens, nil
}

// --- OpenAI-compatible (OpenAI, xAI, Mistral) ---

type openAIRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"usage"`
}

func openAICompatURL(provider string) string {
	switch provider {
	case "xai":
		return "https://api.x.ai/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	default: // openai
		return "https://api.openai.com/v1"
	}
}

func (d *Dispatcher) callOpenAICompat(ctx context.Context, model string, messages []chatMessage, apiKey, baseURL string) (string, int64, error) {
	reqBody, _ := json.Marshal(openAIRequest{Model: model, Messages: messages})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("%s %d: %s", baseURL, resp.StatusCode, body)
	}

	var out openAIResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, err
	}
	if len(out.Choices) == 0 {
		return "", 0, errors.New("empty response")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), out.Usage.TotalTokens, nil
}

// --- Google Gemini ---

type googleRequest struct {
	Contents          []googleContent      `json:"contents"`
	SystemInstruction *googleSystemContent `json:"systemInstruction,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role"`
	Parts []googlePart `json:"parts"`
}

type googleSystemContent struct {
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		TotalTokenCount int64 `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (d *Dispatcher) callGoogle(ctx context.Context, model string, messages []chatMessage, apiKey string) (string, int64, error) {
	var contents []googleContent
	var sysInstruction *googleSystemContent

	for _, m := range messages {
		if m.Role == "system" {
			sysInstruction = &googleSystemContent{Parts: []googlePart{{Text: m.Content}}}
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, googleContent{
			Role:  role,
			Parts: []googlePart{{Text: m.Content}},
		})
	}

	reqBody, _ := json.Marshal(googleRequest{Contents: contents, SystemInstruction: sysInstruction})
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("google %d: %s", resp.StatusCode, body)
	}

	var out googleResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", 0, errors.New("empty google response")
	}
	return strings.TrimSpace(out.Candidates[0].Content.Parts[0].Text), out.UsageMetadata.TotalTokenCount, nil
}
