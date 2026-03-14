package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"parley/internal/db"
)

// Server represents a Discord server/guild in the API layer
type Server struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IconURL   string    `json:"icon_url,omitempty"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServerMember represents a member of a server
type ServerMember struct {
	ID       string    `json:"id"`
	ServerID string    `json:"server_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Nickname string    `json:"nickname,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
}

// Invite represents an invite code
type Invite struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	Code      string    `json:"code"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// ServerService handles server and member operations
type ServerService struct {
	repo *db.Repository
}

// NewServerService creates a new ServerService
func NewServerService(repo *db.Repository) *ServerService {
	return &ServerService{repo: repo}
}

// CreateServer creates a new server and adds the owner as the first member
func (s *ServerService) CreateServer(ctx context.Context, name, iconURL string, ownerID string) (*Server, error) {
	if name == "" {
		return nil, errors.New("server name is required")
	}
	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	ownerIDInt, err := idToInt64(ownerID)
	if err != nil {
		return nil, errors.New("invalid owner ID format")
	}

	// Create server
	server := &db.Server{
		Name:    name,
		IconURL: nullString(iconURL),
		OwnerID: ownerIDInt,
	}

	err = s.repo.CreateServer(ctx, server)
	if err != nil {
		return nil, err
	}

	// Add owner as first member
	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   ownerIDInt,
		Nickname: "",
	}

	err = s.repo.AddMember(ctx, member)
	if err != nil {
		return nil, err
	}

	return &Server{
		ID:        int64ToID(server.ID),
		Name:      server.Name,
		IconURL:   nullStringToString(server.IconURL),
		OwnerID:   int64ToID(server.OwnerID),
		CreatedAt: server.CreatedAt,
		UpdatedAt: server.UpdatedAt,
	}, nil
}

// GetServer retrieves a server by ID
func (s *ServerService) GetServer(ctx context.Context, id string) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	server, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	return &Server{
		ID:        int64ToID(server.ID),
		Name:      server.Name,
		IconURL:   nullStringToString(server.IconURL),
		OwnerID:   int64ToID(server.OwnerID),
		CreatedAt: server.CreatedAt,
		UpdatedAt: server.UpdatedAt,
	}, nil
}

// GetUserServers retrieves all servers a user is a member of
func (s *ServerService) GetUserServers(ctx context.Context, userID string) ([]*Server, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	servers, err := s.repo.GetServersByUserID(ctx, userIDInt)
	if err != nil {
		return nil, err
	}

	result := make([]*Server, len(servers))
	for i, server := range servers {
		result[i] = &Server{
			ID:        int64ToID(server.ID),
			Name:      server.Name,
			IconURL:   nullStringToString(server.IconURL),
			OwnerID:   int64ToID(server.OwnerID),
			CreatedAt: server.CreatedAt,
			UpdatedAt: server.UpdatedAt,
		}
	}

	return result, nil
}

// UpdateServer updates a server's name and icon
func (s *ServerService) UpdateServer(ctx context.Context, id, name, iconURL string) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}
	if name == "" {
		return nil, errors.New("server name is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	// Get existing server to preserve OwnerID
	server, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	server.Name = name
	server.IconURL = nullString(iconURL)

	err = s.repo.UpdateServer(ctx, server)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	return &Server{
		ID:        int64ToID(server.ID),
		Name:      server.Name,
		IconURL:   nullStringToString(server.IconURL),
		OwnerID:   int64ToID(server.OwnerID),
		CreatedAt: server.CreatedAt,
		UpdatedAt: server.UpdatedAt,
	}, nil
}

// DeleteServer deletes a server by ID
func (s *ServerService) DeleteServer(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("server ID is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return errors.New("invalid server ID format")
	}

	err = s.repo.DeleteServer(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errors.New("server not found")
		}
		return err
	}

	return nil
}

// AddMember adds a user to a server
func (s *ServerService) AddMember(ctx context.Context, serverID, userID, nickname string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	if userID == "" {
		return errors.New("user ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID format")
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID format")
	}

	member := &db.ServerMember{
		ServerID: serverIDInt,
		UserID:   userIDInt,
		Nickname: nickname,
	}

	err = s.repo.AddMember(ctx, member)
	if err != nil {
		return err
	}

	return nil
}

// RemoveMember removes a user from a server
func (s *ServerService) RemoveMember(ctx context.Context, serverID, userID string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	if userID == "" {
		return errors.New("user ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID format")
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID format")
	}

	err = s.repo.RemoveMember(ctx, serverIDInt, userIDInt)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errors.New("member not found")
		}
		return err
	}

	return nil
}

// GetMembers retrieves all members of a server
func (s *ServerService) GetMembers(ctx context.Context, serverID string) ([]*ServerMember, error) {
	if serverID == "" {
		return nil, errors.New("server ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	members, err := s.repo.GetServerMembers(ctx, serverIDInt)
	if err != nil {
		return nil, err
	}

	result := make([]*ServerMember, len(members))
	for i, member := range members {
		result[i] = &ServerMember{
			ID:       int64ToID(member.ID),
			ServerID: int64ToID(member.ServerID),
			UserID:   int64ToID(member.UserID),
			Username: member.Username,
			Nickname: member.Nickname,
			JoinedAt: member.JoinedAt,
		}
	}

	return result, nil
}

// CreateInvite creates a new invite for a server
func (s *ServerService) CreateInvite(ctx context.Context, serverID, createdBy string) (*Invite, error) {
	if serverID == "" {
		return nil, errors.New("server ID is required")
	}
	if createdBy == "" {
		return nil, errors.New("created by is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	createdByInt, err := idToInt64(createdBy)
	if err != nil {
		return nil, errors.New("invalid created by format")
	}

	// Generate random 8-character code
	code, err := generateInviteCode()
	if err != nil {
		return nil, err
	}

	invite := &db.Invite{
		ServerID:  serverIDInt,
		Code:      code,
		CreatedBy: createdByInt,
	}

	err = s.repo.CreateInvite(ctx, invite)
	if err != nil {
		return nil, err
	}

	return &Invite{
		ID:        int64ToID(invite.ID),
		ServerID:  int64ToID(invite.ServerID),
		Code:      invite.Code,
		CreatedBy: int64ToID(invite.CreatedBy),
		CreatedAt: invite.CreatedAt,
	}, nil
}

// GetInviteByCode retrieves an invite by its code
func (s *ServerService) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	if code == "" {
		return nil, errors.New("invite code is required")
	}

	invite, err := s.repo.GetInviteByCode(ctx, code)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	return &Invite{
		ID:        int64ToID(invite.ID),
		ServerID:  int64ToID(invite.ServerID),
		Code:      invite.Code,
		CreatedBy: int64ToID(invite.CreatedBy),
		CreatedAt: invite.CreatedAt,
	}, nil
}

// GetServerByInviteCode retrieves a server by invite code
func (s *ServerService) GetServerByInviteCode(ctx context.Context, code string) (*Server, error) {
	if code == "" {
		return nil, errors.New("invite code is required")
	}

	server, err := s.repo.GetServerByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	return &Server{
		ID:        int64ToID(server.ID),
		Name:      server.Name,
		IconURL:   nullStringToString(server.IconURL),
		OwnerID:   int64ToID(server.OwnerID),
		CreatedAt: server.CreatedAt,
		UpdatedAt: server.UpdatedAt,
	}, nil
}

// JoinServerByInvite adds the current user to a server via invite code
func (s *ServerService) JoinServerByInvite(ctx context.Context, code, userID string) (*Server, error) {
	if code == "" {
		return nil, errors.New("invite code is required")
	}
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	// Get the server by invite code
	server, err := s.repo.GetServerByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	// Add user as member
	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   userIDInt,
		Nickname: "",
	}

	err = s.repo.AddMember(ctx, member)
	if err != nil {
		// Check if already a member
		_, getErr := s.repo.GetMember(ctx, server.ID, userIDInt)
		if getErr == nil {
			// Already a member, just return the server
			return &Server{
				ID:        int64ToID(server.ID),
				Name:      server.Name,
				IconURL:   nullStringToString(server.IconURL),
				OwnerID:   int64ToID(server.OwnerID),
				CreatedAt: server.CreatedAt,
				UpdatedAt: server.UpdatedAt,
			}, nil
		}
		return nil, err
	}

	return &Server{
		ID:        int64ToID(server.ID),
		Name:      server.Name,
		IconURL:   nullStringToString(server.IconURL),
		OwnerID:   int64ToID(server.OwnerID),
		CreatedAt: server.CreatedAt,
		UpdatedAt: server.UpdatedAt,
	}, nil
}

// Helper functions

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringToString(ns sql.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}

func int64ToID(n int64) string {
	return strconv.FormatInt(n, 10)
}

func idToInt64(id string) (int64, error) {
	return strconv.ParseInt(id, 10, 64)
}

// generateInviteCode generates a random 8-character invite code
func generateInviteCode() (string, error) {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}