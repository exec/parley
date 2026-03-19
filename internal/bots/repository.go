// internal/bots/repository.go
package bots

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	dbpkg "parley/internal/db"
)

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

type Repository struct {
	db     *sql.DB
	dbRepo *dbpkg.Repository
}

// NewRepository accepts the shared *db.Repository so permission checks can use it.
func NewRepository(repo *dbpkg.Repository) *Repository {
	return &Repository{db: repo.DB(), dbRepo: repo}
}

// DBRepo exposes the underlying db.Repository for use by the permissions package.
func (r *Repository) DBRepo() *dbpkg.Repository { return r.dbRepo }

// GetBotUserID returns the user ID of the named bot (cached by caller).
func (r *Repository) GetBotUserID(ctx context.Context, username string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username = $1 AND is_bot = TRUE`, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// ListServerBots returns all bots in a server.
func (r *Repository) ListServerBots(ctx context.Context, serverID int64) ([]Bot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), u.is_verified, sb.added_at
		FROM server_bots sb
		JOIN users u ON u.id = sb.bot_user_id
		WHERE sb.server_id = $1
		ORDER BY sb.added_at`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []Bot
	for rows.Next() {
		var b Bot
		if err := rows.Scan(&b.ID, &b.Username, &b.DisplayName, &b.AvatarURL, &b.IsVerified, &b.AddedAt); err != nil {
			return nil, err
		}
		bots = append(bots, b)
	}
	if bots == nil {
		bots = []Bot{}
	}
	return bots, rows.Err()
}

// IsBotInServer returns true if the given bot is in the server.
func (r *Repository) IsBotInServer(ctx context.Context, serverID, botUserID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM server_bots WHERE server_id=$1 AND bot_user_id=$2`, serverID, botUserID).Scan(&n)
	return n > 0, err
}

// AddBotToServer inserts a server_bots row and a server_members row so the bot
// appears in the members sidebar and is resolvable for @mentions.
func (r *Repository) AddBotToServer(ctx context.Context, serverID, botUserID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_bots (server_id, bot_user_id) VALUES ($1, $2)`,
		serverID, botUserID)
	if isPgUniqueViolation(err) {
		return ErrAlreadyExists
	}
	if err != nil {
		return err
	}
	// Mirror into server_members so the bot appears in the sidebar and mention list.
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO server_members (server_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		serverID, botUserID)
	return err
}

// AddBotToServerWithRole adds a bot to a server and (if grantedPermissions > 0)
// creates a role named "@{botUsername}" and assigns it to the bot.
// All steps run in a single transaction.
func (r *Repository) AddBotToServerWithRole(ctx context.Context, serverID, botUserID int64, botUsername string, grantedPermissions int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO server_bots (server_id, bot_user_id) VALUES ($1, $2)`,
		serverID, botUserID)
	if isPgUniqueViolation(err) {
		return ErrAlreadyExists
	}
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO server_members (server_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		serverID, botUserID)
	if err != nil {
		return err
	}

	if grantedPermissions > 0 {
		var roleID int64
		err = tx.QueryRowContext(ctx,
			`INSERT INTO server_roles (server_id, name, color, permissions)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			serverID, "@"+botUsername, "#99aab5", grantedPermissions,
		).Scan(&roleID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO server_member_roles (server_id, user_id, role_id)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			serverID, botUserID, roleID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RemoveBotFromServer deletes server_bots and server_members rows.
func (r *Repository) RemoveBotFromServer(ctx context.Context, serverID, botUserID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM server_bots WHERE server_id=$1 AND bot_user_id=$2`, serverID, botUserID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = r.db.ExecContext(ctx,
		`DELETE FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, botUserID)
	return nil
}

