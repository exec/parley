package db

import (
	"database/sql"
	"time"
)

// ChannelType represents the type of channel
type ChannelType int

const (
	ChannelTypeText  ChannelType = 0
	ChannelTypeVoice ChannelType = 1
)

// User represents a user in the system
type User struct {
	ID           int64     `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	AvatarURL    string    `json:"avatar_url,omitempty" db:"avatar_url"`
	BannerURL    string    `json:"banner_url,omitempty" db:"banner_url"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// Server represents a Discord server/guild
type Server struct {
	ID        int64          `json:"id" db:"id"`
	Name      string         `json:"name" db:"name"`
	IconURL   sql.NullString `json:"icon_url" db:"icon_url"`
	OwnerID   int64          `json:"owner_id" db:"owner_id"`
	VanityURL sql.NullString `json:"vanity_url" db:"vanity_url"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

// ServerMember represents a member of a server
type ServerMember struct {
	ID       int64        `json:"id" db:"id"`
	ServerID int64        `json:"server_id" db:"server_id"`
	UserID   int64        `json:"user_id" db:"user_id"`
	Nickname string       `json:"nickname" db:"nickname"`
	JoinedAt time.Time    `json:"joined_at" db:"joined_at"`
	Username string       `json:"username" db:"-"`
	Roles    []ServerRole `json:"roles" db:"-"`
}

// Channel represents a text or voice channel
type Channel struct {
	ID          int64         `json:"id" db:"id"`
	ServerID    int64         `json:"server_id" db:"server_id"`
	Name        string        `json:"name" db:"name"`
	ChannelType ChannelType   `json:"channel_type" db:"channel_type"`
	Position    int           `json:"position" db:"position"`
	ParentID    sql.NullInt64 `json:"parent_id" db:"parent_id"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at" db:"updated_at"`
}

// Message represents a message in a channel
type Message struct {
	ID             int64     `json:"id" db:"id"`
	ChannelID      int64     `json:"channel_id" db:"channel_id"`
	AuthorID       int64     `json:"author_id" db:"author_id"`
	Content        string    `json:"content" db:"content"`
	Nonce          string    `json:"nonce" db:"nonce"`
	AttachmentURL  string    `json:"attachment_url,omitempty" db:"attachment_url"`
	AttachmentName string    `json:"attachment_name,omitempty" db:"attachment_name"`
	AttachmentType string    `json:"attachment_type,omitempty" db:"attachment_type"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	AuthorUsername string    `json:"author_username" db:"-"`
}

// DmChannel represents a direct message channel between two users
type DmChannel struct {
	ID            int64     `json:"id" db:"id"`
	User1ID       int64     `json:"user1_id" db:"user1_id"`
	User2ID       int64     `json:"user2_id" db:"user2_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	OtherUsername string    `json:"other_username" db:"-"`
	OtherUserID   int64     `json:"other_user_id" db:"-"`
}

// DmMessage represents a direct message
type DmMessage struct {
	ID             int64     `json:"id" db:"id"`
	DmChannelID    int64     `json:"dm_channel_id" db:"dm_channel_id"`
	AuthorID       int64     `json:"author_id" db:"author_id"`
	Content        string    `json:"content" db:"content"`
	AttachmentURL  string    `json:"attachment_url,omitempty" db:"attachment_url"`
	AttachmentName string    `json:"attachment_name,omitempty" db:"attachment_name"`
	AttachmentType string    `json:"attachment_type,omitempty" db:"attachment_type"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	AuthorUsername string    `json:"author_username" db:"-"`
}

// PublicUser represents public user profile info
type PublicUser struct {
	ID        int64     `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	AvatarURL string    `json:"avatar_url" db:"avatar_url"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ReactionGroup represents aggregated reactions for a message, grouped by emoji
type ReactionGroup struct {
	Emoji   string   `json:"emoji"`
	Count   int      `json:"count"`
	UserIDs []string `json:"user_ids"`
}

// Invite represents an invite code for a server
type Invite struct {
	ID        int64     `json:"id" db:"id"`
	ServerID  int64     `json:"server_id" db:"server_id"`
	Code      string    `json:"code" db:"code"`
	CreatedBy int64     `json:"created_by" db:"created_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ServerRole represents a role within a server
type ServerRole struct {
	ID          int64     `json:"id" db:"id"`
	ServerID    int64     `json:"server_id" db:"server_id"`
	Name        string    `json:"name" db:"name"`
	Color       string    `json:"color" db:"color"`
	Permissions int64     `json:"permissions" db:"permissions"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}