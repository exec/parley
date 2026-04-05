package channel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"parley/internal/db"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

// ChannelType represents the type of channel
type ChannelType int

const (
	ChannelTypeText     ChannelType = 0
	ChannelTypeVoice    ChannelType = 1
	ChannelTypeBin      ChannelType = 2
	ChannelTypeCategory ChannelType = 3
)

// Channel represents a chat channel in the system
type Channel struct {
	ID        string      `json:"id"`
	ServerID  string      `json:"server_id"`
	Name      string      `json:"name"`
	Type      ChannelType `json:"type"`
	Position  int         `json:"position"`
	ParentID  *string     `json:"parent_id,omitempty"`
	Topic     string      `json:"topic,omitempty"`
	Synced    bool        `json:"synced"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
}

// ChannelService provides channel management operations
type ChannelService struct {
	repo *db.Repository
	hub  *ws.Hub
}

// NewChannelService creates a new ChannelService with the given repository
func NewChannelService(repo *db.Repository) *ChannelService {
	return &ChannelService{
		repo: repo,
	}
}

// SetHub sets the WebSocket hub for broadcasting channel events
func (s *ChannelService) SetHub(hub *ws.Hub) {
	s.hub = hub
}

// Sentinel errors returned by ChannelService methods.
var (
	ErrForbidden       = errors.New("forbidden")
	ErrServerNotFound  = errors.New("server not found")
	ErrChannelNotFound = errors.New("channel not found")
)

const maxChannelNameLen = 100

// CreateChannel creates a new channel. userID must be the server owner or have MANAGE_CHANNELS.
func (s *ChannelService) CreateChannel(ctx context.Context, serverID, name string, channelType int, parentID *string, topic string, userID string) (*Channel, error) {
	if name == "" {
		return nil, errors.New("channel name is required")
	}
	if len(name) > maxChannelNameLen {
		return nil, errors.New("channel name must be 100 characters or fewer")
	}

	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	srv, err := s.repo.GetServerByID(ctx, serverIDInt)
	if err != nil {
		return nil, ErrServerNotFound
	}
	allowed, err := permissions.HasPermission(ctx, s.repo, serverIDInt, userIDInt, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
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
		Topic:       topic,
	}

	err = s.repo.CreateChannel(ctx, dbChannel)
	if err != nil {
		return nil, err
	}

	// If channel was created inside a category, copy the category's overwrites.
	if parentIDInt != nil {
		if copyErr := s.repo.CopyOverwrites(ctx, *parentIDInt, dbChannel.ID); copyErr != nil {
			log.Printf("CreateChannel: failed to copy overwrites from category %d: %v", *parentIDInt, copyErr)
		}
	}

	ch := dbChannelToChannel(dbChannel)
	if s.hub != nil {
		if payload, err := json.Marshal(ch); err == nil {
			s.hub.BroadcastToChannel("server:"+serverID, ws.EventChannelCreate, payload)
		} else {
			log.Printf("Failed to marshal CHANNEL_CREATE event: %v", err)
		}
	}
	return ch, nil
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
			return nil, ErrChannelNotFound
		}
		return nil, err
	}

	return dbChannelToChannel(channel), nil
}

// GetServerChannels retrieves all channels for a server, filtered by ViewChannel permission.
// userID and ownerID are used to compute per-channel permissions. Pass "" for both to skip filtering.
func (s *ChannelService) GetServerChannels(ctx context.Context, serverID, userID, ownerID string) ([]*Channel, error) {
	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}

	channels, err := s.repo.GetChannelsByServerID(ctx, serverIDInt)
	if err != nil {
		return nil, err
	}

	// If no userID provided, return all channels (e.g. internal/admin calls).
	if userID == "" {
		result := make([]*Channel, len(channels))
		for i, ch := range channels {
			result[i] = dbChannelToChannel(ch)
		}
		return result, nil
	}

	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}
	ownerIDInt, err := strconv.ParseInt(ownerID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid owner ID")
	}

	// Build a set of visible channel IDs.
	visibleIDs := make(map[string]bool, len(channels))
	for _, ch := range channels {
		chIDStr := strconv.FormatInt(ch.ID, 10)
		canView, err := permissions.HasChannelPermission(ctx, s.repo, serverIDInt, userIDInt, ownerIDInt, ch.ID, permissions.PermViewChannel)
		if err != nil {
			// On error, default to visible.
			visibleIDs[chIDStr] = true
			continue
		}
		visibleIDs[chIDStr] = canView
	}

	// Filter out invisible non-category channels; filter out categories with no visible children.
	var result []*Channel
	for _, ch := range channels {
		chIDStr := strconv.FormatInt(ch.ID, 10)
		chType := ChannelType(ch.ChannelType)
		if chType == ChannelTypeCategory {
			// Include category only if at least one child is visible.
			hasVisibleChild := false
			for _, child := range channels {
				if child.ParentID.Valid && child.ParentID.Int64 == ch.ID {
					childIDStr := strconv.FormatInt(child.ID, 10)
					if visibleIDs[childIDStr] {
						hasVisibleChild = true
						break
					}
				}
			}
			if hasVisibleChild {
				result = append(result, dbChannelToChannel(ch))
			}
		} else {
			if visibleIDs[chIDStr] {
				result = append(result, dbChannelToChannel(ch))
			}
		}
	}

	if result == nil {
		result = []*Channel{}
	}
	return result, nil
}

// UpdateChannel updates a channel's name and topic. userID must be the server owner or have MANAGE_CHANNELS.
func (s *ChannelService) UpdateChannel(ctx context.Context, id, name, topic, userID string) (*Channel, error) {
	if name == "" {
		return nil, errors.New("channel name is required")
	}
	if len(name) > maxChannelNameLen {
		return nil, errors.New("channel name must be 100 characters or fewer")
	}

	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	channel, err := s.repo.GetChannelByID(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrChannelNotFound
		}
		return nil, err
	}

	srv, err := s.repo.GetServerByID(ctx, channel.ServerID)
	if err != nil {
		return nil, ErrServerNotFound
	}
	allowed, err := permissions.HasPermission(ctx, s.repo, channel.ServerID, userIDInt, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}

	channel.Name = name
	channel.Topic = topic

	err = s.repo.UpdateChannel(ctx, channel)
	if err != nil {
		return nil, err
	}

	ch := dbChannelToChannel(channel)
	if s.hub != nil {
		if payload, err := json.Marshal(ch); err == nil {
			s.hub.BroadcastToChannel("server:"+ch.ServerID, ws.EventChannelUpdate, payload)
		} else {
			log.Printf("Failed to marshal CHANNEL_UPDATE event: %v", err)
		}
	}
	return ch, nil
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
			return ErrChannelNotFound
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
	allowed, err := permissions.HasPermission(ctx, s.repo, srv.ID, userIDInt, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}

	serverID := strconv.FormatInt(ch.ServerID, 10)

	err = s.repo.DeleteChannel(ctx, idInt)
	if err != nil {
		if err == db.ErrNotFound {
			return ErrChannelNotFound
		}
		return err
	}

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"channel_id": id, "server_id": serverID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventChannelDelete, payload)
	}
	return nil
}

// dbChannelToChannel converts a db.Channel to a Channel for API responses
func dbChannelToChannel(dbCh *db.Channel) *Channel {
	ch := &Channel{
		ID:        strconv.FormatInt(dbCh.ID, 10),
		ServerID:  strconv.FormatInt(dbCh.ServerID, 10),
		Name:      dbCh.Name,
		Type:      ChannelType(dbCh.ChannelType),
		Position:  dbCh.Position,
		Topic:     dbCh.Topic,
		Synced:    dbCh.Synced,
		CreatedAt: dbCh.CreatedAt.Format(time.RFC3339),
		UpdatedAt: dbCh.UpdatedAt.Format(time.RFC3339),
	}

	if dbCh.ParentID.Valid {
		parentID := strconv.FormatInt(dbCh.ParentID.Int64, 10)
		ch.ParentID = &parentID
	}

	return ch
}

// ChannelOrder represents a single channel's position/parent update
type ChannelOrder struct {
	ID       string  `json:"id"`
	Position int     `json:"position"`
	ParentID *string `json:"parent_id"`
}

// ReorderChannels bulk-updates positions and parent_ids for channels in a server.
func (s *ChannelService) ReorderChannels(ctx context.Context, serverID string, orders []ChannelOrder, userID string) ([]*Channel, error) {
	serverIDInt, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	srv, err := s.repo.GetServerByID(ctx, serverIDInt)
	if err != nil {
		return nil, ErrServerNotFound
	}
	allowed, err := permissions.HasPermission(ctx, s.repo, serverIDInt, userIDInt, srv.OwnerID, permissions.PermManageChannels)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}

	for _, o := range orders {
		chIDInt, err := strconv.ParseInt(o.ID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid channel ID: " + o.ID)
		}
		var parentIDInt *int64
		if o.ParentID != nil {
			pid, err := strconv.ParseInt(*o.ParentID, 10, 64)
			if err != nil {
				return nil, errors.New("invalid parent ID")
			}
			parentIDInt = &pid
		}
		if err := s.repo.UpdateChannelOrder(ctx, chIDInt, o.Position, int64ToNullInt64(parentIDInt)); err != nil {
			return nil, err
		}
	}

	updated, err := s.repo.GetChannelsByServerID(ctx, serverIDInt)
	if err != nil {
		return nil, err
	}

	channels := make([]*Channel, len(updated))
	for i, c := range updated {
		channels[i] = dbChannelToChannel(c)
	}

	if s.hub != nil {
		for _, ch := range channels {
			if payload, err := json.Marshal(ch); err == nil {
				s.hub.BroadcastToChannel("server:"+serverID, ws.EventChannelUpdate, payload)
			}
		}
	}

	return channels, nil
}

// Repo returns the underlying repository. Used by handlers for audit logging.
func (s *ChannelService) Repo() *db.Repository { return s.repo }

// GetServerOwnerID returns the server owner's ID as a string, or "" on error.
func (s *ChannelService) GetServerOwnerID(ctx context.Context, serverID string) string {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return ""
	}
	srv, err := s.repo.GetServerByID(ctx, id)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(srv.OwnerID, 10)
}

// int64ToNullInt64 converts *int64 to sql.NullInt64
func int64ToNullInt64(val *int64) sql.NullInt64 {
	if val == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: *val, Valid: true}
}