// SetBotDegraded updates the degraded flag and records the error timestamp.
func (r *Repository) SetBotDegraded(ctx context.Context, serverID, botUserID int64, degraded bool) error {
	if degraded {
		_, err := r.db.ExecContext(ctx,
			`UPDATE server_bots SET is_degraded=TRUE, last_error_at=NOW()
			 WHERE server_id=$1 AND bot_user_id=$2`, serverID, botUserID)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE server_bots SET is_degraded=FALSE
		 WHERE server_id=$1 AND bot_user_id=$2`, serverID, botUserID)
	return err
}

// GetAIConfig returns the AI config for a server, or nil if not set.
func (r *Repository) GetAIConfig(ctx context.Context, serverID int64) (*AIConfig, string, error) {
	var cfg AIConfig
	var apiKeyEnc sql.NullString
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT provider, model, api_key_enc,
		        COALESCE(preset_verbosity,'concise'), COALESCE(preset_personality,'friendly'), COALESCE(preset_role,'assistant'),
		        updated_at
		 FROM server_ai_config WHERE server_id = $1`, serverID).
		Scan(&cfg.Provider, &cfg.Model, &apiKeyEnc,
			&cfg.PresetVerbosity, &cfg.PresetPersonality, &cfg.PresetRole,
			&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	cfg.APIKeySet = apiKeyEnc.Valid && apiKeyEnc.String != ""
	cfg.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	rawEnc := ""
	if apiKeyEnc.Valid {
		rawEnc = apiKeyEnc.String
	}
	return &cfg, rawEnc, nil
}

// UpsertAIConfig saves AI config. Pass nil apiKeyEnc to leave existing key unchanged.
func (r *Repository) UpsertAIConfig(ctx context.Context, serverID int64, provider, model, verbosity, personality, role string, apiKeyEnc *string) error {
	if apiKeyEnc != nil {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO server_ai_config (server_id, provider, model, api_key_enc, preset_verbosity, preset_personality, preset_role, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			ON CONFLICT (server_id) DO UPDATE SET
				provider=EXCLUDED.provider, model=EXCLUDED.model,
				api_key_enc=EXCLUDED.api_key_enc,
				preset_verbosity=EXCLUDED.preset_verbosity, preset_personality=EXCLUDED.preset_personality, preset_role=EXCLUDED.preset_role,
				updated_at=NOW()`,
			serverID, provider, model, *apiKeyEnc, verbosity, personality, role)
		return err
	}
	// Don't overwrite existing api_key_enc
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO server_ai_config (server_id, provider, model, preset_verbosity, preset_personality, preset_role, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (server_id) DO UPDATE SET
			provider=EXCLUDED.provider, model=EXCLUDED.model,
			preset_verbosity=EXCLUDED.preset_verbosity, preset_personality=EXCLUDED.preset_personality, preset_role=EXCLUDED.preset_role,
			updated_at=NOW()`,
		serverID, provider, model, verbosity, personality, role)
	return err
}

// GetMonthlyUsage returns tokens_used for the current month.
func (r *Repository) GetMonthlyUsage(ctx context.Context, serverID int64) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(tokens_used,0) FROM server_bot_usage
		 WHERE server_id=$1 AND month=DATE_TRUNC('month',NOW())::date`, serverID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// AddTokenUsage atomically increments tokens_used for the current month.
func (r *Repository) AddTokenUsage(ctx context.Context, serverID int64, delta int64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO server_bot_usage (server_id, month, tokens_used)
		VALUES ($1, DATE_TRUNC('month',NOW())::date, $2)
		ON CONFLICT (server_id, month) DO UPDATE
		SET tokens_used = server_bot_usage.tokens_used + EXCLUDED.tokens_used`,
		serverID, delta)
	return err
}

// ResolveInviteToken returns bot user ID and permissions for a given invite token UUID.
func (r *Repository) ResolveInviteToken(ctx context.Context, token string) (int64, int64, error) {
	var botUserID, permissions int64
	err := r.db.QueryRowContext(ctx,
		`SELECT bot_user_id, permissions FROM bot_invite_tokens WHERE token=$1::uuid`, token).
		Scan(&botUserID, &permissions)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	return botUserID, permissions, err
}

