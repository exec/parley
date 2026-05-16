package notification

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

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

// BlockChecker lets the notification service skip delivering notifications
// when the recipient has blocked the actor. Wired via SetBlockChecker from
// cmd/api/routes.go to keep internal/notification from importing internal/friend.
type BlockChecker interface {
	IsBlocked(ctx context.Context, blockerID, blockedID int64) (bool, error)
}

// Service creates and broadcasts in-app notifications.
type Service struct {
	repo   *db.Repository
	hub    *ws.Hub
	blocks BlockChecker // optional; nil-safe — gating skipped when unset
}

// New creates a notification Service.
func New(repo *db.Repository, hub *ws.Hub) *Service {
	return &Service{repo: repo, hub: hub}
}

// SetBlockChecker wires the friend service so notifications can be filtered
// by recipient block list.
func (s *Service) SetBlockChecker(b BlockChecker) { s.blocks = b }

// suppressed returns true if recipientID has blocked actorID. Failures fall
// open (deliver) — better to over-notify than to silently drop a real event
// because Redis or the DB hiccupped.
func (s *Service) suppressed(ctx context.Context, recipientID, actorID int64) bool {
	if s.blocks == nil || actorID == 0 {
		return false
	}
	blocked, err := s.blocks.IsBlocked(ctx, recipientID, actorID)
	if err != nil {
		log.Printf("notification: block check failed: %v", err)
		return false
	}
	return blocked
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
//
// Each recipient must be a member of the source server. Without this check, a
// stranger with SendMessages on any server they share with no one could fan
// arbitrary `<@uid>` strings out as cross-server notification spam. (audit #10)
func (s *Service) NotifyMentions(ctx context.Context, authorID int64, authorUsername, authorAvatarURL, content, channelName string, serverID, channelID, messageID int64) {
	matches := mentionRe.FindAllStringSubmatch(content, -1)

	// Parse + dedup sequentially: the `seen` map is not safe for concurrent
	// use, so collect the surviving deduped uids before fanning out.
	seen := map[int64]bool{}
	uids := make([]int64, 0, len(matches))
	for _, m := range matches {
		uid, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || uid == authorID || seen[uid] {
			continue
		}
		seen[uid] = true
		uids = append(uids, uid)
	}

	// Process recipients concurrently: each recipient's GetMember +
	// suppression + create work is independent blocking DB I/O. errgroup
	// with SetLimit bounds goroutine fan-out for large mention lists.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for _, uid := range uids {
		uid := uid
		g.Go(func() error {
			// Skip notifications for users who aren't members of the source
			// server. GetMember returns ErrNotFound for non-members.
			if _, err := s.repo.GetMember(gctx, serverID, uid); err != nil {
				return nil
			}
			// Recipient blocked the author? Drop the notification but let the
			// message through (defense-in-depth — the message itself isn't
			// gated server-wide, only its notification spam vector is).
			if s.suppressed(gctx, uid, authorID) {
				return nil
			}
			// Each goroutine builds its own sid/cid/mid so the Notification
			// pointer fields never alias across goroutines.
			sid, cid, mid := serverID, channelID, messageID
			s.create(gctx, &db.Notification{
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
			return nil
		})
	}
	// Per-user steps never abort siblings; ignore the aggregate error.
	_ = g.Wait()
}

// NotifyDM creates a notification for a new DM received. authorID is used to
// suppress the notification if the recipient has blocked the author.
func (s *Service) NotifyDM(ctx context.Context, recipientID, authorID int64, authorUsername, authorAvatarURL string, dmChannelID int64) {
	if s.suppressed(ctx, recipientID, authorID) {
		return
	}
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
// senderID is used for the recipient's block-list check.
func (s *Service) NotifyFriendRequest(ctx context.Context, recipientID, senderID int64, senderUsername, senderAvatarURL string) {
	if s.suppressed(ctx, recipientID, senderID) {
		return
	}
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
// accepterID is included for symmetry with the suppression hook (a user who
// blocks the original sender mid-request shouldn't get an "X accepted" toast).
func (s *Service) NotifyFriendAccept(ctx context.Context, recipientID, accepterID int64, acceptorUsername, acceptorAvatarURL string) {
	if s.suppressed(ctx, recipientID, accepterID) {
		return
	}
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
