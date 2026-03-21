package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ============ Message Operations ============

// CreateMessage creates a new message in the database.
// If a nonce is set, duplicate submissions (retries) return the existing message instead of inserting.
// parentID may be nil for top-level messages or a valid message ID for nested replies.
func (r *Repository) CreateMessage(ctx context.Context, channelID, authorID int64, content, nonce, attachmentURL, attachmentName, attachmentType string, viaAPI bool, parentID *int64) (*Message, error) {
	now := time.Now()

	query := `
		INSERT INTO messages (channel_id, author_id, content, nonce, attachment_url, attachment_name, attachment_type, via_api, parent_id, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (nonce) WHERE nonce IS NOT NULL AND nonce != ''
		DO UPDATE SET updated_at = messages.updated_at
		RETURNING id, created_at, updated_at, COALESCE(nonce, ''), attachment_url, attachment_name, attachment_type, parent_id
	`

	var message Message
	message.ChannelID = channelID
	message.AuthorID = authorID
	message.Content = content
	message.CreatedAt = now
	message.UpdatedAt = now
	message.ViaApi = viaAPI

	err := r.db.QueryRowContext(ctx, query,
		channelID,
		authorID,
		content,
		nonce,
		attachmentURL,
		attachmentName,
		attachmentType,
		viaAPI,
		parentID,
		now,
		now,
	).Scan(
		&message.ID,
		&message.CreatedAt,
		&message.UpdatedAt,
		&message.Nonce,
		&message.AttachmentURL,
		&message.AttachmentName,
		&message.AttachmentType,
		&message.ParentID,
	)
	if err != nil {
		return nil, err
	}

	return &message, nil
}

func (r *Repository) GetMessageByID(ctx context.Context, id int64) (*Message, error) {
	query := `
		SELECT id, channel_id, author_id, content, COALESCE(nonce, ''), created_at, updated_at
		FROM messages
		WHERE id = $1
	`

	var message Message
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&message.ID,
		&message.ChannelID,
		&message.AuthorID,
		&message.Content,
		&message.Nonce,
		&message.CreatedAt,
		&message.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &message, nil
}

