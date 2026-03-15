package db

import (
	"context"
	"database/sql"
)

// CreateBinChannelTag inserts a new tag for a bin channel.
func (r *Repository) CreateBinChannelTag(ctx context.Context, channelID int64, name, color string) (*BinChannelTag, error) {
	tag := &BinChannelTag{
		ChannelID: channelID,
		Name:      name,
		Color:     color,
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO bin_channel_tags (channel_id, name, color) VALUES ($1, $2, $3) RETURNING id`,
		channelID, name, color,
	).Scan(&tag.ID)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

// GetBinChannelTags retrieves all tags for a bin channel.
func (r *Repository) GetBinChannelTags(ctx context.Context, channelID int64) ([]BinChannelTag, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, name, color FROM bin_channel_tags WHERE channel_id = $1`,
		channelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []BinChannelTag
	for rows.Next() {
		var t BinChannelTag
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// DeleteBinChannelTag deletes a bin channel tag by ID.
func (r *Repository) DeleteBinChannelTag(ctx context.Context, tagID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM bin_channel_tags WHERE id = $1`, tagID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetBinChannelTagByID retrieves a single bin channel tag by ID.
func (r *Repository) GetBinChannelTagByID(ctx context.Context, tagID int64) (*BinChannelTag, error) {
	var t BinChannelTag
	err := r.db.QueryRowContext(ctx,
		`SELECT id, channel_id, name, color FROM bin_channel_tags WHERE id = $1`, tagID,
	).Scan(&t.ID, &t.ChannelID, &t.Name, &t.Color)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
