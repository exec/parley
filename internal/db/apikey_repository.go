package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
)

// ============ Developer API Key Operations ============

// CreateBotUser creates a new bot user account owned by the given user.
func (r *Repository) CreateBotUser(ctx context.Context, username string, ownerID int64) (int64, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return 0, err
	}
	placeholderEmail := fmt.Sprintf("bot_%x@internal.parley", randomBytes)
	var botID int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash, is_bot, bot_owner_id, created_at, updated_at)
		 VALUES ($1, $2, '', TRUE, $3, NOW(), NOW())
		 RETURNING id`,
		username, placeholderEmail, ownerID,
	).Scan(&botID)
	return botID, err
}

// RenameBotUser renames a bot user, verifying ownership.
func (r *Repository) RenameBotUser(ctx context.Context, botID, ownerID int64, newUsername string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = $1, updated_at = NOW()
		 WHERE id = $2 AND is_bot = TRUE AND bot_owner_id = $3`,
		newUsername, botID, ownerID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateAPIKey stores a new hashed API key.
func (r *Repository) CreateAPIKey(ctx context.Context, keyHash, keyPrefix, name string, userID, ownerID int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO api_keys (key_hash, key_prefix, user_id, owner_id, name, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 RETURNING id`,
		keyHash, keyPrefix, userID, ownerID, name,
	).Scan(&id)
	return id, err
}

// GetAPIKeyByHash looks up an API key by its SHA-256 hash.
// Returns (keyID, userID, error).
func (r *Repository) GetAPIKeyByHash(ctx context.Context, keyHash string) (int64, int64, error) {
	var keyID, userID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id FROM api_keys WHERE key_hash = $1`, keyHash,
	).Scan(&keyID, &userID)
	if err == sql.ErrNoRows {
		return 0, 0, ErrNotFound
	}
	return keyID, userID, err
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key.
func (r *Repository) UpdateAPIKeyLastUsed(ctx context.Context, keyID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`, keyID,
	)
	return err
}

// CountBotsByOwner returns how many bot users the given owner has created.
func (r *Repository) CountBotsByOwner(ctx context.Context, ownerID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE is_bot = TRUE AND bot_owner_id = $1`,
		ownerID,
	).Scan(&count)
	return count, err
}

// GetAPIKeysByOwner returns all API keys owned by the given user, enriched with bot info.
func (r *Repository) GetAPIKeysByOwner(ctx context.Context, ownerID int64) ([]APIKeyInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT k.id, k.key_prefix, k.user_id, k.owner_id, k.name, k.created_at, k.last_used_at,
		       u.is_bot, CASE WHEN u.is_bot THEN u.username ELSE '' END
		FROM api_keys k
		JOIN users u ON u.id = k.user_id
		WHERE k.owner_id = $1
		ORDER BY k.created_at DESC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []APIKeyInfo
	for rows.Next() {
		var k APIKeyInfo
		if err := rows.Scan(&k.ID, &k.KeyPrefix, &k.UserID, &k.OwnerID, &k.Name,
			&k.CreatedAt, &k.LastUsedAt, &k.IsBot, &k.BotUsername); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// CreateBotInviteToken inserts a bot_invite_tokens row for a newly created bot,
// recording the ownerID as created_by so the bot appears in the developer portal.
func (r *Repository) CreateBotInviteToken(ctx context.Context, botUserID, ownerID int64) (string, error) {
	var token string
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO bot_invite_tokens (bot_user_id, token, created_by)
		 VALUES ($1, gen_random_uuid(), $2)
		 ON CONFLICT (bot_user_id) DO UPDATE SET created_by = EXCLUDED.created_by
		 RETURNING token::text`,
		botUserID, ownerID,
	).Scan(&token)
	return token, err
}

// CreateBotWithKey atomically creates a bot user, its invite token, and an API key.
// Returns (botUserID, keyID, error).
func (r *Repository) CreateBotWithKey(ctx context.Context, botUsername, keyHash, keyPrefix, keyName string, ownerID int64) (botUserID int64, keyID int64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// 1. Create bot user
	randomBytes := make([]byte, 8)
	if _, err = rand.Read(randomBytes); err != nil {
		return 0, 0, err
	}
	placeholderEmail := fmt.Sprintf("bot_%x@internal.parley", randomBytes)
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (username, email, password_hash, is_bot, bot_owner_id, created_at, updated_at)
		 VALUES ($1, $2, '', TRUE, $3, NOW(), NOW()) RETURNING id`,
		botUsername, placeholderEmail, ownerID,
	).Scan(&botUserID)
	if err != nil {
		return 0, 0, err
	}

	// 2. Create invite token
	_, err = tx.ExecContext(ctx,
		`INSERT INTO bot_invite_tokens (bot_user_id, token, created_by)
		 VALUES ($1, gen_random_uuid(), $2)`,
		botUserID, ownerID,
	)
	if err != nil {
		return 0, 0, err
	}

	// 3. Create API key
	err = tx.QueryRowContext(ctx,
		`INSERT INTO api_keys (key_hash, key_prefix, user_id, owner_id, name, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW()) RETURNING id`,
		keyHash, keyPrefix, botUserID, ownerID, keyName,
	).Scan(&keyID)
	if err != nil {
		return 0, 0, err
	}

	return botUserID, keyID, tx.Commit()
}

// RevokeAPIKey deletes an API key by ID, verifying ownership.
func (r *Repository) RevokeAPIKey(ctx context.Context, keyID, ownerID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE id = $1 AND owner_id = $2`, keyID, ownerID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
