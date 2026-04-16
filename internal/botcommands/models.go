// Package botcommands implements slash commands (Phase 1 MVP).
//
// A bot registers one or more /commands per-server. A user invokes a command,
// which creates a bot_interactions row and pushes INTERACTION_CREATE over the
// bot's WebSocket connection. The bot then POSTs to /api/interactions/{token}/respond
// with a text body, which creates a message in the channel tagged with
// kind='interaction_response'.
package botcommands

import "time"

// BotCommand is a single registered slash command scoped to (bot_id, server_id, name).
// BotUsername/BotDisplayName/BotAvatarURL are populated only on the user-facing
// list endpoint (GET /servers/{id}/commands) so the autocomplete dropdown can show
// which bot owns each command. They stay empty on the bot-facing endpoints and
// are omitted from JSON via `omitempty`.
type BotCommand struct {
	ID             int64              `json:"id"`
	BotID          int64              `json:"bot_id"`
	ServerID       int64              `json:"server_id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Options        []BotCommandOption `json:"options"`
	BotUsername    string             `json:"bot_username,omitempty"`
	BotDisplayName string             `json:"bot_display_name,omitempty"`
	BotAvatarURL   string             `json:"bot_avatar_url,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// BotCommandOption describes a single argument accepted by a command.
// Type is one of "STRING", "INTEGER", "BOOLEAN" in Phase 1.
// MinValue/MaxValue apply to INTEGER options; MinLength/MaxLength apply to STRING options.
// Choices is an optional, mutually-exclusive enumeration of allowed values
// (STRING/INTEGER only, ignored for BOOLEAN).
type BotCommandOption struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	Required    bool           `json:"required,omitempty"`
	Choices     []OptionChoice `json:"choices,omitempty"`
	MinValue    *float64       `json:"min_value,omitempty"`
	MaxValue    *float64       `json:"max_value,omitempty"`
	MinLength   *int           `json:"min_length,omitempty"`
	MaxLength   *int           `json:"max_length,omitempty"`
}

// OptionChoice is an enum entry for STRING/INTEGER options. Value must be the
// same JSON kind as the option's Type (string for STRING, number for INTEGER).
type OptionChoice struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// BotInteraction is one invocation of a registered command.
type BotInteraction struct {
	Token             string                 `json:"token"`
	BotID             int64                  `json:"bot_id"`
	CommandID         int64                  `json:"command_id"`
	InvokerUserID     int64                  `json:"invoker_user_id"`
	ChannelID         int64                  `json:"channel_id"`
	ServerID          int64                  `json:"server_id"`
	Options           map[string]interface{} `json:"options"`
	State             string                 `json:"state"`
	ResponseMessageID *int64                 `json:"response_message_id,omitempty"`
	ExpiresAt         time.Time              `json:"expires_at"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// Interaction state values.
const (
	StatePending   = "pending"
	StateResponded = "responded"
	StateExpired   = "expired"
)

// Option type values.
const (
	OptionTypeString  = "STRING"
	OptionTypeInteger = "INTEGER"
	OptionTypeBoolean = "BOOLEAN"
)

// Message kind values used this phase.
const (
	MessageKindNormal              = "normal"
	MessageKindInteractionResponse = "interaction_response"
	MessageKindSystem              = "system"
)

// InteractionLifetime is how long a pending interaction remains valid.
const InteractionLifetime = 15 * time.Minute

// ============ HTTP DTOs ============

// RegisterCommandRequest is the body accepted by POST/PUT /commands.
// When PUT is called, the body is an array of these.
type RegisterCommandRequest struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Options     []BotCommandOption `json:"options,omitempty"`
}

// InvokeInteractionRequest is the body for POST /api/channels/{channelID}/interactions.
type InvokeInteractionRequest struct {
	CommandID int64                  `json:"command_id"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// InvokeInteractionResponse is returned to the frontend after a successful invoke.
type InvokeInteractionResponse struct {
	InteractionID string    `json:"interaction_id"`
	Status        string    `json:"status"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// RespondRequest is the body POSTed by the bot to /api/interactions/{token}/respond.
type RespondRequest struct {
	Content string `json:"content"`
}

// RespondResponse is returned to the bot after a successful respond.
type RespondResponse struct {
	MessageID int64 `json:"message_id"`
}

// InteractionCreatePayload is the WebSocket event payload delivered to the bot
// when a user invokes one of its commands.
type InteractionCreatePayload struct {
	Token     string                   `json:"token"`
	Command   InteractionCreateCommand `json:"command"`
	Options   map[string]interface{}   `json:"options"`
	Invoker   InteractionInvoker       `json:"invoker"`
	ChannelID int64                    `json:"channel_id"`
	ServerID  int64                    `json:"server_id"`
	CreatedAt time.Time                `json:"created_at"`
	ExpiresAt time.Time                `json:"expires_at"`
}

// InteractionCreateCommand is the command summary embedded in INTERACTION_CREATE.
type InteractionCreateCommand struct {
	ID      int64              `json:"id"`
	Name    string             `json:"name"`
	Options []BotCommandOption `json:"options"`
}

// InteractionInvoker is the user who invoked the command.
type InteractionInvoker struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}
