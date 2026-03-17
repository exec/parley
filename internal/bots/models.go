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
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	APIKeySet    bool   `json:"api_key_set"`
	SystemPrompt string `json:"system_prompt"`
	UpdatedAt    string `json:"updated_at"`
}

// AIUsage is the current-month token usage for a server.
type AIUsage struct {
	TokensUsed  int64  `json:"tokens_used"`
	TokensLimit int64  `json:"tokens_limit"`
	Model       string `json:"model"`
	ResetsAt    string `json:"resets_at"`
}

// BotInviteInfo is returned by GET /api/bots/invite/{token}.
type BotInviteInfo struct {
	BotID       int64  `json:"bot_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsVerified  bool   `json:"is_verified"`
}

// ParleyModelAllowances maps stored model value → monthly token allowance.
var ParleyModelAllowances = map[string]int64{
	"ministral-3:14b": 2_000_000,
	"gpt-oss:20b":     1_500_000,
	"gemma3:27b":      1_000_000,
	"gpt-oss:120b":    300_000,
	"qwen3:latest":    100_000,
}
