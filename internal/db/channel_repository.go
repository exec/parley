package db

import (
	"context"
	"database/sql"
	"time"
)

// ============ Channel Operations ============

func (r *Repository) CreateChannel(ctx context.Context, channel *Channel) error {
	query := `
		INSERT INTO channels (server_id, name, channel_type, position, parent_id, topic, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	channel.CreatedAt = time.Now()

	err := r.db.QueryRowContext(ctx, query,
		channel.ServerID,
		channel.Name,
		channel.ChannelType,
		channel.Position,
		channel.ParentID,
		channel.Topic,
		channel.CreatedAt,
	).Scan(&channel.ID)

	return err
}

func (r *Repository) GetChannelByID(ctx context.Context, id int64) (*Channel, error) {
	query := `
		SELECT id, server_id, name, channel_type, position, parent_id, topic, created_at, updated_at
		FROM channels
		WHERE id = $1
	`

	var channel Channel
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID,
		&channel.ServerID,
		&channel.Name,
		&channel.ChannelType,
		&channel.Position,
		&channel.ParentID,
		&channel.Topic,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &channel, nil
}

func (r *Repository) GetChannelsByServerID(ctx context.Context, serverID int64) ([]*Channel, error) {
	query := `
		SELECT id, server_id, name, channel_type, position, parent_id, topic, created_at, updated_at
		FROM channels
		WHERE server_id = $1
		ORDER BY position, name
	`

	rows, err := r.db.QueryContext(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		var channel Channel
		err := rows.Scan(
			&channel.ID,
			&channel.ServerID,
			&channel.Name,
			&channel.ChannelType,
			&channel.Position,
			&channel.ParentID,
			&channel.Topic,
			&channel.CreatedAt,
			&channel.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, &channel)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return channels, nil
}

func (r *Repository) UpdateChannel(ctx context.Context, channel *Channel) error {
	query := `
		UPDATE channels
		SET name = $1, channel_type = $2, position = $3, parent_id = $4, topic = $5, updated_at = NOW()
		WHERE id = $6
	`

	result, err := r.db.ExecContext(ctx, query,
		channel.Name,
		channel.ChannelType,
		channel.Position,
		channel.ParentID,
		channel.Topic,
		channel.ID,
	)
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

func (r *Repository) UpdateChannelOrder(ctx context.Context, id int64, position int, parentID sql.NullInt64) error {
	query := `UPDATE channels SET position = $1, parent_id = $2, updated_at = NOW() WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, position, parentID, id)
	return err
}

func (r *Repository) DeleteChannel(ctx context.Context, id int64) error {
	query := `DELETE FROM channels WHERE id = $1`

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
