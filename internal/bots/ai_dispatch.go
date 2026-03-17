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
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	ws "parley/internal/websocket"
)

const (
	dispatchTimeout  = 60 * time.Second
	dispatchRetries  = 1 // retry once on transient errors before giving up
	replyChainHops   = 50
	replyCharBudget  = 32000
)

// httpClient has an explicit timeout to guard against providers that accept the
// connection but never complete the response body, which would not be caught by
// the request context alone in all Go transport implementations.
var httpClient = &http.Client{Timeout: dispatchTimeout + 5*time.Second}

// PostFunc sends a message to a channel as the bot user.
// replyToMsgID, if non-empty, sets the parent (reply thread) of the posted message.
type PostFunc func(ctx context.Context, channelID, botUserIDStr, content, replyToMsgID string) error

// mentionRe matches <@123456> user mention tags.
var mentionRe = regexp.MustCompile(`<@(\d+)>`)

// Dispatcher holds dependencies for AI dispatch.
type Dispatcher struct {
	svc       *Service
	repo      *Repository
	hub       *ws.Hub
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

// SetHub wires the WebSocket hub so degraded-status changes can be pushed live.
func (d *Dispatcher) SetHub(hub *ws.Hub) { d.hub = hub }

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

		// Only respond if mentioned (@polly or <@botUserID>) or this is a reply to the bot
		botUserIDStr := strconv.FormatInt(d.botUserID, 10)
		mentioned := strings.Contains(content, "@polly") || strings.Contains(content, "<@"+botUserIDStr+">")

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

		// Dispatch asynchronously so we never block the HTTP response.
		// Budget covers all attempts: dispatchTimeout per attempt × (1 + retries) + padding.
		go func() {
			totalCtx, cancel := context.WithTimeout(context.Background(), dispatchTimeout*(dispatchRetries+1)+10*time.Second)
			defer cancel()

			var lastErr error
			for attempt := 0; attempt <= dispatchRetries; attempt++ {
				if attempt > 0 {
					log.Printf("bot dispatch retry %d/%d after: %v", attempt, dispatchRetries, lastErr)
				}
				attemptCtx, attemptCancel := context.WithTimeout(totalCtx, dispatchTimeout)
				lastErr = d.dispatch(attemptCtx, serverIDInt, channelID, botUserIDStr, msgID, parentID, content, postFn)
				attemptCancel()
				if lastErr == nil {
					return
				}
			}
			log.Printf("bot dispatch error: %v", lastErr)
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
	systemPrompt := (&AIConfig{
		PresetVerbosity:   "concise",
		PresetPersonality: "friendly",
		PresetRole:        "assistant",
	}).BuildSystemPrompt()
	var apiKey string

	if cfg != nil {
		provider = cfg.Provider
		model = cfg.Model
		systemPrompt = cfg.BuildSystemPrompt()
		if rawKeyEnc != "" {
			apiKey, err = d.svc.DecryptAPIKey(rawKeyEnc)
			if err != nil {
				return fmt.Errorf("decrypt api key: %w", err)
			}
		}
	}

	// Check monthly compute-credit budget for parley provider
	if provider == "parley" {
		used, err := d.repo.GetMonthlyUsage(ctx, serverIDInt)
		if err != nil {
			return fmt.Errorf("get monthly usage: %w", err)
		}
		if used >= ParleyMonthlyBudget {
			_ = d.repo.SetBotDegraded(ctx, serverIDInt, d.botUserID, true)
			d.broadcastBotStatus(serverIDInt, true)
			return nil // silently skip — over quota
		}
	}

	// Strip bot self-mentions and resolve other user mentions before sending to LLM
	cleanedContent := d.preprocessContent(ctx, content, botUserIDStr)

	// Build conversation context from reply chain
	messages := d.buildMessages(ctx, msgIDStr, parentID, cleanedContent, systemPrompt)

	// Show typing indicator while the LLM call is in flight.
	stopTyping := make(chan struct{})
	go func() {
		d.sendTyping(channelID)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.sendTyping(channelID)
			case <-stopTyping:
				return
			}
		}
	}()

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
		close(stopTyping)
		return fmt.Errorf("unknown provider: %s", provider)
	}
	close(stopTyping)
	if err != nil {
		_ = d.repo.SetBotDegraded(ctx, serverIDInt, d.botUserID, true)
		d.broadcastBotStatus(serverIDInt, true)
		return fmt.Errorf("provider %s: %w", provider, err)
	}
	if reply == "" {
		return nil
	}

	// Successful response — clear any degraded state
	_ = d.repo.SetBotDegraded(ctx, serverIDInt, d.botUserID, false)
	d.broadcastBotStatus(serverIDInt, false)

	// Track compute-credit usage for parley provider (scaled by model cost factor)
	if provider == "parley" && tokensUsed > 0 {
		factor := ParleyModelCostFactor[model]
		if factor == 0 {
			factor = 1
		}
		_ = d.repo.AddTokenUsage(ctx, serverIDInt, tokensUsed*factor)
	}

	// Post reply as a threaded reply to the triggering message
	return postFn(ctx, channelID, botUserIDStr, reply, msgIDStr)
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

	// Most providers (Anthropic, OpenAI, etc.) require the first non-system
	// message to have role "user". When a reply chain starts with a bot message,
	// prepend a synthetic placeholder so the array is always valid.
	firstNonSystem := -1
	for i, m := range msgs {
		if m.Role != "system" {
			firstNonSystem = i
			break
		}
	}
	if firstNonSystem >= 0 && msgs[firstNonSystem].Role == "assistant" {
		placeholder := chatMessage{Role: "user", Content: "…"}
		msgs = append(msgs[:firstNonSystem], append([]chatMessage{placeholder}, msgs[firstNonSystem:]...)...)
	}

	return msgs
}

