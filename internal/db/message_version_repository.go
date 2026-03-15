package db

import (
	"context"
	"time"
)

// MessageVersion represents a previous version of a message.
type MessageVersion struct {
	ID        int64     `json:"id" db:"id"`
	MessageID int64     `json:"message_id" db:"message_id"`
	Content   string    `json:"content" db:"content"`
	EditedAt  time.Time `json:"edited_at" db:"edited_at"`
}

// SaveMessageVersion saves the current content of a message before it is edited.
func (r *Repository) SaveMessageVersion(ctx context.Context, messageID int64, content string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO message_versions (message_id, content, edited_at) VALUES ($1, $2, NOW())`,
		messageID, content)
	return err
}

// GetMessageVersions returns all saved versions for a message, newest first.
func (r *Repository) GetMessageVersions(ctx context.Context, messageID int64) ([]MessageVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, message_id, content, edited_at FROM message_versions
         WHERE message_id = $1 ORDER BY edited_at DESC`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []MessageVersion
	for rows.Next() {
		var v MessageVersion
		if err := rows.Scan(&v.ID, &v.MessageID, &v.Content, &v.EditedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// PurgeOldMessageVersions deletes message versions older than 90 days.
func (r *Repository) PurgeOldMessageVersions(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM message_versions WHERE edited_at < NOW() - INTERVAL '90 days'`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
