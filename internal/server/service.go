package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strconv"
	"time"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

// ============ API layer types ============

type Server struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IconURL   string    `json:"icon_url,omitempty"`
	OwnerID   string    `json:"owner_id"`
	VanityURL string    `json:"vanity_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Role struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"server_id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Permissions int64     `json:"permissions"`
	Hoist       bool      `json:"hoist"`
	Position    int       `json:"position"`
	IsEveryone  bool      `json:"is_everyone"`
	CreatedAt   time.Time `json:"created_at"`
}

type ServerMember struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"server_id"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	Nickname    string    `json:"nickname,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	BannerURL   string    `json:"banner_url,omitempty"`
	Bio         string    `json:"bio,omitempty"`
	Badges      int       `json:"badges"`
	JoinedAt    time.Time `json:"joined_at"`
	Roles       []Role    `json:"roles"`
	IsBot       bool      `json:"is_bot,omitempty"`
	BotDegraded bool      `json:"bot_degraded,omitempty"`
}

type Invite struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	Code      string    `json:"code"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// ============ Service ============

type ServerService struct {
	repo *db.Repository
	hub  *ws.Hub
}

func NewServerService(repo *db.Repository) *ServerService {
	return &ServerService{repo: repo}
}

func (s *ServerService) SetHub(hub *ws.Hub) {
	s.hub = hub
}

// Repo exposes the repository for permission checks in handlers.
func (s *ServerService) Repo() *db.Repository { return s.repo }

// ============ Helper functions ============

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

func dbRoleToRole(r db.ServerRole) Role {
	name := r.Name
	if r.IsEveryone {
		name = "@everyone"
	}
	return Role{
		ID:          strconv.FormatInt(r.ID, 10),
		ServerID:    strconv.FormatInt(r.ServerID, 10),
		Name:        name,
		Color:       r.Color,
		Permissions: r.Permissions,
		Hoist:       r.Hoist,
		Position:    r.Position,
		IsEveryone:  r.IsEveryone,
		CreatedAt:   r.CreatedAt,
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

func generateInviteCode() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