// sendTyping broadcasts a USER_TYPING event to the given channel so clients show
// the "Polly is typing…" indicator. It is called in a tight goroutine while the
// LLM request is in flight.
func (d *Dispatcher) sendTyping(channelID string) {
	if d.hub == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"channel_id": channelID,
		"user_id":    strconv.FormatInt(d.botUserID, 10),
		"username":   "Polly",
	})
	if err != nil {
		return
	}
	d.hub.BroadcastToChannel(channelID, ws.EventUserTyping, payload)
}

// broadcastBotStatus pushes a BOT_STATUS_UPDATE event to all members of a server.
func (d *Dispatcher) broadcastBotStatus(serverIDInt int64, degraded bool) {
	if d.hub == nil {
		return
	}
	payload, err := json.Marshal(map[string]interface{}{
		"server_id":   strconv.FormatInt(serverIDInt, 10),
		"bot_user_id": strconv.FormatInt(d.botUserID, 10),
		"degraded":    degraded,
	})
	if err != nil {
		return
	}
	d.hub.BroadcastToChannel("server:"+strconv.FormatInt(serverIDInt, 10), ws.EventBotStatusUpdate, payload)
}

// preprocessContent strips bot self-mentions and resolves other user mention tags.
// "@polly" and "<@botID>" are removed; "<@userID>" for other users becomes
// "<@userID> (name: displayname)" using a DB lookup.
func (d *Dispatcher) preprocessContent(ctx context.Context, content, botUserIDStr string) string {
	// Strip bot self-mention forms
	content = strings.ReplaceAll(content, "@polly", "")
	content = strings.ReplaceAll(content, "<@"+botUserIDStr+">", "")

	// Resolve other <@id> mentions
	content = mentionRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := mentionRe.FindStringSubmatch(match)
		if len(sub) < 2 || sub[1] == botUserIDStr {
			return ""
		}
		uid, err := strconv.ParseInt(sub[1], 10, 64)
		if err != nil {
			return match
		}
		name, err := d.repo.GetUserDisplayName(ctx, uid)
		if err != nil || name == "" {
			return match
		}
		return match + " (name: " + name + ")"
	})

	return strings.TrimSpace(content)
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
	// Derive the cloud routing model name:
	//   - Models with a version tag (e.g. "ministral-3:14b") → append "-cloud" → "ministral-3:14b-cloud"
	//   - Models without a tag (e.g. "qwen3.5") → append ":cloud" → "qwen3.5:cloud"
	ollamaModel := model
	if strings.Contains(model, ":") {
		ollamaModel = model + "-cloud"
	} else {
		ollamaModel = model + ":cloud"
	}

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
	googleURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", url.PathEscape(model))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

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
