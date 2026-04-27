// Package account collects user-account-lifecycle endpoints (data export and
// self-serve deletion) into one place. The export side assembles a single
// in-memory JSON document with everything a user has authored or been the
// subject of, satisfying the GDPR right to data portability.
package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"

	"parley/internal/db"
	"parley/internal/friend"
)

// ExportFormatVersion is bumped whenever the export envelope shape changes
// in a way that downstream parsers need to detect (e.g. a renamed top-level
// field). Additive changes do not require a bump.
const ExportFormatVersion = 1

// ExportEnvelope is the top-level JSON object returned by GET /api/me/export.
// The shape is locked in
// docs/superpowers/specs/2026-04-26-account-deletion-and-data-export-design.md.
type ExportEnvelope struct {
	ExportedAt              time.Time            `json:"exported_at"`
	FormatVersion           int                  `json:"format_version"`
	Profile                 *ExportProfile       `json:"profile"`
	Passkeys                []ExportPasskey      `json:"passkeys"`
	Friends                 []friend.FriendUser  `json:"friends"`
	BlockedUsers            []friend.FriendUser  `json:"blocked_users"`
	OwnedServers            []ExportServerSummary `json:"owned_servers"`
	OwnedBots               []ExportBotSummary    `json:"owned_bots"`
	ThemesPublished         []ExportTheme         `json:"themes_published"`
	MessagesAuthored        ExportMessagesAuthored `json:"messages_authored"`
	DmChannelsMemberOf      []ExportDmChannel      `json:"dm_channels_member_of"`
	NotificationsReceived   []db.Notification      `json:"notifications_received"`
	AuditLogEntriesAboutMe  []ExportAuditLogEntry  `json:"audit_log_entries_about_me"`
}

// ExportProfile is db.User minus password_hash and any verification token /
// secret material. Built by copying explicit fields out of *db.User; this is
// the spec-mandated approach (do NOT mutate the User pointer to zero its
// PasswordHash, since that User can flow back into other code paths).
type ExportProfile struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email,omitempty"`
	AvatarURL     string     `json:"avatar_url,omitempty"`
	BannerURL     string     `json:"banner_url,omitempty"`
	Bio           string     `json:"bio,omitempty"`
	DisplayName   string     `json:"display_name,omitempty"`
	EmailVerified bool       `json:"email_verified"`
	PhoneNumber   string     `json:"phone_number,omitempty"`
	PhoneVerified bool       `json:"phone_verified"`
	IsSystem      bool       `json:"is_system"`
	Badges        int        `json:"badges"`
	StatusType    string     `json:"status_type,omitempty"`
	StatusText    string     `json:"status_text,omitempty"`
	InviteCount   int        `json:"invite_count"`
	BannedAt      *time.Time `json:"banned_at,omitempty"`
	BanReason     string     `json:"ban_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// profileFromUser copies the safe subset out of u. password_hash,
// email_verification_token, phone_verification_code, password reset tokens,
// and force_logout_at (operational state, not user data) are deliberately
// not copied. This is the only place in the codebase that should serialize
// a User's full credential surface, so the allowlist lives here.
func profileFromUser(u *db.User) *ExportProfile {
	if u == nil {
		return nil
	}
	return &ExportProfile{
		ID:            u.ID,
		Username:      u.Username,
		Email:         u.Email,
		AvatarURL:     u.AvatarURL,
		BannerURL:     u.BannerURL,
		Bio:           u.Bio,
		DisplayName:   u.DisplayName,
		EmailVerified: u.EmailVerified,
		PhoneNumber:   u.PhoneNumber,
		PhoneVerified: u.PhoneVerified,
		IsSystem:      u.IsSystem,
		Badges:        u.Badges,
		StatusType:    u.StatusType,
		StatusText:    u.StatusText,
		InviteCount:   u.InviteCount,
		BannedAt:      u.BannedAt,
		BanReason:     u.BanReason,
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

// ExportPasskey is the credential-metadata projection. Public-key bytes,
// AAGUID, sign_count, and backup flags are intentionally excluded — they
// are not useful to the user and revealing them would let a passive
// observer of the export fingerprint the user's authenticator hardware.
type ExportPasskey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ExportServerSummary is the {id, name, created_at} projection requested by
// the spec for owned_servers — the user's own message rows in those
// servers' channels are already exported separately under messages_authored.
type ExportServerSummary struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportBotSummary is the {id, username, created_at} projection requested by
// the spec for owned_bots.
type ExportBotSummary struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportTheme is the full user_themes row a user published. Includes the CSS
// content; share_token is preserved so an exported theme can be re-imported
// elsewhere.
type ExportTheme struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	CSS              string    `json:"css"`
	BaseTheme        string    `json:"base_theme"`
	BackgroundURL    *string   `json:"background_url,omitempty"`
	ShareToken       *string   `json:"share_token,omitempty"`
	IsPublished      bool      `json:"is_published"`
	IsFeatured       bool      `json:"is_featured"`
	SourceShareToken *string   `json:"source_share_token,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ExportMessage is the export projection of a server-channel message: locator
