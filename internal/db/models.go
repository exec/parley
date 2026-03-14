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
	ID           int64          `json:"id" db:"id"`
	Username     string         `json:"username" db:"username"`
	Email        string         `json:"email" db:"email"`
	PasswordHash string         `json:"-" db:"password_hash"`
	AvatarURL    sql.NullString `json:"avatar_url" db:"avatar_url"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
}

// Server represents a Discord server/guild
type Server struct {
	ID        int64          `json:"id" db:"id"`
	Name      string         `json:"name" db:"name"`
	IconURL   sql.NullString `json:"icon_url" db:"icon_url"`
	OwnerID   int64          `json:"owner_id" db:"owner_id"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

// ServerMember represents a member of a server
type ServerMember struct {
	ID       int64     `json:"id" db:"id"`
	ServerID int64     `json:"server_id" db:"server_id"`
	UserID   int64     `json:"user_id" db:"user_id"`
	Nickname string    `json:"nickname" db:"nickname"`
	JoinedAt time.Time `json:"joined_at" db:"joined_at"`
	Username string    `json:"username" db:"-"`
}

// Channel represents a text or voice channel
type Channel struct {
	ID         int64          `json:"id" db:"id"`
	ServerID   int64          `json:"server_id" db:"server_id"`
	Name       string         `json:"name" db:"name"`
	ChannelType ChannelType   `json:"channel_type" db:"channel_type"`
	Position   int            `json:"position" db:"position"`
	ParentID   sql.NullInt64  `json:"parent_id" db:"parent_id"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
}

// Message represents a message in a channel
type Message struct {
	ID             int64     `json:"id" db:"id"`
	ChannelID      int64     `json:"channel_id" db:"channel_id"`
	AuthorID       int64     `json:"author_id" db:"author_id"`
	Content        string    `json:"content" db:"content"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	AuthorUsername string    `json:"author_username" db:"-"`
}