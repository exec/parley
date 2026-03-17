package db

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// ChannelType represents the type of channel
type ChannelType int

const (
	ChannelTypeText  ChannelType = 0
	ChannelTypeVoice ChannelType = 1
	ChannelTypeBin   ChannelType = 2
)

// User represents a user in the system
type User struct {
	ID                      int64      `json:"id" db:"id"`
	Username                string     `json:"username" db:"username"`
	Email                   string     `json:"email" db:"email"`
	PasswordHash            string     `json:"-" db:"password_hash"`
	AvatarURL               string     `json:"avatar_url,omitempty" db:"avatar_url"`
	BannerURL               string     `json:"banner_url,omitempty" db:"banner_url"`
	Bio                     string     `json:"bio,omitempty" db:"bio"`
	DisplayName             string     `json:"display_name" db:"display_name"`
	EmailVerified           bool       `json:"email_verified" db:"email_verified"`
	EmailVerificationToken  string     `json:"-" db:"email_verification_token"`
	PhoneNumber             string     `json:"phone_number,omitempty" db:"phone_number"`
	PhoneVerified           bool       `json:"phone_verified" db:"phone_verified"`
	BannedAt                *time.Time `json:"banned_at,omitempty" db:"banned_at"`
	BanReason               string     `json:"ban_reason,omitempty" db:"ban_reason"`
	ForceLogoutAt           *time.Time `json:"force_logout_at,omitempty" db:"force_logout_at"`
	IsSystem                bool       `json:"is_system" db:"is_system"`
	Badges                  int        `json:"badges" db:"badges"`
	RegistrationIP          string     `json:"registration_ip,omitempty" db:"registration_ip"`
	LastSeenIP              string     `json:"last_seen_ip,omitempty" db:"last_seen_ip"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

// AdminUser represents an admin panel user
type AdminUser struct {
	ID           int64      `json:"id" db:"id"`
	Username     string     `json:"username" db:"username"`
	PasswordHash string     `json:"-" db:"password_hash"`
	Active       bool       `json:"active" db:"active"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
}

// Report represents a user or message report
type Report struct {
	ID                int64      `json:"id" db:"id"`
	ReporterID        *int64     `json:"reporter_id,omitempty" db:"reporter_id"`
	ReportedUserID    *int64     `json:"reported_user_id,omitempty" db:"reported_user_id"`
	ReportedMessageID *int64     `json:"reported_message_id,omitempty" db:"reported_message_id"`
	CategoryID        int64      `json:"category_id" db:"category_id"`
	CategoryName      string     `json:"category_name" db:"-"`
	Description       string     `json:"description" db:"description"`
	Status            string     `json:"status" db:"status"`
	ResolvedBy        *int64     `json:"resolved_by,omitempty" db:"resolved_by"`
	ResolutionNote    string     `json:"resolution_note,omitempty" db:"resolution_note"`
	ReporterUsername  string     `json:"reporter_username,omitempty" db:"-"`
	ReportedUsername  string     `json:"reported_username,omitempty" db:"-"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// ReportCategory represents a category for reports
type ReportCategory struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
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
	ID        int64        `json:"id" db:"id"`
	ServerID  int64        `json:"server_id" db:"server_id"`
	UserID    int64        `json:"user_id" db:"user_id"`
	Nickname  string       `json:"nickname" db:"nickname"`
	JoinedAt  time.Time    `json:"joined_at" db:"joined_at"`
	Username    string       `json:"username" db:"-"`
	DisplayName string       `json:"display_name,omitempty" db:"-"`
	AvatarURL   string       `json:"avatar_url,omitempty" db:"-"`
	BannerURL   string       `json:"banner_url,omitempty" db:"-"`
	Bio         string       `json:"bio,omitempty" db:"-"`
	Badges      int          `json:"badges" db:"-"`
	Roles       []ServerRole `json:"roles" db:"-"`
	IsBot       bool         `json:"is_bot,omitempty" db:"-"`
	BotDegraded bool         `json:"bot_degraded,omitempty" db:"-"`
}

// Channel represents a text or voice channel
type Channel struct {
	ID          int64         `json:"id" db:"id"`
	ServerID    int64         `json:"server_id" db:"server_id"`
	Name        string        `json:"name" db:"name"`
	ChannelType ChannelType   `json:"channel_type" db:"channel_type"`
	Position    int           `json:"position" db:"position"`
	ParentID    sql.NullInt64 `json:"parent_id" db:"parent_id"`
	Topic       string        `json:"topic,omitempty" db:"topic"`
	Synced      bool          `json:"synced" db:"synced"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at" db:"updated_at"`
}

// Message represents a message in a channel
type Message struct {
	ID              int64     `json:"id" db:"id"`
	ChannelID       int64     `json:"channel_id" db:"channel_id"`
	AuthorID        int64     `json:"author_id" db:"author_id"`
	Content         string    `json:"content" db:"content"`
	Nonce           string    `json:"nonce" db:"nonce"`
	ParentID        *int64    `json:"parent_id,omitempty" db:"parent_id"`
	AttachmentURL   string    `json:"attachment_url,omitempty" db:"attachment_url"`
	AttachmentName  string    `json:"attachment_name,omitempty" db:"attachment_name"`
	AttachmentType  string    `json:"attachment_type,omitempty" db:"attachment_type"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	AuthorUsername          string    `json:"author_username" db:"-"`
	AuthorDisplayName       string    `json:"author_display_name" db:"-"`
	AuthorAvatarURL         string    `json:"author_avatar_url,omitempty" db:"-"`
	AuthorIsBot             bool      `json:"author_is_bot,omitempty"`
	ViaApi                  bool      `json:"via_api,omitempty"`
	ParentAuthorUsername    string    `json:"parent_author_username,omitempty" db:"-"`
	ParentAuthorDisplayName string    `json:"parent_author_display_name,omitempty" db:"-"`
}

// DmChannel represents a direct message channel between two users
type DmChannel struct {
	ID            int64     `json:"id" db:"id"`
	User1ID       int64     `json:"user1_id" db:"user1_id"`
	User2ID       int64     `json:"user2_id" db:"user2_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	OtherUsername  string    `json:"other_username" db:"-"`
	OtherUserID    int64     `json:"other_user_id" db:"-"`
	OtherAvatarURL string    `json:"other_avatar_url,omitempty" db:"-"`
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
	AuthorUsername    string    `json:"author_username" db:"-"`
	AuthorDisplayName string    `json:"author_display_name" db:"-"`
	AuthorAvatarURL   string    `json:"author_avatar_url,omitempty" db:"-"`
}