// fields (channel_id, server_id) are included so a recipient can find the
// message in context, and forwarded_data is kept verbatim because forwarded
// content is part of what the user actually sent.
type ExportMessage struct {
	ID             int64                    `json:"id"`
	ChannelID      int64                    `json:"channel_id"`
	ServerID       int64                    `json:"server_id"`
	Content        string                   `json:"content"`
	ParentID       *int64                   `json:"parent_id,omitempty"`
	AttachmentURL  string                   `json:"attachment_url,omitempty"`
	AttachmentName string                   `json:"attachment_name,omitempty"`
	AttachmentType string                   `json:"attachment_type,omitempty"`
	ForwardedData  *json.RawMessage         `json:"forwarded_data,omitempty"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

// ExportDmMessage is the DM analogue of ExportMessage. dm_channel_id is the
// locator; system_event is included verbatim for system-event rows in group
// DMs (NULL for normal messages).
type ExportDmMessage struct {
	ID             int64            `json:"id"`
	DmChannelID    int64            `json:"dm_channel_id"`
	Content        string           `json:"content"`
	ParentID       *int64           `json:"parent_id,omitempty"`
	AttachmentURL  string           `json:"attachment_url,omitempty"`
	AttachmentName string           `json:"attachment_name,omitempty"`
	AttachmentType string           `json:"attachment_type,omitempty"`
	ForwardedData  *json.RawMessage `json:"forwarded_data,omitempty"`
	SystemEvent    *json.RawMessage `json:"system_event,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

// ExportBinPost is a bin post as authored by the user. Files are serialized
// inline since they are the actual user content.
type ExportBinPost struct {
	ID          int64               `json:"id"`
	ChannelID   int64               `json:"channel_id"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Tags        []string            `json:"tags,omitempty"`
	Files       []ExportBinPostFile `json:"files,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// ExportBinPostFile mirrors bin_post_files. Position is preserved so file
// ordering can be reconstructed.
type ExportBinPostFile struct {
	Filename string `json:"filename"`
	Language string `json:"language,omitempty"`
	Content  string `json:"content"`
	Position int    `json:"position"`
}

// ExportBinLineComment is a bin line comment authored by the user. post_id +
// version_id + file_id + line_number form the locator.
type ExportBinLineComment struct {
	ID         int64     `json:"id"`
	PostID     int64     `json:"post_id"`
	VersionID  int64     `json:"version_id"`
	FileID     int64     `json:"file_id"`
	LineNumber int       `json:"line_number"`
	Content    string    `json:"content"`
	ParentID   *int64    `json:"parent_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ExportMessagesAuthored buckets the four authored-content surfaces. Empty
// arrays (not null) so JSON consumers always get a list.
type ExportMessagesAuthored struct {
	ServerChannels   []ExportMessage        `json:"server_channels"`
	DMs              []ExportDmMessage      `json:"dms"`
	BinPosts         []ExportBinPost        `json:"bin_posts"`
	BinLineComments  []ExportBinLineComment `json:"bin_line_comments"`
}

// ExportDmChannel is the per-channel summary the user is/was a member of.
// is_group + name distinguish 1:1 from group DMs; created_at is the channel's
// creation time, not the user's join time.
type ExportDmChannel struct {
	ID        int64     `json:"id"`
	IsGroup   bool      `json:"is_group"`
	Name      *string   `json:"name,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ExportAuditLogEntry is a row from server_audit_logs where the user is the
// target. server_audit_logs.target_id is TEXT (not user_id) — the spec's
// "target_user_id" is realised as (target_type='user' AND target_id=uid::text).
type ExportAuditLogEntry struct {
	ID            int64           `json:"id"`
	ServerID      int64           `json:"server_id"`
	ActorID       *int64          `json:"actor_id,omitempty"`
	ActorUsername string          `json:"actor_username,omitempty"`
	Action        string          `json:"action"`
	TargetID      string          `json:"target_id,omitempty"`
	TargetType    string          `json:"target_type,omitempty"`
	TargetName    string          `json:"target_name,omitempty"`
	Changes       json.RawMessage `json:"changes,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// ExportService assembles the ExportEnvelope for a user. Holds *sql.DB
// directly (no caching layer) — the export is one-shot and per-user, so
// repository caches would just churn for nothing.
type ExportService struct {
	repo *db.Repository
	db   *sql.DB
}

// NewExportService wires the export service. The hub/spaces aren't needed —
// nothing about export fans out events or touches object storage.
func NewExportService(repo *db.Repository) *ExportService {
	return &ExportService{repo: repo, db: repo.DB()}
}

// BuildExport collects every export section for userID into a single envelope.
// Errors from any section short-circuit the entire request — partial exports
// would silently lose data and that is worse than a retry.
func (s *ExportService) BuildExport(ctx context.Context, userID int64) (*ExportEnvelope, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}

	passkeys, err := s.collectPasskeys(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load passkeys: %w", err)
	}

	friends, err := s.collectFriends(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load friends: %w", err)
	}

	blocks, err := s.collectBlocks(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load blocks: %w", err)
	}

	servers, err := s.collectOwnedServers(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load owned servers: %w", err)
	}

	bots, err := s.collectOwnedBots(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load owned bots: %w", err)
	}

	themes, err := s.collectThemes(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load themes: %w", err)
	}

	channelMsgs, err := s.collectAuthoredChannelMessages(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load channel messages: %w", err)
	}

	dmMsgs, err := s.collectAuthoredDmMessages(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load dm messages: %w", err)
	}

	binPosts, err := s.collectAuthoredBinPosts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load bin posts: %w", err)
	}

	binLineComments, err := s.collectAuthoredBinLineComments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load bin line comments: %w", err)
	}

	dmChannels, err := s.collectDmChannelMembership(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load dm membership: %w", err)
	}

	notifications, err := s.collectNotifications(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load notifications: %w", err)
	}

	audit, err := s.collectAuditLogTargetingUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load audit log: %w", err)
	}

	return &ExportEnvelope{
		ExportedAt:    time.Now().UTC(),
		FormatVersion: ExportFormatVersion,
		Profile:       profileFromUser(user),
		Passkeys:      passkeys,
		Friends:       friends,
		BlockedUsers:  blocks,
		OwnedServers:  servers,
		OwnedBots:     bots,
		ThemesPublished: themes,
		MessagesAuthored: ExportMessagesAuthored{
			ServerChannels:  channelMsgs,
			DMs:             dmMsgs,
			BinPosts:        binPosts,
			BinLineComments: binLineComments,
		},
		DmChannelsMemberOf:     dmChannels,
		NotificationsReceived:  notifications,
		AuditLogEntriesAboutMe: audit,
	}, nil
}

func (s *ExportService) collectPasskeys(ctx context.Context, userID int64) ([]ExportPasskey, error) {
	rows, err := s.repo.GetPasskeysByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ExportPasskey, 0, len(rows))
	for _, p := range rows {
		out = append(out, ExportPasskey{
			ID:         p.ID,
			Name:       p.Name,
			CreatedAt:  p.CreatedAt,
			LastUsedAt: p.LastUsedAt,
		})
	}
	return out, nil
}

func (s *ExportService) collectFriends(ctx context.Context, userID int64) ([]friend.FriendUser, error) {
	// Inline the friend.GetFriends query rather than constructing a
	// friend.Service just to call one method — the export package would
	// otherwise pull a websocket dependency it has no use for.
	return queryFriendUsers(ctx, s.db, `
		SELECT
			CASE WHEN fr.sender_id = $1 THEN fr.receiver_id ELSE fr.sender_id END AS friend_id,
			u.username,
			COALESCE(u.display_name, '') AS display_name,
			COALESCE(u.avatar_url, '') AS avatar_url
		FROM friend_requests fr
		JOIN users u ON u.id = CASE WHEN fr.sender_id = $1 THEN fr.receiver_id ELSE fr.sender_id END
		WHERE (fr.sender_id = $1 OR fr.receiver_id = $1) AND fr.status = 'accepted'
		ORDER BY u.username
	`, userID)
}

func (s *ExportService) collectBlocks(ctx context.Context, userID int64) ([]friend.FriendUser, error) {
	return queryFriendUsers(ctx, s.db, `
		SELECT u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.avatar_url,'')
		FROM user_blocks b
		JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1
		ORDER BY u.username
	`, userID)
}

// queryFriendUsers runs a 4-column query (id, username, display_name,
// avatar_url) and adapts each row into a friend.FriendUser. id is stringified
// to match the wire shape friend.Service uses.
func queryFriendUsers(ctx context.Context, dbh *sql.DB, query string, args ...any) ([]friend.FriendUser, error) {
	rows, err := dbh.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []friend.FriendUser{}
	for rows.Next() {
		var (
			id          int64
			username    string
			displayName string
			avatarURL   string
		)
		if err := rows.Scan(&id, &username, &displayName, &avatarURL); err != nil {
			return nil, err
		}
		out = append(out, friend.FriendUser{
			ID:          fmt.Sprintf("%d", id),
			Username:    username,
			DisplayName: displayName,
			AvatarURL:   avatarURL,
		})
	}
	return out, rows.Err()
}

func (s *ExportService) collectOwnedServers(ctx context.Context, userID int64) ([]ExportServerSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM servers WHERE owner_id = $1 ORDER BY created_at`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportServerSummary{}
	for rows.Next() {
		var srv ExportServerSummary
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *ExportService) collectOwnedBots(ctx context.Context, userID int64) ([]ExportBotSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, created_at FROM users WHERE bot_owner_id = $1 ORDER BY created_at`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportBotSummary{}
	for rows.Next() {
		var b ExportBotSummary
		if err := rows.Scan(&b.ID, &b.Username, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *ExportService) collectThemes(ctx context.Context, userID int64) ([]ExportTheme, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(css,''), COALESCE(base_theme,'rory'),
		       background_url, share_token::text, source_share_token::text,
		       is_published, is_featured, created_at
		FROM user_themes
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportTheme{}
	for rows.Next() {
		var (
			t                                    ExportTheme
			bgURL, shareToken, sourceShareToken sql.NullString
		)
		if err := rows.Scan(&t.ID, &t.Name, &t.CSS, &t.BaseTheme,
			&bgURL, &shareToken, &sourceShareToken,
			&t.IsPublished, &t.IsFeatured, &t.CreatedAt); err != nil {
			return nil, err
		}
		if bgURL.Valid {
			v := bgURL.String
			t.BackgroundURL = &v
		}
		if shareToken.Valid {
			v := shareToken.String
			t.ShareToken = &v
		}
		if sourceShareToken.Valid {
			v := sourceShareToken.String
			t.SourceShareToken = &v
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *ExportService) collectAuthoredChannelMessages(ctx context.Context, userID int64) ([]ExportMessage, error) {
	// Join channels for server_id; LEFT JOIN handles the (rare) case of a
	// channel deleted out from under the message — the row should still
	// appear in the export with server_id=0 rather than being silently
	// dropped.
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.channel_id, COALESCE(c.server_id, 0),
		       m.content, m.parent_id,
		       COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
		       m.forwarded_data,
		       m.created_at, m.updated_at
		FROM messages m
		LEFT JOIN channels c ON c.id = m.channel_id
		WHERE m.author_id = $1
		ORDER BY m.created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportMessage{}
	for rows.Next() {
		var (
			m         ExportMessage
			parentID  sql.NullInt64
			forwarded []byte
		)
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.ServerID,
			&m.Content, &parentID,
			&m.AttachmentURL, &m.AttachmentName, &m.AttachmentType,
			&forwarded,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			v := parentID.Int64
			m.ParentID = &v
		}
		if len(forwarded) > 0 {
			raw := json.RawMessage(forwarded)
			m.ForwardedData = &raw
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *ExportService) collectAuthoredDmMessages(ctx context.Context, userID int64) ([]ExportDmMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, dm_channel_id, content, parent_id,
		       COALESCE(attachment_url,''), COALESCE(attachment_name,''), COALESCE(attachment_type,''),
		       forwarded_data, system_event,
		       created_at, updated_at
		FROM dm_messages
		WHERE author_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportDmMessage{}
	for rows.Next() {
		var (
			m                       ExportDmMessage
			parentID                sql.NullInt64
			forwarded, systemEvent []byte
		)
		if err := rows.Scan(&m.ID, &m.DmChannelID, &m.Content, &parentID,
			&m.AttachmentURL, &m.AttachmentName, &m.AttachmentType,
			&forwarded, &systemEvent,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			v := parentID.Int64
			m.ParentID = &v
		}
		if len(forwarded) > 0 {
			raw := json.RawMessage(forwarded)
			m.ForwardedData = &raw
		}
		if len(systemEvent) > 0 {
			raw := json.RawMessage(systemEvent)
			m.SystemEvent = &raw
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *ExportService) collectAuthoredBinPosts(ctx context.Context, userID int64) ([]ExportBinPost, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, channel_id, title, COALESCE(description,''), COALESCE(tags, '{}'),
		       created_at, updated_at
		FROM bin_posts
		WHERE author_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := []ExportBinPost{}
	for rows.Next() {
		var (
			p    ExportBinPost
			tags pq.StringArray
		)
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.Title, &p.Description, &tags,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Tags = []string(tags)
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Pull files for each post in a second pass. Bin posts almost always
	// have at least one file; skipping this loop would lose the actual
	// pasted code, which is the entire point of the post. N+1 here is
	// acceptable since user-owned post counts are bounded in practice.
	for i := range posts {
		files, err := s.collectBinPostFiles(ctx, posts[i].ID)
		if err != nil {
			return nil, err
		}
		posts[i].Files = files
	}
	return posts, nil
}

func (s *ExportService) collectBinPostFiles(ctx context.Context, postID int64) ([]ExportBinPostFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT filename, COALESCE(language,''), COALESCE(content,''), position
		FROM bin_post_files
		WHERE post_id = $1
		ORDER BY position
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportBinPostFile{}
	for rows.Next() {
		var f ExportBinPostFile
		if err := rows.Scan(&f.Filename, &f.Language, &f.Content, &f.Position); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *ExportService) collectAuthoredBinLineComments(ctx context.Context, userID int64) ([]ExportBinLineComment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, post_id, version_id, file_id, line_number, content, parent_id,
		       created_at, updated_at
		FROM bin_line_comments
		WHERE author_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportBinLineComment{}
	for rows.Next() {
		var (
			c        ExportBinLineComment
			parentID sql.NullInt64
		)
		if err := rows.Scan(&c.ID, &c.PostID, &c.VersionID, &c.FileID,
			&c.LineNumber, &c.Content, &parentID,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			v := parentID.Int64
			c.ParentID = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ExportService) collectDmChannelMembership(ctx context.Context, userID int64) ([]ExportDmChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT dc.id, dc.is_group, dc.name, m.joined_at, dc.created_at
		FROM dm_channel_members m
		JOIN dm_channels dc ON dc.id = m.dm_channel_id
		WHERE m.user_id = $1
		ORDER BY dc.created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportDmChannel{}
	for rows.Next() {
		var (
			c    ExportDmChannel
			name sql.NullString
		)
		if err := rows.Scan(&c.ID, &c.IsGroup, &name, &c.JoinedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		if name.Valid {
			v := name.String
			c.Name = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ExportService) collectNotifications(ctx context.Context, userID int64) ([]db.Notification, error) {
	// Spec calls for "full rows" — bypass the repository's GetUserNotifications
	// helper because that one caps at 100 and is used for the live inbox.
	// Export must not silently truncate.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, type, title, body, actor_username, actor_avatar_url,
		       server_id, channel_id, message_id, dm_channel_id, read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []db.Notification{}
	for rows.Next() {
		n := db.Notification{}
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body,
			&n.ActorUsername, &n.ActorAvatarURL,
			&n.ServerID, &n.ChannelID, &n.MessageID, &n.DmChannelID,
			&n.Read, &n.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *ExportService) collectAuditLogTargetingUser(ctx context.Context, userID int64) ([]ExportAuditLogEntry, error) {
	// server_audit_logs.target_id is TEXT; the canonical "this row is about
	// user X" filter is (target_type='user' AND target_id=X::text). actor_id
	// is FK-on-delete-set-null so an actor that has since deleted their
	// account becomes NULL.
	uidStr := fmt.Sprintf("%d", userID)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, server_id, actor_id, COALESCE(actor_username,''),
		       action, COALESCE(target_id,''), COALESCE(target_type,''),
		       COALESCE(target_name,''), changes, COALESCE(reason,''),
		       created_at
		FROM server_audit_logs
		WHERE target_type = 'user' AND target_id = $1
		ORDER BY created_at
	`, uidStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportAuditLogEntry{}
	for rows.Next() {
		var (
			e       ExportAuditLogEntry
			actorID sql.NullInt64
			changes []byte
		)
		if err := rows.Scan(&e.ID, &e.ServerID, &actorID, &e.ActorUsername,
			&e.Action, &e.TargetID, &e.TargetType, &e.TargetName,
			&changes, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		if actorID.Valid {
			v := actorID.Int64
			e.ActorID = &v
		}
		if len(changes) > 0 {
			e.Changes = json.RawMessage(changes)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
