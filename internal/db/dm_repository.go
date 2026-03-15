package db

import (
	"context"
	"database/sql"
)

// ============ DM Channel Operations ============

// GetOrCreateDmChannel finds or creates a DM channel between two users.
func (r *Repository) GetOrCreateDmChannel(ctx context.Context, userAID, userBID int64) (*DmChannel, error) {
	// Ensure user1_id < user2_id for the UNIQUE constraint
	user1ID, user2ID := userAID, userBID
	if user1ID > user2ID {
		user1ID, user2ID = user2ID, user1ID
	}

	insertQuery := `
		INSERT INTO dm_channels (user1_id, user2_id, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, insertQuery, user1ID, user2ID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, user1_id, user2_id, created_at
		FROM dm_channels
		WHERE user1_id = $1 AND user2_id = $2
	`
	var channel DmChannel
	err = r.db.QueryRowContext(ctx, query, user1ID, user2ID).Scan(
		&channel.ID,
		&channel.User1ID,
		&channel.User2ID,
		&channel.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	otherUserID := user2ID
	if userAID == user1ID {
		otherUserID = user2ID
	} else {
		otherUserID = user1ID
	}

	var otherUser User
	err = r.db.QueryRowContext(ctx, "SELECT id, username, COALESCE(avatar_url, '') FROM users WHERE id = $1", otherUserID).Scan(&otherUser.ID, &otherUser.Username, &otherUser.AvatarURL)
	if err == nil {
		channel.OtherUserID = otherUser.ID
		channel.OtherUsername = otherUser.Username
		channel.OtherAvatarURL = otherUser.AvatarURL
	}

	return &channel, nil
}

func (r *Repository) GetUserDmChannels(ctx context.Context, userID int64) ([]DmChannel, error) {
	query := `
		SELECT dc.id, dc.user1_id, dc.user2_id, dc.created_at,
			   u.id as other_user_id, u.username as other_username, COALESCE(u.avatar_url, '') as other_avatar_url
		FROM dm_channels dc
		JOIN users u ON u.id = CASE WHEN dc.user1_id = $1 THEN dc.user2_id ELSE dc.user1_id END
		WHERE dc.user1_id = $1 OR dc.user2_id = $1
		ORDER BY dc.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []DmChannel
	for rows.Next() {
		var channel DmChannel
		err := rows.Scan(
			&channel.ID,
			&channel.User1ID,
			&channel.User2ID,
			&channel.CreatedAt,
			&channel.OtherUserID,
			&channel.OtherUsername,
			&channel.OtherAvatarURL,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *Repository) GetDmChannelByID(ctx context.Context, id int64) (*DmChannel, error) {
	query := `
		SELECT id, user1_id, user2_id, created_at
		FROM dm_channels
		WHERE id = $1
	`

	var channel DmChannel
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID,
		&channel.User1ID,
		&channel.User2ID,
		&channel.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// ============ DM Message Operations ============

func (r *Repository) CreateDmMessage(ctx context.Context, dmChannelID, authorID int64, content, attachmentURL, attachmentName, attachmentType string) (*DmMessage, error) {
	query := `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, attachment_url, attachment_name, attachment_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, dm_channel_id, author_id, content, attachment_url, attachment_name, attachment_type, created_at, updated_at
	`

	var msg DmMessage
	err := r.db.QueryRowContext(ctx, query, dmChannelID, authorID, content, attachmentURL, attachmentName, attachmentType).Scan(
		&msg.ID,
		&msg.DmChannelID,
		&msg.AuthorID,
		&msg.Content,
		&msg.AttachmentURL,
		&msg.AttachmentName,
		&msg.AttachmentType,
		&msg.CreatedAt,
		&msg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	var username, avatarURL string
	r.db.QueryRowContext(ctx, "SELECT username, COALESCE(avatar_url, '') FROM users WHERE id = $1", authorID).Scan(&username, &avatarURL)
	msg.AuthorUsername = username
	msg.AuthorAvatarURL = avatarURL

	return &msg, nil
}

// GetDmMessages retrieves messages for a DM channel with cursor-based pagination.
// If beforeID > 0, returns messages with id < beforeID (older messages).
// Otherwise returns the latest `limit` messages.
// Results are always returned in ascending order (oldest first).
func (r *Repository) GetDmMessages(ctx context.Context, dmChannelID int64, limit int, beforeID int64) ([]DmMessage, error) {
	var query string
	var rows *sql.Rows
	var err error

	if beforeID > 0 {
		query = `
			SELECT * FROM (
				SELECT m.id, m.dm_channel_id, m.author_id, m.content,
				       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
				       m.created_at, m.updated_at, u.username, COALESCE(u.avatar_url, ''), COALESCE(u.display_name, '')
				FROM dm_messages m
				JOIN users u ON u.id = m.author_id
				WHERE m.dm_channel_id = $1 AND m.id < $3
				ORDER BY m.id DESC
				LIMIT $2
			) sub
			ORDER BY id ASC
		`
		rows, err = r.db.QueryContext(ctx, query, dmChannelID, limit, beforeID)
	} else {
		query = `
			SELECT * FROM (
				SELECT m.id, m.dm_channel_id, m.author_id, m.content,
				       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
				       m.created_at, m.updated_at, u.username, COALESCE(u.avatar_url, ''), COALESCE(u.display_name, '')
				FROM dm_messages m
				JOIN users u ON u.id = m.author_id
				WHERE m.dm_channel_id = $1
				ORDER BY m.id DESC
				LIMIT $2
			) sub
			ORDER BY id ASC
		`
		rows, err = r.db.QueryContext(ctx, query, dmChannelID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []DmMessage
	for rows.Next() {
		var msg DmMessage
		err := rows.Scan(
			&msg.ID,
			&msg.DmChannelID,
			&msg.AuthorID,
			&msg.Content,
			&msg.AttachmentURL,
			&msg.AttachmentName,
			&msg.AttachmentType,
			&msg.CreatedAt,
			&msg.UpdatedAt,
			&msg.AuthorUsername,
			&msg.AuthorAvatarURL,
			&msg.AuthorDisplayName,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

// SendSystemDM sends a DM from the system user to a recipient. Creates DM channel if needed.
func (r *Repository) SendSystemDM(ctx context.Context, systemUserID, recipientID int64, content string) error {
	var dmChannelID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM dm_channels
		WHERE (user1_id = $1 AND user2_id = $2) OR (user1_id = $2 AND user2_id = $1)
	`, systemUserID, recipientID).Scan(&dmChannelID)
	if err == sql.ErrNoRows {
		err = r.db.QueryRowContext(ctx, `
			INSERT INTO dm_channels (user1_id, user2_id, created_at)
			VALUES ($1, $2, NOW()) RETURNING id
		`, systemUserID, recipientID).Scan(&dmChannelID)
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, dmChannelID, systemUserID, content)
	return err
}