// PublicUser represents public user profile info
type PublicUser struct {
	ID          int64     `json:"id" db:"id"`
	Username    string    `json:"username" db:"username"`
	DisplayName string    `json:"display_name,omitempty" db:"display_name"`
	AvatarURL   string    `json:"avatar_url" db:"avatar_url"`
	BannerURL   string    `json:"banner_url" db:"banner_url"`
	Bio         string    `json:"bio,omitempty" db:"bio"`
	Badges      int       `json:"badges" db:"badges"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// Badge bit constants
const (
	BadgeDonor int = 1 << 0 // 1 — financial supporter
	BadgeAdmin int = 1 << 1 // 2 — Parley staff/admin
)

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
	Hoist       bool      `json:"hoist" db:"hoist"`
	Position    int       `json:"position" db:"position"`
	IsEveryone  bool      `json:"is_everyone" db:"is_everyone"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// APIKeyInfo is an API key enriched with bot info for display
type APIKeyInfo struct {
	ID          int64      `json:"id"`
	KeyPrefix   string     `json:"key_prefix"`
	UserID      int64      `json:"user_id"`
	OwnerID     int64      `json:"owner_id"`
	Name        string     `json:"name"`
	IsBot       bool       `json:"is_bot"`
	BotUsername string     `json:"bot_username,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// BinPost represents a post in a bin channel.
type BinPost struct {
	ID              int64          `json:"id" db:"id"`
	ChannelID       int64          `json:"channel_id" db:"channel_id"`
	ThreadChannelID int64          `json:"thread_channel_id" db:"thread_channel_id"`
	AuthorID        int64          `json:"author_id" db:"author_id"`
	Title           string         `json:"title" db:"title"`
	Description     string         `json:"description" db:"description"`
	Tags            pq.StringArray `json:"tags" db:"tags"`
	CreatedAt       time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at" db:"updated_at"`
	// Computed fields
	AuthorUsername    string        `json:"author_username" db:"-"`
	AuthorAvatarURL  string        `json:"author_avatar_url,omitempty" db:"-"`
	Files            []BinPostFile `json:"files,omitempty" db:"-"`
	CommentCount     int           `json:"comment_count" db:"-"`
	LineCommentCount int           `json:"line_comment_count" db:"-"`
	VersionCount     int           `json:"version_count" db:"-"`
}

// BinPostFile represents a code file attached to a bin post.
type BinPostFile struct {
	ID       int64  `json:"id" db:"id"`
	PostID   int64  `json:"post_id" db:"post_id"`
	Filename string `json:"filename" db:"filename"`
	Language string `json:"language" db:"language"`
	Content  string `json:"content" db:"content"`
	Position int    `json:"position" db:"position"`
}

// BinPostVersion represents a version snapshot of a bin post.
type BinPostVersion struct {
	ID          int64     `json:"id" db:"id"`
	PostID      int64     `json:"post_id" db:"post_id"`
	Version     int       `json:"version" db:"version"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	Files []BinPostVersionFile `json:"files,omitempty" db:"-"`
}

// BinPostVersionFile represents a file snapshot within a version.
type BinPostVersionFile struct {
	ID        int64  `json:"id" db:"id"`
	VersionID int64  `json:"version_id" db:"version_id"`
	Filename  string `json:"filename" db:"filename"`
	Language  string `json:"language" db:"language"`
	Content   string `json:"content" db:"content"`
	Position  int    `json:"position" db:"position"`
}

// BinLineComment represents a comment anchored to a specific line in a file version.
type BinLineComment struct {
	ID         int64     `json:"id" db:"id"`
	PostID     int64     `json:"post_id" db:"post_id"`
	VersionID  int64     `json:"version_id" db:"version_id"`
	FileID     int64     `json:"file_id" db:"file_id"`
	LineNumber int       `json:"line_number" db:"line_number"`
	AuthorID   int64     `json:"author_id" db:"author_id"`
	Content    string    `json:"content" db:"content"`
	ParentID   *int64    `json:"parent_id,omitempty" db:"parent_id"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
	AuthorUsername  string `json:"author_username" db:"-"`
	AuthorAvatarURL string `json:"author_avatar_url,omitempty" db:"-"`
}

// Overwrite represents a permission overwrite for a channel (role or member).
// TargetType: 0 = role, 1 = member.
type Overwrite struct {
	TargetType int
	TargetID   int64
	Allow      int64
	Deny       int64
}

// BinChannelTag represents an admin-defined tag for a bin channel.
type BinChannelTag struct {
	ID        int64  `json:"id" db:"id"`
	ChannelID int64  `json:"channel_id" db:"channel_id"`
	Name      string `json:"name" db:"name"`
	Color     string `json:"color" db:"color"`
}