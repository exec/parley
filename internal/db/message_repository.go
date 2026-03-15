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
func (r *Repository) CreateMessage(ctx context.Context, channelID, authorID int64, content, nonce, attachmentURL, attachmentName, attachmentType string, viaAPI bool) (*Message, error) {
	now := time.Now()

	query := `
		INSERT INTO messages (channel_id, author_id, content, nonce, attachment_url, attachment_name, attachment_type, via_api, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10)
		ON CONFLICT (nonce) WHERE nonce IS NOT NULL AND nonce != ''
		DO UPDATE SET updated_at = messages.updated_at
		RETURNING id, created_at, updated_at, COALESCE(nonce, ''), attachment_url, attachment_name, attachment_type
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
				       u.username, COALESCE(u.avatar_url, ''), u.is_bot, COALESCE(u.display_name, '')
				FROM messages m
				JOIN users u ON u.id = m.author_id
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
				       u.username, COALESCE(u.avatar_url, ''), u.is_bot, COALESCE(u.display_name, '')
				FROM messages m
				JOIN users u ON u.id = m.author_id
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

// SearchMessages searches messages by content and/or author username, paginated.
func (r *Repository) SearchMessages(ctx context.Context, query string, userID int64, limit, offset int) ([]Message, error) {
	args := []interface{}{}
	where := []string{}
	if query != "" {
		args = append(args, "%"+query+"%")
		where = append(where, fmt.Sprintf("m.content ILIKE $%d", len(args)))
	}
	if userID > 0 {
		args = append(args, userID)
		where = append(where, fmt.Sprintf("m.author_id = $%d", len(args)))
	}
	sqlStr := `SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce,''),
	                  COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
	                  m.created_at, m.updated_at, u.username
	           FROM messages m JOIN users u ON u.id = m.author_id`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, limit, offset)
	sqlStr += fmt.Sprintf(` ORDER BY m.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
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
