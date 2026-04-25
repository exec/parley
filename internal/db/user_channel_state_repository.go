package db

import (
	"context"
	"database/sql"
)

// GetUserChannelState returns nil + nil error if no row exists (default state).
func (r *Repository) GetUserChannelState(ctx context.Context, userID int64, kind ChannelKind, channelID int64) (*UserChannelState, error) {
	var s UserChannelState
	err := r.db.QueryRowContext(ctx, `
        SELECT user_id, channel_kind, channel_id, last_read_message_id, notification_setting, updated_at
          FROM user_channel_state
         WHERE user_id = $1 AND channel_kind = $2 AND channel_id = $3
    `, userID, kind, channelID).Scan(
		&s.UserID, &s.ChannelKind, &s.ChannelID,
		&s.LastReadMessageID, &s.NotificationSetting, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertReadMarker writes the last_read_message_id for (user, channel), preserving notification_setting.
func (r *Repository) UpsertReadMarker(ctx context.Context, userID int64, kind ChannelKind, channelID int64, messageID int64) error {
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO user_channel_state (user_id, channel_kind, channel_id, last_read_message_id, updated_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (user_id, channel_kind, channel_id)
        DO UPDATE SET last_read_message_id = EXCLUDED.last_read_message_id, updated_at = NOW()
    `, userID, kind, channelID, messageID)
	return err
}

// UpsertNotificationSetting writes the notification_setting for (user, channel), preserving last_read_message_id.
func (r *Repository) UpsertNotificationSetting(ctx context.Context, userID int64, kind ChannelKind, channelID int64, setting NotificationSetting) error {
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO user_channel_state (user_id, channel_kind, channel_id, notification_setting, updated_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (user_id, channel_kind, channel_id)
        DO UPDATE SET notification_setting = EXCLUDED.notification_setting, updated_at = NOW()
    `, userID, kind, channelID, setting)
	return err
}

// BulkGetUserChannelState returns every row for user — used by client to hydrate state on connect.
func (r *Repository) BulkGetUserChannelState(ctx context.Context, userID int64) ([]UserChannelState, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT user_id, channel_kind, channel_id, last_read_message_id, notification_setting, updated_at
          FROM user_channel_state
         WHERE user_id = $1
    `, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserChannelState
	for rows.Next() {
		var s UserChannelState
		if err := rows.Scan(&s.UserID, &s.ChannelKind, &s.ChannelID, &s.LastReadMessageID, &s.NotificationSetting, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
