package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"parley/internal/db"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

// Server represents a Discord server/guild in the API layer
type Server struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IconURL   string    `json:"icon_url,omitempty"`
	OwnerID   string    `json:"owner_id"`
	VanityURL string    `json:"vanity_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Role represents a server role in the API layer
type Role struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"server_id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Permissions int64     `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

// ServerMember represents a member of a server
type ServerMember struct {
	ID       string    `json:"id"`
	ServerID string    `json:"server_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Nickname string    `json:"nickname,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
	Roles    []Role    `json:"roles"`
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
	repo    *db.Repository
	hub     *ws.Hub
}

// NewServerService creates a new ServerService
func NewServerService(repo *db.Repository) *ServerService {
	return &ServerService{repo: repo}
}

// SetHub sets the WebSocket hub for broadcasting events
func (s *ServerService) SetHub(hub *ws.Hub) {
	s.hub = hub
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

	return dbServerToService(server), nil
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

	return dbServerToService(server), nil
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
		result[i] = dbServerToService(server)
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

	srv := dbServerToService(server)
	if s.hub != nil {
		if payload, err := json.Marshal(srv); err == nil {
			s.hub.BroadcastToChannel("server:"+id, ws.EventServerUpdate, payload)
		}
	}
	return srv, nil
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

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": id})
		s.hub.BroadcastToChannel("server:"+id, ws.EventServerDelete, payload)
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

	// Broadcast to all members of the server that a new member joined
	s.broadcastMemberJoin(serverID, userID)
	return nil
}

// broadcastMemberJoin sends a WebSocket event to all members of a server that a new member has joined.
func (s *ServerService) broadcastMemberJoin(serverID, userID string) {
	if s.hub == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
	if err != nil {
		log.Printf("Failed to marshal member join event: %v", err)
		return
	}
	s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberJoin, payload)
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

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberLeave, payload)
	}
	return nil
}

// KickMember removes a member from a server and notifies them via WebSocket.
func (s *ServerService) KickMember(ctx context.Context, serverID, userID string) error {
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

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberKick, payload)
		// Also send directly to the kicked user so they navigate away immediately
		s.hub.SendToUser(userID, ws.EventMemberKick, payload)
	}
	return nil
}

