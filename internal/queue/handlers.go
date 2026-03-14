package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"

	"parley/internal/db"
)

// MessageEvent represents a message event from the queue
type MessageEvent struct {
	Type      string `json:"type"`
	MessageID int64  `json:"message_id"`
	ChannelID int64  `json:"channel_id"`
	AuthorID  int64  `json:"author_id"`
	Content   string `json:"content"`
}

// Notification represents a notification to send to a user
type Notification struct {
	Type      string          `json:"type"`
	UserID    int64           `json:"user_id"`
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
}

// WSHubInterface defines the interface for sending messages to users via WebSocket
type WSHubInterface interface {
	SendToUser(userID string, event string, data []byte) error
}

// MessageProcessor handles message processing from the queue
type MessageProcessor struct {
	db     *sql.DB
	hub    WSHubInterface
}

// NewMessageProcessor creates a new message processor
func NewMessageProcessor(dbConn *sql.DB, hub WSHubInterface) *MessageProcessor {
	return &MessageProcessor{
		db:  dbConn,
		hub: hub,
	}
}

// StartMessageProcessor starts consuming messages from the message.processing queue
func StartMessageProcessor(q *RabbitMQ, dbConn *sql.DB, hub WSHubInterface) error {
	processor := NewMessageProcessor(dbConn, hub)

	log.Println("Starting message processor...")

	err := q.Consume(QueueMessageProcessing, processor.handleMessage)
	if err != nil {
		return err
	}

	log.Println("Message processor started, consuming from", QueueMessageProcessing)
	return nil
}

// handleMessage processes a single message from the queue
func (mp *MessageProcessor) handleMessage(body []byte) {
	var event MessageEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("Failed to unmarshal message event: %v", err)
		return
	}

	log.Printf("Processing message event: type=%s, message_id=%d, channel_id=%d",
		event.Type, event.MessageID, event.ChannelID)

	ctx := context.Background()

	switch event.Type {
	case RoutingKeyMessageCreate:
		mp.handleMessageCreate(ctx, &event)
	case RoutingKeyMessageUpdate:
		mp.handleMessageUpdate(ctx, &event)
	case RoutingKeyMessageDelete:
		mp.handleMessageDelete(ctx, &event)
	default:
		log.Printf("Unknown message event type: %s", event.Type)
	}
}

// handleMessageCreate handles new message events
func (mp *MessageProcessor) handleMessageCreate(ctx context.Context, event *MessageEvent) {
	// Get channel members to notify them
	rows, err := mp.db.QueryContext(ctx, `
		SELECT DISTINCT sm.user_id
		FROM server_members sm
		JOIN channels c ON c.server_id = sm.server_id
		WHERE c.id = $1
	`, event.ChannelID)
	if err != nil {
		log.Printf("Failed to get channel members: %v", err)
		return
	}
	defer rows.Close()

	// Prepare notification data
	notificationData, err := json.Marshal(map[string]interface{}{
		"message_id": event.MessageID,
		"channel_id": event.ChannelID,
		"author_id":  event.AuthorID,
		"content":    event.Content,
	})
	if err != nil {
		log.Printf("Failed to marshal notification data: %v", err)
		return
	}

	// Notify each member who is not the author
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			log.Printf("Failed to scan user_id: %v", err)
			continue
		}

		// Skip the message author
		if userID == event.AuthorID {
			continue
		}

		// Send notification via WebSocket
		if mp.hub != nil {
			err := mp.hub.SendToUser(intToString(userID), "new_message", notificationData)
			if err != nil {
				log.Printf("Failed to send notification to user %d: %v", userID, err)
			}
		}
	}
}

// handleMessageUpdate handles message update events
func (mp *MessageProcessor) handleMessageUpdate(ctx context.Context, event *MessageEvent) {
	// Get channel members to notify them
	rows, err := mp.db.QueryContext(ctx, `
		SELECT DISTINCT sm.user_id
		FROM server_members sm
		JOIN channels c ON c.server_id = sm.server_id
		WHERE c.id = $1
	`, event.ChannelID)
	if err != nil {
		log.Printf("Failed to get channel members: %v", err)
		return
	}
	defer rows.Close()

	// Prepare notification data
	notificationData, err := json.Marshal(map[string]interface{}{
		"message_id": event.MessageID,
		"channel_id": event.ChannelID,
		"author_id":  event.AuthorID,
		"content":    event.Content,
	})
	if err != nil {
		log.Printf("Failed to marshal notification data: %v", err)
		return
	}

	// Notify each member who is not the author
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			log.Printf("Failed to scan user_id: %v", err)
			continue
		}

		if userID == event.AuthorID {
			continue
		}

		if mp.hub != nil {
			err := mp.hub.SendToUser(intToString(userID), "message_updated", notificationData)
			if err != nil {
				log.Printf("Failed to send update notification to user %d: %v", userID, err)
			}
		}
	}
}

