package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"parley/internal/audit"
	"parley/internal/cache"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

// Sentinel errors returned by ServerService methods.
var (
	ErrServerNotFound  = errors.New("server not found")
	ErrMemberNotFound  = errors.New("member not found")
	ErrInviteNotFound  = errors.New("invite not found")
	ErrBanned          = errors.New("you are banned from this server")
	ErrNotMember       = errors.New("not a member of this server")
	ErrOwnerOnly       = errors.New("only the server owner can set the vanity URL")
	ErrForbidden       = errors.New("forbidden")
)

// ============ API layer types ============

type Server struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	IconURL     string    `json:"icon_url,omitempty"`
	OwnerID     string    `json:"owner_id"`
	VanityURL   string    `json:"vanity_url,omitempty"`
	Description string    `json:"description,omitempty"`
	IsPublic    bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PublicServer is the API representation of a server in the public discovery directory.
type PublicServer struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	IconURL     string              `json:"icon_url,omitempty"`
	VanityURL   string              `json:"vanity_url"`
	Description string              `json:"description,omitempty"`
	MemberCount int                 `json:"member_count"`
	Categories  []db.ServerCategory `json:"categories"`
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
	InviteCode  string    `json:"invite_code,omitempty"`
	Roles       []Role    `json:"roles"`
	IsBot       bool      `json:"is_bot,omitempty"`
	BotDegraded bool      `json:"bot_degraded,omitempty"`
}

type Invite struct {
	ID              string     `json:"id"`
	ServerID        string     `json:"server_id"`
	Code            string     `json:"code"`
	CreatedBy       string     `json:"created_by"`
	CreatorUsername string     `json:"creator_username,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	MaxUses         *int       `json:"max_uses,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	UseCount        int        `json:"use_count"`
	IsActive        bool       `json:"is_active"`
}

// ============ Service ============

type ServerService struct {
	repo        *db.Repository
	hub         *ws.Hub
	auditSvc    *audit.AuditService
	memberCache *cache.MembershipCache
}

func NewServerService(repo *db.Repository, auditSvc *audit.AuditService) *ServerService {
	return &ServerService{repo: repo, auditSvc: auditSvc}
}

func (s *ServerService) SetHub(hub *ws.Hub) {
	s.hub = hub
}

// SetMemberCache sets the membership cache so that permission entries can be
// invalidated when roles are updated or deleted.
func (s *ServerService) SetMemberCache(mc *cache.MembershipCache) {
	s.memberCache = mc
}

// Repo exposes the repository for permission checks in handlers.
func (s *ServerService) Repo() *db.Repository { return s.repo }

// ============ Helper functions ============

func dbServerToService(s *db.Server) *Server {
	return &Server{
		ID:          int64ToID(s.ID),
		Name:        s.Name,
		IconURL:     nullStringToString(s.IconURL),
		OwnerID:     int64ToID(s.OwnerID),
		VanityURL:   nullStringToString(s.VanityURL),
		Description: s.Description.String,
		IsPublic:    s.IsPublic,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
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