// UpdateBotInvitePermissions sets permissions on the invite token for botUserID,
// owned by callerID. Returns ErrNotFound if the caller doesn't own this bot.
func (r *Repository) UpdateBotInvitePermissions(ctx context.Context, botUserID, callerID, permissions int64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE bot_invite_tokens SET permissions=$1 WHERE bot_user_id=$2 AND created_by=$3`,
		permissions, botUserID, callerID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetUserBots returns bots whose invite tokens were created by callerID,
// excluding selfbots (bot_user_id = callerID). Each entry includes the
// invite token so the caller can add the bot to a server directly.
func (r *Repository) GetUserBots(ctx context.Context, callerID int64) ([]UserBot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT ON (u.id)
			u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.avatar_url,''),
			u.is_verified, bit.token::text, bit.permissions
		FROM bot_invite_tokens bit
		JOIN users u ON u.id = bit.bot_user_id
		WHERE bit.created_by = $1
		  AND bit.bot_user_id != $1
		ORDER BY u.id, bit.created_at DESC`, callerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []UserBot
	for rows.Next() {
		var b UserBot
		if err := rows.Scan(&b.ID, &b.Username, &b.DisplayName, &b.AvatarURL, &b.IsVerified, &b.InviteToken, &b.Permissions); err != nil {
			return nil, err
		}
		bots = append(bots, b)
	}
	if bots == nil {
		bots = []UserBot{}
	}
	return bots, rows.Err()
}

// GetBotInfo returns BotInviteInfo for a bot user ID.
func (r *Repository) GetBotInfo(ctx context.Context, botUserID int64) (*BotInviteInfo, error) {
	var b BotInviteInfo
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, COALESCE(display_name,''), COALESCE(avatar_url,''), is_verified
		 FROM users WHERE id=$1 AND is_bot=TRUE`, botUserID).
		Scan(&b.BotID, &b.Username, &b.DisplayName, &b.AvatarURL, &b.IsVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

// IsServerMember returns true if userID is a member of the server.
func (r *Repository) IsServerMember(ctx context.Context, serverID, userID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&n)
	return n > 0, err
}

// GetChannelServerID returns the server_id for a channel.
// Note: DM messages use the dm_messages table and go through a completely separate code
// path — they never reach SendMessage and therefore never trigger this. All rows in the
// channels table have a non-null server_id (NOT NULL constraint in migration 0).
func (r *Repository) GetChannelServerID(ctx context.Context, channelID int64) (int64, bool, error) {
	var serverID int64
	err := r.db.QueryRowContext(ctx, `SELECT server_id FROM channels WHERE id=$1`, channelID).Scan(&serverID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return serverID, true, nil
}

// GetServerOwnerID returns the owner_id for a server. Used for permission checks.
func (r *Repository) GetServerOwnerID(ctx context.Context, serverID int64) (int64, error) {
	var ownerID int64
	err := r.db.QueryRowContext(ctx, `SELECT owner_id FROM servers WHERE id=$1`, serverID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return ownerID, err
}

// GetReplyChain returns messages walking the parent_id chain from msgID, oldest first.
// Stops at maxHops hops or when totalChars exceeds charBudget.
func (r *Repository) GetReplyChain(ctx context.Context, msgID int64, maxHops int, charBudget int) ([]ChainMessage, error) {
	var chain []ChainMessage
	current := msgID
	totalChars := 0
	for i := 0; i < maxHops; i++ {
		var cm ChainMessage
		var parentID sql.NullInt64
		err := r.db.QueryRowContext(ctx,
			`SELECT m.id, m.author_id, m.content, m.parent_id, u.is_bot
			 FROM messages m JOIN users u ON u.id = m.author_id
			 WHERE m.id = $1`, current).
			Scan(&cm.ID, &cm.AuthorID, &cm.Content, &parentID, &cm.IsBot)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return nil, err
		}
		chain = append(chain, cm)
		totalChars += len(cm.Content)
		if totalChars > charBudget || !parentID.Valid {
			break
		}
		current = parentID.Int64
	}
	// Reverse to get oldest-first order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// ChainMessage is a lightweight message used for context building.
type ChainMessage struct {
	ID       int64
	AuthorID int64
	Content  string
	IsBot    bool
}

// GetUserDisplayName returns display_name (falling back to username) for a user ID.
func (r *Repository) GetUserDisplayName(ctx context.Context, userID int64) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(NULLIF(display_name,''), username) FROM users WHERE id=$1`, userID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// isPgUniqueViolation checks for Postgres unique constraint violation (code 23505).
// The project uses github.com/lib/pq so we can use the typed error code.
func isPgUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