// BanMember bans a user from a server, removes them, and notifies them via WebSocket.
func (s *ServerService) BanMember(ctx context.Context, serverID, userID, bannedByID, reason string) error {
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
	bannedByIDInt, err := idToInt64(bannedByID)
	if err != nil {
		return errors.New("invalid banned_by ID format")
	}

	if err := s.repo.AddServerBan(ctx, serverIDInt, userIDInt, bannedByIDInt, reason); err != nil {
		return err
	}

	// Remove from server (may already not be a member, which is fine)
	_ = s.repo.RemoveMember(ctx, serverIDInt, userIDInt)

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberBan, payload)
		s.hub.SendToUser(userID, ws.EventMemberBan, payload)
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
			Roles:    []Role{},
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

	// Generate random 8-character code, retrying if it collides with invite codes or vanity URLs
	var code string
	for attempts := 0; attempts < 10; attempts++ {
		candidate, genErr := generateInviteCode()
		if genErr != nil {
			return nil, genErr
		}
		exists, checkErr := s.repo.InviteCodeExists(ctx, candidate)
		if checkErr != nil {
			return nil, checkErr
		}
		if !exists {
			code = candidate
			break
		}
	}
	if code == "" {
		return nil, errors.New("failed to generate unique invite code")
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

	return dbServerToService(server), nil
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
			return dbServerToService(server), nil
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

// JoinServerByVanityURL adds a user to a server via its vanity URL
func (s *ServerService) JoinServerByVanityURL(ctx context.Context, vanityURL, userID string) (*Server, error) {
	if vanityURL == "" {
		return nil, errors.New("vanity URL is required")
	}
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	server, err := s.repo.GetServerByVanityURL(ctx, vanityURL)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   userIDInt,
		Nickname: "",
	}
	if err = s.repo.AddMember(ctx, member); err != nil {
		_, getErr := s.repo.GetMember(ctx, server.ID, userIDInt)
		if getErr == nil {
			return dbServerToService(server), nil
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

// SetVanityURL sets or clears the vanity URL for a server
func (s *ServerService) SetVanityURL(ctx context.Context, serverID, userID, vanityURL string) (*Server, error) {
	if serverID == "" {
		return nil, errors.New("server ID is required")
	}
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	// Verify ownership
	srv, err := s.repo.GetServerByID(ctx, serverIDInt)
	if err != nil {
		return nil, errors.New("server not found")
	}
	userIDInt, _ := idToInt64(userID)
	if srv.OwnerID != userIDInt {
		return nil, errors.New("only the server owner can set the vanity URL")
	}

	slug := sql.NullString{}
	if vanityURL != "" {
		// Ensure the vanity URL doesn't collide with existing invite codes or another server's vanity URL
		exists, checkErr := s.repo.InviteCodeExists(ctx, vanityURL, serverIDInt)
		if checkErr != nil {
			return nil, checkErr
		}
		if exists {
			return nil, errors.New("that URL is already in use — choose a different one")
		}
		slug = sql.NullString{String: vanityURL, Valid: true}
	}

	if err := s.repo.SetVanityURL(ctx, serverIDInt, slug); err != nil {
		return nil, err
	}

	srv.VanityURL = slug
	return dbServerToService(srv), nil
}

// GetServerByVanityURL retrieves a server by vanity URL
func (s *ServerService) GetServerByVanityURL(ctx context.Context, vanityURL string) (*Server, error) {
	srv, err := s.repo.GetServerByVanityURL(ctx, vanityURL)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}
	return dbServerToService(srv), nil
}

// Helper functions

func dbServerToService(s *db.Server) *Server {
	return &Server{
		ID:        int64ToID(s.ID),
		Name:      s.Name,
		IconURL:   nullStringToString(s.IconURL),
		OwnerID:   int64ToID(s.OwnerID),
		VanityURL: nullStringToString(s.VanityURL),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

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

// dbRoleToRole converts a db.ServerRole to a service Role
func dbRoleToRole(r db.ServerRole) Role {
	return Role{
		ID:          strconv.FormatInt(r.ID, 10),
		ServerID:    strconv.FormatInt(r.ServerID, 10),
		Name:        r.Name,
		Color:       r.Color,
		Permissions: r.Permissions,
		CreatedAt:   r.CreatedAt,
	}
}

// GetServerRoles returns all roles for a server
func (s *ServerService) GetServerRoles(ctx context.Context, serverID string) ([]Role, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	dbRoles, err := s.repo.GetServerRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		roles[i] = dbRoleToRole(r)
	}
	return roles, nil
}

// CreateServerRole creates a new role in a server
func (s *ServerService) CreateServerRole(ctx context.Context, serverID, name, color string, permissions int64) (*Role, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	if name == "" {
		return nil, errors.New("role name is required")
	}
	if color == "" {
		color = "#99aab5"
	}
	dbRole, err := s.repo.CreateServerRole(ctx, id, name, color, permissions)
	if err != nil {
		return nil, err
	}
	r := dbRoleToRole(*dbRole)
	return &r, nil
}

// DeleteServerRole deletes a role from a server
func (s *ServerService) DeleteServerRole(ctx context.Context, serverID, roleID string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	return s.repo.DeleteServerRole(ctx, sID, rID)
}

// GetMemberRoles returns all roles assigned to a member
func (s *ServerService) GetMemberRoles(ctx context.Context, serverID, userID string) ([]Role, error) {
	sID, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}
	dbRoles, err := s.repo.GetMemberRoles(ctx, sID, uID)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		roles[i] = dbRoleToRole(r)
	}
	return roles, nil
}

// AssignRoleToMember assigns a role to a member
func (s *ServerService) AssignRoleToMember(ctx context.Context, serverID, userID, roleID string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	if err := s.repo.AssignRoleToMember(ctx, sID, uID, rID); err != nil {
		return err
	}
	s.broadcastRoleUpdate(ctx, serverID, userID, sID, uID)
	return nil
}

// RemoveRoleFromMember removes a role from a member
func (s *ServerService) RemoveRoleFromMember(ctx context.Context, serverID, userID, roleID string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	if err := s.repo.RemoveRoleFromMember(ctx, sID, uID, rID); err != nil {
		return err
	}
	s.broadcastRoleUpdate(ctx, serverID, userID, sID, uID)
	return nil
}

// broadcastRoleUpdate sends a MEMBER_ROLE_UPDATE event with the user's current roles.
func (s *ServerService) broadcastRoleUpdate(ctx context.Context, serverID, userID string, sID, uID int64) {
	if s.hub == nil {
		return
	}
	dbRoles, err := s.repo.GetMemberRoles(ctx, sID, uID)
	if err != nil {
		return
	}
	roles := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		roles[i] = dbRoleToRole(r)
	}
	payload, err := json.Marshal(map[string]interface{}{
		"server_id": serverID,
		"user_id":   userID,
		"roles":     roles,
	})
	if err != nil {
		return
	}
	s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberRoleUpdate, payload)
}

