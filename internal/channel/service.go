package channel

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"parley/internal/db"
)

// ChannelType represents the type of channel
type ChannelType int

const (
	ChannelTypeText  ChannelType = 0
	ChannelTypeVoice ChannelType = 1
)

// Channel represents a chat channel in the system
type Channel struct {
	ID         string      `json:"id"`
	ServerID   string      `json:"server_id"`
	Name       string      `json:"name"`
	Type       ChannelType `json:"type"`
	ParentID   *string     `json:"parent_id,omitempty"`
	CreatedAt  string      `json:"created_at"`
	UpdatedAt  string      `json:"updated_at"`
}

// ChannelService provides channel management operations
type ChannelService struct {
	repo *db.Repository
}

// NewChannelService creates a new ChannelService with the given repository
func NewChannelService(repo *db.Repository) *ChannelService {
	return &ChannelService{
		repo: repo,
	}
}

// CreateChannel creates a new channel
func (s *ChannelService) CreateChannel(ctx context.Context, serverID, name string, channelType int, parentID *string) (*Channel, error) {
	if name == "" {
		return nil, errors.New("channel name is required")
	}

	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}

	var parentIDInt *int64
	if parentID != nil {
		pid, err := strconv.ParseInt(*parentID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid parent ID")
		}
		parentIDInt = &pid
	}

	dbChannel := &db.Channel{
		ServerID:    serverIDInt,
		Name:        name,
		ChannelType: db.ChannelType(channelType),
		ParentID:    int64ToNullInt64(parentIDInt),
	}

	err = s.repo.CreateChannel(ctx, dbChannel)
	if err != nil {
		return nil, err
	}

	return dbChannelToChannel(dbChannel), nil
}

// GetChannel retrieves a channel by ID
func (s *ChannelService) GetChannel(ctx context.Context, id string) (*Channel, error) {
	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	channel, err := s.repo.GetChannelByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("channel not found")
		}
		return nil, err
	}

	return dbChannelToChannel(channel), nil
}

// GetServerChannels retrieves all channels for a server
func (s *ChannelService) GetServerChannels(ctx context.Context, serverID string) ([]*Channel, error) {
	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}

	channels, err := s.repo.GetChannelsByServerID(ctx, serverIDInt)
	if err != nil {
		return nil, err
	}

	result := make([]*Channel, len(channels))
	for i, ch := range channels {
		result[i] = dbChannelToChannel(ch)
	}

	return result, nil
}

// UpdateChannel updates a channel's name
func (s *ChannelService) UpdateChannel(ctx context.Context, id, name string) (*Channel, error) {
	if name == "" {
		return nil, errors.New("channel name is required")
	}

	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	channel, err := s.repo.GetChannelByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("channel not found")
		}
		return nil, err
	}

	channel.Name = name

	err = s.repo.UpdateChannel(ctx, channel)
	if err != nil {
		return nil, err
	}

	return dbChannelToChannel(channel), nil
}

// DeleteChannel deletes a channel by ID. Only the server owner may delete channels.
func (s *ChannelService) DeleteChannel(ctx context.Context, id string, userID string) error {
	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return errors.New("invalid channel ID")
	}

	ch, err := s.repo.GetChannelByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("channel not found")
		}
		return err
	}

	srv, err := s.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		return err
	}

	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return errors.New("invalid user ID")
	}
	if srv.OwnerID != userIDInt {
		return errors.New("forbidden")
	}

	err = s.repo.DeleteChannel(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("channel not found")
		}
		return err
	}

	return nil
}

// dbChannelToChannel converts a db.Channel to a Channel for API responses
func dbChannelToChannel(dbCh *db.Channel) *Channel {
	ch := &Channel{
		ID:         strconv.FormatInt(dbCh.ID, 10),
		ServerID:   strconv.FormatInt(dbCh.ServerID, 10),
		Name:       dbCh.Name,
		Type:       ChannelType(dbCh.ChannelType),
		CreatedAt:  dbCh.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  dbCh.CreatedAt.Format(time.RFC3339), // DB doesn't track updated_at for channels
	}

	if dbCh.ParentID.Valid {
		parentID := strconv.FormatInt(dbCh.ParentID.Int64, 10)
		ch.ParentID = &parentID
	}

	return ch
}

// int64ToNullInt64 converts *int64 to sql.NullInt64
func int64ToNullInt64(val *int64) sql.NullInt64 {
	if val == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: *val, Valid: true}
}