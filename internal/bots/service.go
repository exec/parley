// internal/bots/service.go
package bots

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"parley/internal/permissions"
)

var ErrForbidden = errors.New("forbidden")
var ErrBadRequest = errors.New("bad request")

type Service struct {
	repo      *Repository
	keySecret []byte // 32 bytes for AES-256
}

func NewService(repo *Repository, keySecret []byte) *Service {
	if len(keySecret) < 32 {
		panic("bots.NewService: keySecret must be at least 32 bytes")
	}
	return &Service{repo: repo, keySecret: keySecret[:32]}
}

// ListBots returns bots in a server. Any member may call this.
func (s *Service) ListBots(ctx context.Context, serverID int64) ([]Bot, error) {
	return s.repo.ListServerBots(ctx, serverID)
}

// AddBot adds a bot to a server by invite token. Caller must be server admin or owner.
// Returns the bot's user ID so the caller can broadcast a member join event.
func (s *Service) AddBot(ctx context.Context, serverID, callerID int64, inviteToken string) (int64, error) {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return 0, err
	}
	botUserID, _, err := s.repo.ResolveInviteToken(ctx, inviteToken)
	if errors.Is(err, ErrNotFound) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return botUserID, s.repo.AddBotToServer(ctx, serverID, botUserID)
}

// RemoveBot removes a bot from a server. Caller must be server admin or owner.
func (s *Service) RemoveBot(ctx context.Context, serverID, botUserID, callerID int64) error {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return err
	}
	return s.repo.RemoveBotFromServer(ctx, serverID, botUserID)
}

// GetAIConfig returns the AI config for a server. Caller must be server admin or owner.
func (s *Service) GetAIConfig(ctx context.Context, serverID, callerID int64) (*AIConfig, error) {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return nil, err
	}
	cfg, _, err := s.repo.GetAIConfig(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &AIConfig{
			Provider:          "parley",
			Model:             "ministral-3:14b",
			PresetVerbosity:   "concise",
			PresetPersonality: "friendly",
			PresetRole:        "assistant",
		}, nil
	}
	return cfg, nil
}

// SetAIConfig saves AI config for a server. Caller must be server admin or owner.
// If apiKey is non-empty, it is AES-256-GCM encrypted before storage.
func (s *Service) SetAIConfig(ctx context.Context, serverID, callerID int64, provider, model, verbosity, personality, role, apiKey string) error {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return err
	}
	var apiKeyEncPtr *string
	if apiKey != "" {
		enc, err := s.encrypt(apiKey)
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
		apiKeyEncPtr = &enc
	}
	return s.repo.UpsertAIConfig(ctx, serverID, provider, model, verbosity, personality, role, apiKeyEncPtr)
}

// GetAIUsage returns the monthly token usage for a server. Caller must be server admin or owner.
func (s *Service) GetAIUsage(ctx context.Context, serverID, callerID int64) (*AIUsage, error) {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return nil, err
	}
	cfg, _, err := s.repo.GetAIConfig(ctx, serverID)
	if err != nil {
		return nil, err
	}
	model := "ministral-3:14b"
	if cfg != nil {
		model = cfg.Model
	}
	used, err := s.repo.GetMonthlyUsage(ctx, serverID)
	if err != nil {
		return nil, err
	}
	// Resets 1st of next month
	now := time.Now().UTC()
	resetsAt := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return &AIUsage{
		TokensUsed:  used,
		TokensLimit: ParleyMonthlyBudget,
		Model:       model,
		ResetsAt:    resetsAt.Format(time.RFC3339),
	}, nil
}

// GetMyBots returns bots owned by the caller (invite tokens they created, excluding selfbots).
func (s *Service) GetMyBots(ctx context.Context, callerID int64) ([]UserBot, error) {
	return s.repo.GetUserBots(ctx, callerID)
}

// ResolveInvite resolves a bot invite token to bot info. Public (no auth required).
func (s *Service) ResolveInvite(ctx context.Context, token string) (*BotInviteInfo, error) {
	botUserID, permissions, err := s.repo.ResolveInviteToken(ctx, token)
	if err != nil {
		return nil, ErrNotFound
	}
	info, err := s.repo.GetBotInfo(ctx, botUserID)
	if err != nil {
		return nil, ErrNotFound
	}
	info.Permissions = permissions
	return info, nil
}

// AcceptInvite adds a bot to a server via invite token. Caller must be server admin or owner,
// and must be a member of the target server.
// Returns the bot's user ID so the caller can broadcast a member_join event.
func (s *Service) AcceptInvite(ctx context.Context, token string, serverID, callerID int64, grantedPermissions int64) (int64, error) {
	const maxPerms = (int64(1) << 42) - 1
	if grantedPermissions < 0 || grantedPermissions > maxPerms {
		return 0, ErrBadRequest
	}
	isMember, err := s.repo.IsServerMember(ctx, serverID, callerID)
	if err != nil {
		return 0, err
	}
	if !isMember {
		return 0, ErrNotFound
	}
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return 0, err
	}
	botUserID, _, err := s.repo.ResolveInviteToken(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	info, err := s.repo.GetBotInfo(ctx, botUserID)
	if err != nil {
		return 0, err
	}
	if err := s.repo.AddBotToServerWithRole(ctx, serverID, botUserID, info.Username, grantedPermissions); err != nil {
		return 0, err
	}
	return botUserID, nil
}

// UpdateInvitePermissions sets the requested permissions on the bot's invite token.
// callerID must own the bot. Returns ErrForbidden if they don't.
func (s *Service) UpdateInvitePermissions(ctx context.Context, botUserID, callerID, permissions int64) error {
	const maxPerms = (int64(1) << 42) - 1
	if permissions < 0 || permissions > maxPerms {
		return ErrBadRequest
	}
	err := s.repo.UpdateBotInvitePermissions(ctx, botUserID, callerID, permissions)
	if errors.Is(err, ErrNotFound) {
		return ErrForbidden
	}
	return err
}

// DecryptAPIKey decrypts an AES-256-GCM encrypted API key for use in LLM dispatch.
func (s *Service) DecryptAPIKey(enc string) (string, error) {
	return s.decrypt(enc)
}

// requireAdmin checks that callerID is a server administrator or owner.
func (s *Service) requireAdmin(ctx context.Context, serverID, callerID int64) error {
	ownerID, err := s.repo.GetServerOwnerID(ctx, serverID)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	ok, err := permissions.HasPermission(ctx, s.repo.DBRepo(), serverID, callerID, ownerID, permissions.PermAdministrator)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

// encrypt performs AES-256-GCM encryption, returning base64url-encoded ciphertext.
func (s *Service) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.keySecret)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(sealed), nil
}

// decrypt reverses encrypt.
func (s *Service) decrypt(encoded string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.keySecret)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
