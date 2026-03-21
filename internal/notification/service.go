package notification

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"time"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

var mentionRe = regexp.MustCompile(`<@(\d+)>`)

// Response is the JSON shape sent over WS and returned from the HTTP API.
type Response struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ActorUsername  string `json:"actor_username"`
	ActorAvatarURL string `json:"actor_avatar_url,omitempty"`
	ServerID       string `json:"server_id,omitempty"`
	ChannelID      string `json:"channel_id,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	DmChannelID    string `json:"dm_channel_id,omitempty"`
	Read           bool   `json:"read"`
	CreatedAt      string `json:"created_at"`
}

// ToResponse converts a DB notification to the wire format.
func ToResponse(n *db.Notification) Response {
	r := Response{
		ID:             strconv.FormatInt(n.ID, 10),
		Type:           n.Type,
		Title:          n.Title,
		Body:           n.Body,
		ActorUsername:  n.ActorUsername,
		ActorAvatarURL: n.ActorAvatarURL,
		Read:           n.Read,
		CreatedAt:      n.CreatedAt.Format(time.RFC3339),
	}
	if n.ServerID != nil {
		r.ServerID = strconv.FormatInt(*n.ServerID, 10)
	}
	if n.ChannelID != nil {
		r.ChannelID = strconv.FormatInt(*n.ChannelID, 10)
	}
	if n.MessageID != nil {
		r.MessageID = strconv.FormatInt(*n.MessageID, 10)
	}
	if n.DmChannelID != nil {
		r.DmChannelID = strconv.FormatInt(*n.DmChannelID, 10)
	}
	return r
}

// Service creates and broadcasts in-app notifications.
type Service struct {
	repo *db.Repository
	hub  *ws.Hub
}

// New creates a notification Service.
func New(repo *db.Repository, hub *ws.Hub) *Service {
	return &Service{repo: repo, hub: hub}
}

func (s *Service) create(ctx context.Context, n *db.Notification) {
	out, err := s.repo.CreateNotification(ctx, n)
	if err != nil {
		log.Printf("notification: create failed: %v", err)
		return
	}
	resp := ToResponse(out)
	b, err := json.Marshal(resp)
	if err != nil {
		log.Printf("notification: marshal failed: %v", err)
		return
	}
	userIDStr := strconv.FormatInt(n.UserID, 10)
	if err := s.hub.SendToUser(userIDStr, ws.EventNotificationCreate, b); err != nil {
		log.Printf("notification: SendToUser %s: %v", userIDStr, err)
	}
}

// NotifyMentions parses @mentions from content and creates mention notifications
// for each mentioned user (excluding the author themselves).
func (s *Service) NotifyMentions(ctx context.Context, authorID int64, authorUsername, authorAvatarURL, content, channelName string, serverID, channelID, messageID int64) {
	matches := mentionRe.FindAllStringSubmatch(content, -1)
	seen := map[int64]bool{}
	for _, m := range matches {
		uid, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || uid == authorID || seen[uid] {
			continue
		}
		seen[uid] = true
		sid, cid, mid := serverID, channelID, messageID
		s.create(ctx, &db.Notification{
			UserID:         uid,
			Type:           "mention",
			Title:          authorUsername + " mentioned you in #" + channelName,
			Body:           truncate(content, 120),
			ActorUsername:  authorUsername,
			ActorAvatarURL: authorAvatarURL,
			ServerID:       &sid,
			ChannelID:      &cid,
			MessageID:      &mid,
		})
	}
}

// NotifyDM creates a notification for a new DM received.
func (s *Service) NotifyDM(ctx context.Context, recipientID int64, authorUsername, authorAvatarURL string, dmChannelID int64) {
	dcid := dmChannelID
	s.create(ctx, &db.Notification{
		UserID:         recipientID,
		Type:           "dm",
		Title:          "New message from " + authorUsername,
		Body:           "",
		ActorUsername:  authorUsername,
		ActorAvatarURL: authorAvatarURL,
		DmChannelID:    &dcid,
	})
}

// NotifyFriendRequest creates a notification for a friend request received.
func (s *Service) NotifyFriendRequest(ctx context.Context, recipientID int64, senderUsername, senderAvatarURL string) {
	s.create(ctx, &db.Notification{
		UserID:         recipientID,
		Type:           "friend_request",
		Title:          senderUsername + " sent you a friend request",
		Body:           "",
		ActorUsername:  senderUsername,
		ActorAvatarURL: senderAvatarURL,
	})
}

// NotifyFriendAccept creates a notification when a friend request is accepted.
func (s *Service) NotifyFriendAccept(ctx context.Context, recipientID int64, acceptorUsername, acceptorAvatarURL string) {
	s.create(ctx, &db.Notification{
		UserID:         recipientID,
		Type:           "friend_accept",
		Title:          acceptorUsername + " accepted your friend request",
		Body:           "",
		ActorUsername:  acceptorUsername,
		ActorAvatarURL: acceptorAvatarURL,
	})
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