// handleMessageDelete handles message delete events
func (mp *MessageProcessor) handleMessageDelete(ctx context.Context, event *MessageEvent) {
	// Get channel members to notify them
	rows, err := mp.db.QueryContext(ctx, `
		SELECT DISTINCT sm.user_id
		FROM server_members sm
		JOIN channels c ON c.server_id = sm.server_id
		WHERE c.id = $1
	`, event.ChannelID)
	if err != nil {
		log.Printf("Failed to get channel members: %v", err)
		return
	}
	defer rows.Close()

	// Prepare notification data
	notificationData, err := json.Marshal(map[string]interface{}{
		"message_id": event.MessageID,
		"channel_id": event.ChannelID,
	})
	if err != nil {
		log.Printf("Failed to marshal notification data: %v", err)
		return
	}

	// Notify each member who is not the author
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			log.Printf("Failed to scan user_id: %v", err)
			continue
		}

		if userID == event.AuthorID {
			continue
		}

		if mp.hub != nil {
			err := mp.hub.SendToUser(intToString(userID), "message_deleted", notificationData)
			if err != nil {
				log.Printf("Failed to send delete notification to user %d: %v", userID, err)
			}
		}
	}
}

// NotificationWorker handles notification delivery
type NotificationWorker struct {
	hub WSHubInterface
}

// NewNotificationWorker creates a new notification worker
func NewNotificationWorker(hub WSHubInterface) *NotificationWorker {
	return &NotificationWorker{
		hub: hub,
	}
}

// StartNotificationWorker starts consuming notifications from the notifications queue
func StartNotificationWorker(q *RabbitMQ, hub WSHubInterface) error {
	worker := NewNotificationWorker(hub)

	log.Println("Starting notification worker...")

	err := q.Consume(QueueNotificationsSend, worker.handleNotification)
	if err != nil {
		return err
	}

	log.Println("Notification worker started, consuming from", QueueNotificationsSend)
	return nil
}

// handleNotification processes a single notification from the queue
func (nw *NotificationWorker) handleNotification(body []byte) {
	var notification Notification
	if err := json.Unmarshal(body, &notification); err != nil {
		log.Printf("Failed to unmarshal notification: %v", err)
		return
	}

	log.Printf("Processing notification: type=%s, user_id=%d, event=%s",
		notification.Type, notification.UserID, notification.Event)

	if nw.hub == nil {
		log.Println("No hub available for notification delivery")
		return
	}

	// Send the notification via WebSocket
	err := nw.hub.SendToUser(intToString(notification.UserID), notification.Event, notification.Data)
	if err != nil {
		log.Printf("Failed to send notification to user %d: %v", notification.UserID, err)
		return
	}

	log.Printf("Notification sent to user %d: event=%s", notification.UserID, notification.Event)
}

// intToString converts an int64 to string
func intToString(i int64) string {
	return strconv.FormatInt(i, 10)
}

// GetChannelMembers retrieves user IDs of all members in a channel
func GetChannelMembers(ctx context.Context, db *sql.DB, channelID int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT sm.user_id
		FROM server_members sm
		JOIN channels c ON c.server_id = sm.server_id
		WHERE c.id = $1
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}

	return userIDs, rows.Err()
}

// IsUserOnline checks if a user has any active WebSocket connections
// This is a placeholder - in a real implementation, you'd query the hub's state
func IsUserOnline(hub WSHubInterface, userID string) bool {
	// In a real implementation, you'd check the hub's userToClient map
	// For now, we assume offline and send notifications anyway
	return false
}

// SendOfflineNotification sends a notification for an offline user
// In a real implementation, this would store the notification for later delivery
// or send it via email/push notification
func SendOfflineNotification(hub WSHubInterface, userID int64, event string, data []byte) error {
	// For offline users, we could:
	// 1. Store in database and deliver when they come online
	// 2. Send email notification
	// 3. Send push notification
	log.Printf("User %d is offline, notification queued: event=%s", userID, event)
	return nil
}

// Ensure db is imported
var _ = db.User{}