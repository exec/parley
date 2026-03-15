package db

import (
	"context"

	"github.com/lib/pq"
)

// permServerOnlyMask masks out server-only permission bits (bits 0–13) from channel overwrites.
// Mirrors permissions.PermServerOnlyMask = (1 << 14) - 1.
const permServerOnlyMask int64 = (1 << 14) - 1

// PermissionOverwrite represents a permission overwrite record in the database.
type PermissionOverwrite struct {
	ID         int64 `json:"id" db:"id"`
	ChannelID  int64 `json:"channel_id" db:"channel_id"`
	TargetType int   `json:"target_type" db:"target_type"`
	TargetID   int64 `json:"target_id" db:"target_id"`
	Allow      int64 `json:"allow" db:"allow"`
	Deny       int64 `json:"deny" db:"deny"`
}

// ============ Permission Overwrite Operations ============

// GetChannelOverwrites returns all permission overwrites for a channel as Overwrite structs.
func (r *Repository) GetChannelOverwrites(ctx context.Context, channelID int64) ([]Overwrite, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_type, target_id, allow, deny
         FROM permission_overwrites WHERE channel_id = $1`,
		channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overwrites []Overwrite
	for rows.Next() {
		var ow Overwrite
		if err := rows.Scan(&ow.TargetType, &ow.TargetID, &ow.Allow, &ow.Deny); err != nil {
			return nil, err
		}
		overwrites = append(overwrites, ow)
	}
	return overwrites, rows.Err()
}

// GetOverwritesByChannels bulk-fetches permission overwrites for a list of channels.
// Returns a map keyed by channel_id.
func (r *Repository) GetOverwritesByChannels(ctx context.Context, channelIDs []int64) (map[int64][]Overwrite, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT channel_id, target_type, target_id, allow, deny
         FROM permission_overwrites WHERE channel_id = ANY($1)`,
		pq.Array(channelIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]Overwrite)
	for rows.Next() {
		var channelID int64
		var ow Overwrite
		if err := rows.Scan(&channelID, &ow.TargetType, &ow.TargetID, &ow.Allow, &ow.Deny); err != nil {
			return nil, err
		}
		result[channelID] = append(result[channelID], ow)
	}
	return result, rows.Err()
}

// GetRawChannelOverwrites returns all permission overwrites for a channel as PermissionOverwrite DB models (includes IDs).
func (r *Repository) GetRawChannelOverwrites(ctx context.Context, channelID int64) ([]PermissionOverwrite, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, channel_id, target_type, target_id, allow, deny
         FROM permission_overwrites WHERE channel_id = $1`,
		channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overwrites []PermissionOverwrite
	for rows.Next() {
		var ow PermissionOverwrite
		if err := rows.Scan(&ow.ID, &ow.ChannelID, &ow.TargetType, &ow.TargetID, &ow.Allow, &ow.Deny); err != nil {
			return nil, err
		}
		overwrites = append(overwrites, ow)
	}
	return overwrites, rows.Err()
}

// UpsertOverwrite inserts or updates a permission overwrite for a channel target.
// Server-only permission bits are masked out from allow and deny.
func (r *Repository) UpsertOverwrite(ctx context.Context, channelID int64, targetType int, targetID int64, allow, deny int64) (*PermissionOverwrite, error) {
	allow &= ^permServerOnlyMask
	deny &= ^permServerOnlyMask

	var ow PermissionOverwrite
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO permission_overwrites (channel_id, target_type, target_id, allow, deny)
         VALUES ($1, $2, $3, $4, $5)
         ON CONFLICT (channel_id, target_type, target_id) DO UPDATE
             SET allow = EXCLUDED.allow, deny = EXCLUDED.deny
         RETURNING id, channel_id, target_type, target_id, allow, deny`,
		channelID, targetType, targetID, allow, deny,
	).Scan(&ow.ID, &ow.ChannelID, &ow.TargetType, &ow.TargetID, &ow.Allow, &ow.Deny)
	if err != nil {
		return nil, err
	}
	return &ow, nil
}

// DeleteOverwrite deletes a permission overwrite by ID.
func (r *Repository) DeleteOverwrite(ctx context.Context, overwriteID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM permission_overwrites WHERE id = $1`,
		overwriteID)
	return err
}

// DeleteOverwritesByTarget deletes all overwrites for a given role or member target across all channels.
func (r *Repository) DeleteOverwritesByTarget(ctx context.Context, targetType int, targetID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM permission_overwrites WHERE target_type = $1 AND target_id = $2`,
		targetType, targetID)
	return err
}

// CopyOverwrites replaces all overwrites on toChannelID with copies from fromChannelID.
func (r *Repository) CopyOverwrites(ctx context.Context, fromChannelID, toChannelID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM permission_overwrites WHERE channel_id = $1`,
		toChannelID)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO permission_overwrites (channel_id, target_type, target_id, allow, deny)
         SELECT $1, target_type, target_id, allow, deny
         FROM permission_overwrites WHERE channel_id = $2`,
		toChannelID, fromChannelID)
	return err
}

