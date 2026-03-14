package message

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"parley/internal/db"
)

// Reaction represents aggregated emoji reactions for a message.
type Reaction struct {
	Emoji   string   `json:"emoji"`
	Count   int      `json:"count"`
	UserIDs []string `json:"user_ids"`
}

// Message represents a message in the system
type Message struct {
	ID             string     `json:"id"`
	ChannelID      string     `json:"channel_id"`
	AuthorID       string     `json:"author_id"`
	AuthorUsername string     `json:"author_username"`
	Content        string     `json:"content"`
	Nonce          string     `json:"nonce,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Reactions      []Reaction `json:"reactions"`
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
func (s *MessageService) SendMessage(ctx context.Context, channelID, authorID, content, nonce string) (*Message, error) {
	if channelID == "" {
		return nil, errors.New("channel ID is required")
	}
	if authorID == "" {
		return nil, errors.New("author ID is required")
	}
	if content == "" {
		return nil, errors.New("content is required")
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

	now := time.Now()
	dbMsg := &db.Message{
		ChannelID: channelIDInt,
		AuthorID:  authorIDInt,
		Content:   content,
		Nonce:     nonce,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = s.repo.CreateMessage(ctx, dbMsg)
	if err != nil {
		return nil, err
	}

	// Look up author username
	var authorUsername string
	if err := s.repo.DB().QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", authorIDInt).Scan(&authorUsername); err != nil {
		log.Printf("SendMessage: failed to fetch username for author %d: %v", authorIDInt, err)
	}

	msg := &Message{
		ID:             strconv.FormatInt(dbMsg.ID, 10),
		ChannelID:      channelID,
		AuthorID:       authorID,
		AuthorUsername: authorUsername,
		Content:        content,
		Nonce:          dbMsg.Nonce,
		CreatedAt:      dbMsg.CreatedAt,
		UpdatedAt:      dbMsg.UpdatedAt,
		Reactions:      []Reaction{},
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

// GetChannelMessages retrieves messages for a channel with pagination
func (s *MessageService) GetChannelMessages(ctx context.Context, channelID string, limit, offset int) ([]*Message, error) {
	if channelID == "" {
		return nil, errors.New("channel ID is required")
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Convert channelID from string to int64
	channelIDInt, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	dbMessages, err := s.repo.GetChannelMessages(ctx, channelIDInt, limit, offset)
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
			ID:             strconv.FormatInt(dbMsg.ID, 10),
			ChannelID:      channelID,
			AuthorID:       strconv.FormatInt(dbMsg.AuthorID, 10),
			AuthorUsername: dbMsg.AuthorUsername,
			Content:        dbMsg.Content,
			CreatedAt:      dbMsg.CreatedAt,
			UpdatedAt:      dbMsg.UpdatedAt,
			Reactions:      reactions,
		})
	}

	return messages, nil
}

// EditMessage updates a message's content
func (s *MessageService) EditMessage(ctx context.Context, id, content string) (*Message, error) {
	if content == "" {
		return nil, errors.New("content is required")
	}

	// Convert id from string to int64
	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errors.New("invalid message ID")
	}

	// First get the existing message to get channelID
	dbMsg, err := s.repo.GetMessageByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("message not found")
		}
		return nil, err
	}

	// Update the message
	dbMsg.Content = content
	dbMsg.UpdatedAt = time.Now()

	query := `UPDATE messages SET content = $1, updated_at = $2 WHERE id = $3`
	_, err = s.repo.DB().ExecContext(ctx, query, dbMsg.Content, dbMsg.UpdatedAt, dbMsg.ID)
	if err != nil {
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
