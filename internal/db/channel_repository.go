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
	if v, ok := r.channelCache.Load(id); ok {
		e := v.(*cachedChannel)
		if time.Now().Before(e.expiresAt) {
			return e.ch, nil
		}
	}

	query := `
		SELECT id, server_id, name, channel_type, position, parent_id, topic, synced, created_at, updated_at
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
		&channel.Synced,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	r.channelCache.Store(id, &cachedChannel{ch: &channel, expiresAt: time.Now().Add(metadataCacheTTL)})
	return &channel, nil
}

func (r *Repository) GetChannelsByServerID(ctx context.Context, serverID int64) ([]*Channel, error) {
	query := `
		SELECT id, server_id, name, channel_type, position, parent_id, topic, synced, created_at, updated_at
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
			&channel.Synced,
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

	r.invalidateChannelCache(channel.ID)
	return nil
}

func (r *Repository) UpdateChannelOrder(ctx context.Context, id int64, position int, parentID sql.NullInt64) error {
	query := `UPDATE channels SET position = $1, parent_id = $2, updated_at = NOW() WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, position, parentID, id)
	if err == nil {
		r.invalidateChannelCache(id)
	}
	return err
}

// SetChannelSynced sets the synced flag on a channel.
func (r *Repository) SetChannelSynced(ctx context.Context, channelID int64, synced bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE channels SET synced = $1, updated_at = NOW() WHERE id = $2`,
		synced, channelID)
	if err == nil {
		r.invalidateChannelCache(channelID)
	}
	return err
}

// GetSyncedChildrenByParent returns channels that have the given parentID and synced = true.
func (r *Repository) GetSyncedChildrenByParent(ctx context.Context, parentID int64) ([]*Channel, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, name, channel_type, position, parent_id, topic, synced, created_at, updated_at
         FROM channels WHERE parent_id = $1 AND synced = true`,
		parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.ServerID, &ch.Name, &ch.ChannelType, &ch.Position, &ch.ParentID, &ch.Topic, &ch.Synced, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, &ch)
	}
	return channels, rows.Err()
}

// HasChildren returns true if the channel has any child channels.
func (r *Repository) HasChildren(ctx context.Context, channelID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM channels WHERE parent_id = $1`,
		channelID).Scan(&count)
	return count > 0, err
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
	r.invalidateChannelCache(id)

	return nil
}