// GetChannelMessages retrieves messages for a channel with cursor-based pagination.
// If beforeID > 0, returns messages with id < beforeID (older messages).
// Otherwise returns the latest `limit` messages.
// Results are always returned in ascending order (oldest first).
func (r *Repository) GetChannelMessages(ctx context.Context, channelID int64, limit int, beforeID int64) ([]*Message, error) {
	var query string
	var rows *sql.Rows
	var err error

	if beforeID > 0 {
		query = `
			SELECT * FROM (
				SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce, ''), m.created_at, m.updated_at,
				       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
				       u.username, COALESCE(u.avatar_url, ''), u.is_bot, COALESCE(u.display_name, ''), m.parent_id,
				       COALESCE(pu.username, ''), COALESCE(pu.display_name, ''), m.via_api,
				       (pin.message_id IS NOT NULL) AS is_pinned, pin.pinned_at
				FROM messages m
				JOIN users u ON u.id = m.author_id
				LEFT JOIN messages pm ON pm.id = m.parent_id
				LEFT JOIN users pu ON pu.id = pm.author_id
				LEFT JOIN pinned_messages pin ON pin.message_id = m.id AND pin.channel_id = m.channel_id
				WHERE m.channel_id = $1 AND m.id < $3
				ORDER BY m.id DESC
				LIMIT $2
			) sub
			ORDER BY id ASC
		`
		rows, err = r.db.QueryContext(ctx, query, channelID, limit, beforeID)
	} else {
		query = `
			SELECT * FROM (
				SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce, ''), m.created_at, m.updated_at,
				       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
				       u.username, COALESCE(u.avatar_url, ''), u.is_bot, COALESCE(u.display_name, ''), m.parent_id,
				       COALESCE(pu.username, ''), COALESCE(pu.display_name, ''), m.via_api,
				       (pin.message_id IS NOT NULL) AS is_pinned, pin.pinned_at
				FROM messages m
				JOIN users u ON u.id = m.author_id
				LEFT JOIN messages pm ON pm.id = m.parent_id
				LEFT JOIN users pu ON pu.id = pm.author_id
				LEFT JOIN pinned_messages pin ON pin.message_id = m.id AND pin.channel_id = m.channel_id
				WHERE m.channel_id = $1
				ORDER BY m.id DESC
				LIMIT $2
			) sub
			ORDER BY id ASC
		`
		rows, err = r.db.QueryContext(ctx, query, channelID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var message Message
		err := rows.Scan(
			&message.ID,
			&message.ChannelID,
			&message.AuthorID,
			&message.Content,
			&message.Nonce,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.AttachmentURL,
			&message.AttachmentName,
			&message.AttachmentType,
			&message.AuthorUsername,
			&message.AuthorAvatarURL,
			&message.AuthorIsBot,
			&message.AuthorDisplayName,
			&message.ParentID,
			&message.ParentAuthorUsername,
			&message.ParentAuthorDisplayName,
			&message.ViaApi,
			&message.IsPinned,
			&message.PinnedAt,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, &message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

// PinMessage pins a message in a channel. Idempotent (ON CONFLICT DO NOTHING).
func (r *Repository) PinMessage(ctx context.Context, channelID, messageID, pinnedByID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO pinned_messages (channel_id, message_id, pinned_by, pinned_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (channel_id, message_id) DO NOTHING`,
		channelID, messageID, pinnedByID,
	)
	return err
}

// UnpinMessage unpins a message in a channel.
func (r *Repository) UnpinMessage(ctx context.Context, channelID, messageID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM pinned_messages WHERE channel_id = $1 AND message_id = $2`,
		channelID, messageID,
	)
	return err
}

// GetPinnedMessages returns all pinned messages in a channel, newest-pinned first.
func (r *Repository) GetPinnedMessages(ctx context.Context, channelID int64) ([]*Message, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce, ''), m.created_at, m.updated_at,
		       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
		       u.username, COALESCE(u.avatar_url, ''), u.is_bot, COALESCE(u.display_name, ''), m.parent_id,
		       COALESCE(pu.username, ''), COALESCE(pu.display_name, ''), m.via_api,
		       TRUE AS is_pinned, pin.pinned_at
		FROM pinned_messages pin
		JOIN messages m ON m.id = pin.message_id
		JOIN users u ON u.id = m.author_id
		LEFT JOIN messages pm ON pm.id = m.parent_id
		LEFT JOIN users pu ON pu.id = pm.author_id
		WHERE pin.channel_id = $1
		ORDER BY pin.pinned_at DESC
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var msg Message
		err := rows.Scan(
			&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce,
			&msg.CreatedAt, &msg.UpdatedAt,
			&msg.AttachmentURL, &msg.AttachmentName, &msg.AttachmentType,
			&msg.AuthorUsername, &msg.AuthorAvatarURL, &msg.AuthorIsBot, &msg.AuthorDisplayName,
			&msg.ParentID, &msg.ParentAuthorUsername, &msg.ParentAuthorDisplayName,
			&msg.ViaApi, &msg.IsPinned, &msg.PinnedAt,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, &msg)
	}
	return messages, rows.Err()
}

// SearchMessages searches messages within a server with optional content, author, and channel filters.
// Results are ordered by message ID descending (newest first), limited to `limit` rows.
// If beforeID > 0, only messages with id < beforeID are returned (cursor pagination).
func (r *Repository) SearchMessages(ctx context.Context, serverID int64, query string, authorID int64, channelID int64, limit int, beforeID int64) ([]*Message, error) {
	var conds []string
	var args []any
	p := 1

	if serverID > 0 {
		conds = append(conds, fmt.Sprintf("c.server_id = $%d", p))
		args = append(args, serverID)
		p++
	}

	if query != "" {
		escaped := strings.ReplaceAll(query, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `%`, `\%`)
		escaped = strings.ReplaceAll(escaped, `_`, `\_`)
		conds = append(conds, fmt.Sprintf("m.content ILIKE $%d ESCAPE '\\'", p))
		args = append(args, "%"+escaped+"%")
		p++
	}
	if authorID > 0 {
		conds = append(conds, fmt.Sprintf("m.author_id = $%d", p))
		args = append(args, authorID)
		p++
	}
	if channelID > 0 {
		conds = append(conds, fmt.Sprintf("m.channel_id = $%d", p))
		args = append(args, channelID)
		p++
	}
	if beforeID > 0 {
		conds = append(conds, fmt.Sprintf("m.id < $%d", p))
		args = append(args, beforeID)
		p++
	}
	args = append(args, limit)

	whereClause := "TRUE"
	if len(conds) > 0 {
		whereClause = strings.Join(conds, " AND ")
	}
	q := fmt.Sprintf(`
		SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce, ''),
		       m.created_at, m.updated_at,
		       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
		       u.username, COALESCE(u.avatar_url, ''), u.is_bot, COALESCE(u.display_name, ''), m.parent_id
		FROM messages m
		JOIN users u ON u.id = m.author_id
		JOIN channels c ON c.id = m.channel_id
		WHERE %s
		ORDER BY m.id DESC
		LIMIT $%d
	`, whereClause, p)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(
			&msg.ID, &msg.ChannelID, &msg.AuthorID, &msg.Content, &msg.Nonce,
			&msg.CreatedAt, &msg.UpdatedAt,
			&msg.AttachmentURL, &msg.AttachmentName, &msg.AttachmentType,
			&msg.AuthorUsername, &msg.AuthorAvatarURL, &msg.AuthorIsBot, &msg.AuthorDisplayName, &msg.ParentID,
		); err != nil {
			return nil, err
		}
		messages = append(messages, &msg)
	}
	return messages, rows.Err()
}

// EditMessage saves the current content as a version then updates the message content.
func (r *Repository) EditMessage(ctx context.Context, messageID int64, newContent string) (*Message, error) {
	// Save current content as a version before editing
	var oldContent string
	err := r.db.QueryRowContext(ctx,
		`SELECT content FROM messages WHERE id = $1`, messageID).Scan(&oldContent)
	if err != nil {
		return nil, err
	}
	if err := r.SaveMessageVersion(ctx, messageID, oldContent); err != nil {
		return nil, err
	}

	var message Message
	err = r.db.QueryRowContext(ctx,
		`UPDATE messages SET content = $1, updated_at = NOW()
		 WHERE id = $2
		 RETURNING id, channel_id, author_id, content, COALESCE(nonce, ''), created_at, updated_at,
		           COALESCE(attachment_url, ''), COALESCE(attachment_name, ''), COALESCE(attachment_type, '')`,
		newContent, messageID,
	).Scan(
		&message.ID,
		&message.ChannelID,
		&message.AuthorID,
		&message.Content,
		&message.Nonce,
		&message.CreatedAt,
		&message.UpdatedAt,
		&message.AttachmentURL,
		&message.AttachmentName,
		&message.AttachmentType,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *Repository) DeleteMessage(ctx context.Context, id int64) error {
	query := `DELETE FROM messages WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// GetMessageContext returns N messages before and after a given message ID in the same channel.
func (r *Repository) GetMessageContext(ctx context.Context, messageID int64, before, after int) ([]Message, error) {
	var channelID int64
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, `SELECT channel_id, created_at FROM messages WHERE id = $1`, messageID).Scan(&channelID, &createdAt)
	if err != nil {
		return nil, err
	}

	query := `
		(SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce,''),
		        COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
		        m.created_at, m.updated_at, u.username
		 FROM messages m JOIN users u ON u.id = m.author_id
		 WHERE m.channel_id = $1 AND m.created_at < $2
		 ORDER BY m.created_at DESC LIMIT $3)
		UNION ALL
		(SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce,''),
		        COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
		        m.created_at, m.updated_at, u.username
		 FROM messages m JOIN users u ON u.id = m.author_id
		 WHERE m.channel_id = $1 AND m.created_at >= $2
		 ORDER BY m.created_at ASC LIMIT $4)
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, channelID, createdAt, before+1, after+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce,
			&m.AttachmentURL, &m.AttachmentName, &m.AttachmentType,
			&m.CreatedAt, &m.UpdatedAt, &m.AuthorUsername); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

