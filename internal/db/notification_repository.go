package db

import (
	"context"
	"time"
)

// Notification is a persistent in-app notification for a user.
type Notification struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Type           string     `json:"type"` // mention, dm, friend_request, friend_accept
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	ActorUsername  string     `json:"actor_username"`
	ActorAvatarURL string     `json:"actor_avatar_url"`
	ServerID       *int64     `json:"server_id,omitempty"`
	ChannelID      *int64     `json:"channel_id,omitempty"`
	MessageID      *int64     `json:"message_id,omitempty"`
	DmChannelID    *int64     `json:"dm_channel_id,omitempty"`
	Read           bool       `json:"read"`
	CreatedAt      time.Time  `json:"created_at"`
}

// CreateNotification inserts a new notification and returns it with its ID and created_at set.
func (r *Repository) CreateNotification(ctx context.Context, n *Notification) (*Notification, error) {
	out := *n
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO notifications
			(user_id, type, title, body, actor_username, actor_avatar_url,
			 server_id, channel_id, message_id, dm_channel_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at
	`, n.UserID, n.Type, n.Title, n.Body, n.ActorUsername, n.ActorAvatarURL,
		n.ServerID, n.ChannelID, n.MessageID, n.DmChannelID,
	).Scan(&out.ID, &out.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUserNotifications returns recent notifications for a user, newest first.
func (r *Repository) GetUserNotifications(ctx context.Context, userID int64, limit int) ([]*Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, type, title, body, actor_username, actor_avatar_url,
		       server_id, channel_id, message_id, dm_channel_id, read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Notification
	for rows.Next() {
		n := &Notification{}
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body,
			&n.ActorUsername, &n.ActorAvatarURL,
			&n.ServerID, &n.ChannelID, &n.MessageID, &n.DmChannelID,
			&n.Read, &n.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	if result == nil {
		result = []*Notification{}
	}
	return result, rows.Err()
}

// CountUnreadNotifications returns the number of unread notifications for a user.
func (r *Repository) CountUnreadNotifications(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = FALSE`,
		userID,
	).Scan(&count)
	return count, err
}

// MarkNotificationRead marks a single notification as read (only if it belongs to userID).
func (r *Repository) MarkNotificationRead(ctx context.Context, notifID, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read = TRUE WHERE id = $1 AND user_id = $2`,
		notifID, userID,
	)
	return err
}

// MarkAllNotificationsRead marks all unread notifications for a user as read.
func (r *Repository) MarkAllNotificationsRead(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read = TRUE WHERE user_id = $1 AND read = FALSE`,
		userID,
	)
	return err
}
