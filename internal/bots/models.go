// internal/bots/models.go
package bots

import "time"

// Bot represents a bot user summary.
type Bot struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsVerified  bool      `json:"is_verified"`
	AddedAt     time.Time `json:"added_at"`
}

// AIConfig is the per-server AI chatbot configuration.
type AIConfig struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	APIKeySet        bool   `json:"api_key_set"`
	PresetVerbosity  string `json:"preset_verbosity"`
	PresetPersonality string `json:"preset_personality"`
	PresetRole       string `json:"preset_role"`
	UpdatedAt        string `json:"updated_at"`
}

// BuildSystemPrompt constructs the system prompt from the three personality presets.
func (c *AIConfig) BuildSystemPrompt() string {
	role := map[string]string{
		"assistant": "You are a helpful AI assistant. Answer questions and help with tasks.",
		"member":    "You are a casual server member, not just a bot. Engage in conversations naturally, share opinions, and joke around.",
		"tutor":     "You are a patient tutor. Break down complex topics, check for understanding, and guide learners step by step.",
	}[c.PresetRole]
	if role == "" {
		role = "You are a helpful AI assistant."
	}

	personality := map[string]string{
		"friendly":     "Be warm and encouraging.",
		"gamer":        "You are an enthusiastic gamer. Use gaming slang and references naturally.",
		"professional": "Maintain a professional, precise tone.",
		"unhinged":     "You are chaotic, unpredictable, and wildly enthusiastic. Embrace the chaos.",
		"hacker":       "You speak in terse technical jargon, reference hacker culture, drop l33tspeak occasionally, and treat every problem like a systems exploit.",
	}[c.PresetPersonality]
	if personality == "" {
		personality = "Be warm and encouraging."
	}

	verbosity := map[string]string{
		"concise": "Keep responses short and direct.",
		"verbose": "Be thorough and detailed, explaining your reasoning fully.",
	}[c.PresetVerbosity]
	if verbosity == "" {
		verbosity = "Keep responses short and direct."
	}

	return role + " " + personality + " " + verbosity
}

// AIUsage is the current-month token usage for a server.
type AIUsage struct {
	TokensUsed  int64  `json:"tokens_used"`
	TokensLimit int64  `json:"tokens_limit"`
	Model       string `json:"model"`
	ResetsAt    string `json:"resets_at"`
}

// UserBot is a bot owned by the current user, returned by GET /api/bots/mine.
type UserBot struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsVerified  bool   `json:"is_verified"`
	InviteToken string `json:"invite_token"`
}

// BotInviteInfo is returned by GET /api/bots/invite/{token}.
type BotInviteInfo struct {
	BotID       int64  `json:"bot_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsVerified  bool   `json:"is_verified"`
}

// ParleyMonthlyBudget is the total compute-credit budget per server per month.
// All model usage is multiplied by ParleyModelCostFactor before being charged against
// this budget, so larger models consume proportionally more credits per token.
const ParleyMonthlyBudget int64 = 2_000_000

// ParleyModelCostFactor maps each model to its credit cost per actual token.
// The base unit is ministral-3:14b (1×). Factors are rounded to the nearest
// integer and scaled roughly by parameter count relative to the 14B base.
var ParleyModelCostFactor = map[string]int64{
	"ministral-3:14b": 1,
	"gpt-oss:20b":     2,
	"gemma3:27b":      2,
	"gpt-oss:120b":    9,
	"qwen3:latest":    10,
}