// GetMembersWithRoles returns all members of a server with their roles
func (s *ServerService) GetMembersWithRoles(ctx context.Context, serverID string) ([]*ServerMember, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	dbMembers, err := s.repo.GetServerMembersWithRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	members := make([]*ServerMember, len(dbMembers))
	for i, m := range dbMembers {
		roles := make([]Role, len(m.Roles))
		for j, r := range m.Roles {
			roles[j] = dbRoleToRole(r)
		}
		members[i] = &ServerMember{
			ID:       strconv.FormatInt(m.ID, 10),
			ServerID: strconv.FormatInt(m.ServerID, 10),
			UserID:   strconv.FormatInt(m.UserID, 10),
			Username: m.Username,
			Nickname: m.Nickname,
			JoinedAt: m.JoinedAt,
			Roles:    roles,
		}
	}
	return members, nil
}

// CanKickBan returns true if actorID has permission to kick/ban members in the server.
// Returns (isOwner, hasKickPerm, error).
func (s *ServerService) CanKickBan(ctx context.Context, serverID, actorID string) (isOwner bool, allowed bool, err error) {
	srv, err := s.GetServer(ctx, serverID)
	if err != nil {
		return false, false, err
	}
	if srv.OwnerID == actorID {
		return true, true, nil
	}
	sID, _ := idToInt64(serverID)
	aID, err := idToInt64(actorID)
	if err != nil {
		return false, false, errors.New("invalid actor ID")
	}
	ownerID, _ := idToInt64(srv.OwnerID)
	hasPerm, err := permissions.HasPermission(ctx, s.repo, sID, aID, ownerID, permissions.PermKickMembers)
	if err != nil {
		return false, false, err
	}
	return false, hasPerm, nil
}

// GetMyPermissions returns the effective permission bitfield and owner status for a user in a server.
func (s *ServerService) GetMyPermissions(ctx context.Context, serverID, userID string) (int64, bool, error) {
	srv, err := s.GetServer(ctx, serverID)
	if err != nil {
		return 0, false, err
	}
	isOwner := srv.OwnerID == userID
	if isOwner {
		return ^int64(0), true, nil
	}
	sID, _ := idToInt64(serverID)
	uID, err := idToInt64(userID)
	if err != nil {
		return 0, false, errors.New("invalid user ID")
	}
	ownerID, _ := idToInt64(srv.OwnerID)
	perms, err := permissions.GetEffectivePermissions(ctx, s.repo, sID, uID, ownerID)
	if err != nil {
		return 0, false, err
	}
	return perms, false, nil
}
