package message

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"parley/internal/db"
	"parley/internal/permissions"
	"parley/internal/validation"
)

// Reaction represents aggregated emoji reactions for a message.
type Reaction struct {
	Emoji   string   `json:"emoji"`
	Count   int      `json:"count"`
	UserIDs []string `json:"user_ids"`
}

// Message represents a message in the system
type Message struct {
	ID                 string     `json:"id"`
	ChannelID          string     `json:"channel_id"`
	AuthorID           string     `json:"author_id"`
	AuthorUsername     string     `json:"author_username"`
	AuthorDisplayName  string     `json:"author_display_name,omitempty"`
	AuthorAvatarURL    string     `json:"author_avatar_url,omitempty"`
	AuthorIsBot        bool       `json:"author_is_bot,omitempty"`
	ViaApi             bool       `json:"via_api,omitempty"`
	Content         string     `json:"content"`
	Nonce           string     `json:"nonce,omitempty"`
	ParentID        *int64     `json:"parent_id,omitempty"`
	AttachmentURL   string     `json:"attachment_url,omitempty"`
	AttachmentName  string     `json:"attachment_name,omitempty"`
	AttachmentType  string     `json:"attachment_type,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Reactions       []Reaction `json:"reactions"`
}

// MessageService provides message management operations
type MessageService struct {
	mu          sync.RWMutex
	repo        *db.Repository
	broadcaster Broadcaster
}

// NewMessageService creates a new MessageService with the given repository
func NewMessageService(repo *db.Repository) *MessageService {
	return &MessageService{
		repo:        repo,
		broadcaster: nil,
	}
}

// SetBroadcaster sets the broadcaster for the service
func (s *MessageService) SetBroadcaster(b Broadcaster) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcaster = b
}

// SendMessage creates a new message in a channel.
// nonce is a client-generated UUID used for deduplication; pass "" if not provided.
// parentID is an optional message ID for nested replies; pass "" for top-level messages.
func (s *MessageService) SendMessage(ctx context.Context, channelID, authorID, content, nonce, attachmentURL, attachmentName, attachmentType, parentID string) (*Message, error) {
	if channelID == "" {
		return nil, errors.New("channel ID is required")
	}
	if authorID == "" {
		return nil, errors.New("author ID is required")
	}
	if content == "" && attachmentURL == "" {
		return nil, errors.New("content or attachment is required")
	}
	if validation.HasSpoofedLink(content) {
		return nil, errors.New("message contains a spoofed link")
	}

	// Convert channelID from string to int64
	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	// Convert authorID from string to int64
	authorIDInt, err := strconv.ParseInt(authorID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid author ID")
	}

	// Check SendMessages permission at channel level.
	ch, err := s.repo.GetChannelByID(ctx, channelIDInt)
	if err != nil {
		return nil, errors.New("channel not found")
	}
	srv, err := s.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		return nil, errors.New("server not found")
	}
	canSend, err := permissions.HasChannelPermission(ctx, s.repo, srv.ID, authorIDInt, srv.OwnerID, channelIDInt, permissions.PermSendMessages)
	if err != nil {
		return nil, err
	}
	if !canSend {
		return nil, errors.New("forbidden")
	}

	viaAPI, _ := ctx.Value("isAPIKeyAuth").(bool)

	var parentIDPtr *int64
	if parentID != "" {
		pid, err := strconv.ParseInt(parentID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid parent ID")
		}
		parentIDPtr = &pid
	}

	dbMsg, err := s.repo.CreateMessage(ctx, channelIDInt, authorIDInt, content, nonce, attachmentURL, attachmentName, attachmentType, viaAPI, parentIDPtr)
	if err != nil {
		return nil, err
	}

	// Look up author username, display_name, avatar, and bot status
	var authorUsername, authorDisplayName, authorAvatarURL string
	var authorIsBot bool
	if err := s.repo.DB().QueryRowContext(ctx, "SELECT username, COALESCE(display_name, ''), COALESCE(avatar_url, ''), is_bot FROM users WHERE id = $1", authorIDInt).Scan(&authorUsername, &authorDisplayName, &authorAvatarURL, &authorIsBot); err != nil {
		log.Printf("SendMessage: failed to fetch user info for author %d: %v", authorIDInt, err)
	}

	msg := &Message{
		ID:                strconv.FormatInt(dbMsg.ID, 10),
		ChannelID:         channelID,
		AuthorID:          authorID,
		AuthorUsername:    authorUsername,
		AuthorDisplayName: authorDisplayName,
		AuthorAvatarURL:   authorAvatarURL,
		AuthorIsBot:       authorIsBot,
		ViaApi:          viaAPI,
		Content:         content,
		Nonce:           dbMsg.Nonce,
		ParentID:        dbMsg.ParentID,
		AttachmentURL:   dbMsg.AttachmentURL,
		AttachmentName:  dbMsg.AttachmentName,
		AttachmentType:  dbMsg.AttachmentType,
		CreatedAt:       dbMsg.CreatedAt,
		UpdatedAt:       dbMsg.UpdatedAt,
		Reactions:       []Reaction{},
	}

	// Broadcast the message if a broadcaster is set
	s.mu.RLock()
	if s.broadcaster != nil {
		s.broadcaster.BroadcastToChannel(channelID, "MESSAGE_CREATE", msg)
	}
	s.mu.RUnlock()

	return msg, nil
}

// GetMessage retrieves a message by ID
func (s *MessageService) GetMessage(ctx context.Context, id string) (*Message, error) {
	// Convert id from string to int64
	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errors.New("invalid message ID")
	}

	dbMsg, err := s.repo.GetMessageByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("message not found")
		}
		return nil, err
	}

	var authorUsername string
	if err := s.repo.DB().QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", dbMsg.AuthorID).Scan(&authorUsername); err != nil {
		log.Printf("GetMessage: failed to fetch username for author %d: %v", dbMsg.AuthorID, err)
	}

	return &Message{
		ID:             strconv.FormatInt(dbMsg.ID, 10),
		ChannelID:      strconv.FormatInt(dbMsg.ChannelID, 10),
		AuthorID:       strconv.FormatInt(dbMsg.AuthorID, 10),
		AuthorUsername: authorUsername,
		Content:        dbMsg.Content,
		CreatedAt:      dbMsg.CreatedAt,
		UpdatedAt:      dbMsg.UpdatedAt,
		Reactions:      []Reaction{},
	}, nil
}

// GetChannelMessages retrieves messages for a channel with cursor-based pagination.
// beforeID: if > 0, returns messages older than that ID. If 0, returns the latest messages.
// userID is used for ViewChannel permission check; pass "" to skip the check.
func (s *MessageService) GetChannelMessages(ctx context.Context, channelID, userID string, limit int, beforeID int64) ([]*Message, error) {
	if channelID == "" {
		return nil, errors.New("channel ID is required")
	}

	if limit <= 0 {
		limit = 50
	}

	// Convert channelID from string to int64
	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	// Check ViewChannel permission.
	if userID != "" {
		userIDInt, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid user ID")
		}
		ch, err := s.repo.GetChannelByID(ctx, channelIDInt)
		if err != nil {
			return nil, errors.New("channel not found")
		}
		srv, err := s.repo.GetServerByID(ctx, ch.ServerID)
		if err != nil {
			return nil, errors.New("server not found")
		}
		canView, err := permissions.HasChannelPermission(ctx, s.repo, srv.ID, userIDInt, srv.OwnerID, channelIDInt, permissions.PermViewChannel)
		if err != nil {
			return nil, err
		}
		if !canView {
			return nil, errors.New("channel not found")
		}
	}

	dbMessages, err := s.repo.GetChannelMessages(ctx, channelIDInt, limit, beforeID)
	if err != nil {
		return nil, err
	}

	// Collect message IDs for batch reaction fetch
	messageIDs := make([]int64, len(dbMessages))
	for i, dbMsg := range dbMessages {
		messageIDs[i] = dbMsg.ID
	}

	reactionMap, err := s.repo.GetReactionsForMessages(ctx, messageIDs)
	if err != nil {
		log.Printf("GetChannelMessages: failed to fetch reactions: %v", err)
		reactionMap = map[int64][]db.ReactionGroup{}
	}

	messages := make([]*Message, 0, len(dbMessages))
	for _, dbMsg := range dbMessages {
		reactions := []Reaction{}
		if groups, ok := reactionMap[dbMsg.ID]; ok {
			for _, g := range groups {
				reactions = append(reactions, Reaction{
					Emoji:   g.Emoji,
					Count:   g.Count,
					UserIDs: g.UserIDs,
				})
			}
		}
		messages = append(messages, &Message{
			ID:              strconv.FormatInt(dbMsg.ID, 10),
			ChannelID:       channelID,
			AuthorID:        strconv.FormatInt(dbMsg.AuthorID, 10),
			AuthorUsername:  dbMsg.AuthorUsername,
			AuthorAvatarURL: dbMsg.AuthorAvatarURL,
			AuthorIsBot:     dbMsg.AuthorIsBot,
			ViaApi:          dbMsg.ViaApi,
			Content:         dbMsg.Content,
			Nonce:           dbMsg.Nonce,
			ParentID:        dbMsg.ParentID,
			AttachmentURL:   dbMsg.AttachmentURL,
			AttachmentName:  dbMsg.AttachmentName,
			AttachmentType:  dbMsg.AttachmentType,
			CreatedAt:       dbMsg.CreatedAt,
			UpdatedAt:       dbMsg.UpdatedAt,
			Reactions:       reactions,
		})
	}

	return messages, nil
}

// EditMessage updates a message's content
func (s *MessageService) EditMessage(ctx context.Context, id, content string) (*Message, error) {
	if content == "" {
		return nil, errors.New("content is required")
	}
	if validation.HasSpoofedLink(content) {
		return nil, errors.New("message contains a spoofed link")
	}

	// Convert id from string to int64
	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errors.New("invalid message ID")
	}

	// Save current version and update message via repository
	dbMsg, err := s.repo.EditMessage(ctx, idInt, content)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("message not found")
		}
		return nil, err
	}

	var authorUsername string
	if err := s.repo.DB().QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", dbMsg.AuthorID).Scan(&authorUsername); err != nil {
		log.Printf("EditMessage: failed to fetch username for author %d: %v", dbMsg.AuthorID, err)
	}

	// Fetch current reactions to include in the broadcast
	reactionMap, _ := s.repo.GetReactionsForMessages(ctx, []int64{dbMsg.ID})
	reactions := []Reaction{}
	if groups, ok := reactionMap[dbMsg.ID]; ok {
		for _, g := range groups {
			reactions = append(reactions, Reaction{Emoji: g.Emoji, Count: g.Count, UserIDs: g.UserIDs})
		}
	}

	msg := &Message{
		ID:             id,
		ChannelID:      strconv.FormatInt(dbMsg.ChannelID, 10),
		AuthorID:       strconv.FormatInt(dbMsg.AuthorID, 10),
		AuthorUsername: authorUsername,
		Content:        content,
		CreatedAt:      dbMsg.CreatedAt,
		UpdatedAt:      dbMsg.UpdatedAt,
		Reactions:      reactions,
	}

	// Broadcast the update if a broadcaster is set
	s.mu.RLock()
	if s.broadcaster != nil {
		s.broadcaster.BroadcastToChannel(msg.ChannelID, "MESSAGE_UPDATE", msg)
	}
	s.mu.RUnlock()

	return msg, nil
}

// DeleteMessage deletes a message by ID
func (s *MessageService) DeleteMessage(ctx context.Context, id string) error {
	// Convert id from string to int64
	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return errors.New("invalid message ID")
	}

	// First get the existing message to get channelID for broadcast
	dbMsg, err := s.repo.GetMessageByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("message not found")
		}
		return err
	}

	channelID := strconv.FormatInt(dbMsg.ChannelID, 10)

	err = s.repo.DeleteMessage(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("message not found")
		}
		return err
	}

	// Broadcast the deletion with channel_id so clients can route it correctly
	s.mu.RLock()
	if s.broadcaster != nil {
		s.broadcaster.BroadcastToChannel(channelID, "MESSAGE_DELETE", map[string]string{
			"id":         id,
			"channel_id": channelID,
		})
	}
	s.mu.RUnlock()

	return nil
}

// ToggleReaction adds or removes a user's reaction on a message and broadcasts the event.
func (s *MessageService) ToggleReaction(ctx context.Context, messageID, userID, emoji string) error {
	msgIDInt, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return errors.New("invalid message ID")
	}
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return errors.New("invalid user ID")
	}
	if emoji == "" {
		return errors.New("emoji is required")
	}

	// Need the message's channel for broadcasting
	dbMsg, err := s.repo.GetMessageByID(ctx, msgIDInt)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("message not found")
		}
		return err
	}

	added, err := s.repo.ToggleReaction(ctx, msgIDInt, userIDInt, emoji)
	if err != nil {
		return err
	}

	channelID := strconv.FormatInt(dbMsg.ChannelID, 10)
	eventType := "REACTION_REMOVE"
	if added {
		eventType = "REACTION_ADD"
	}

	s.mu.RLock()
	if s.broadcaster != nil {
		s.broadcaster.BroadcastToChannel(channelID, eventType, map[string]string{
			"message_id": messageID,
			"channel_id": channelID,
			"user_id":    userID,
			"emoji":      emoji,
		})
	}
	s.mu.RUnlock()

	return nil
}

// GetMessageVersions returns the edit history for a message.
func (s *MessageService) GetMessageVersions(ctx context.Context, messageID string) ([]db.MessageVersion, error) {
	id, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid message ID")
	}
	return s.repo.GetMessageVersions(ctx, id)
}

// CanManageMessage returns true if the given user has permission to delete/manage the message.
// Returns true if the user is the message author OR has MANAGE_MESSAGES permission in the server.
func (s *MessageService) CanManageMessage(ctx context.Context, messageID, userID string) (bool, error) {
	msgIDInt, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return false, errors.New("invalid message ID")
	}
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return false, errors.New("invalid user ID")
	}

	msg, err := s.repo.GetMessageByID(ctx, msgIDInt)
	if err != nil {
		return false, err
	}
	if msg.AuthorID == userIDInt {
		return true, nil
	}

	// Look up the channel to get the server ID
	ch, err := s.repo.GetChannelByID(ctx, msg.ChannelID)
	if err != nil {
		return false, err
	}
	srv, err := s.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		return false, err
	}

	return permissions.HasChannelPermission(ctx, s.repo, srv.ID, userIDInt, srv.OwnerID, msg.ChannelID, permissions.PermManageMessages)
}
