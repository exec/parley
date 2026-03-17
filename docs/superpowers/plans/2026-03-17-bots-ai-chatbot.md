# Bots & AI Chatbot Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Bots tab to Server Settings with an AI Chatbot bot that responds to @mentions and replies using a configurable LLM provider (Parley/Ollama free, or user-supplied Anthropic/OpenAI/xAI/Mistral/Google key), with monthly token allowances for the Parley provider.

**Architecture:** A new `internal/bots` Go package handles all bot DB operations, business logic, provider dispatch, and HTTP handlers. The bot trigger is injected into `MessageService` via a `SetBotTrigger` hook (same pattern as `SetBroadcaster`). The frontend adds a Bots tab to `ServerSettings.tsx`, a shared `EmbedCard` component for both theme and bot invite embeds, and a `/bots/invite/:token` route.

**Tech Stack:** Go 1.21, PostgreSQL, chi router, React 18, TypeScript, existing `apiClient` wrapper, AES-256-GCM for API key encryption.

---

## File Structure

**New backend files:**
- `internal/bots/models.go` — domain types
- `internal/bots/repository.go` — all DB queries
- `internal/bots/service.go` — business logic + encryption
- `internal/bots/ai_dispatch.go` — LLM provider dispatch + bot trigger function
- `internal/bots/handler.go` — HTTP handlers

**Modified backend files:**
- `internal/db/migrations.go` — new migration entry (migration 31)
- `internal/message/service.go` — add `BotTriggerFunc` hook
- `cmd/api/main.go` — load `BOT_KEY_SECRET`, cache bot user ID, wire trigger
- `cmd/api/routes.go` — register bot routes

**New frontend files:**
- `frontend/src/components/EmbedCard.tsx`
- `frontend/src/components/EmbedCard.css`
- `frontend/src/api/bots.ts`
- `frontend/src/components/settings/BotsTab.tsx`
- `frontend/src/components/settings/BotConfigPanel.tsx`
- `frontend/src/components/settings/BotsTab.css`
- `frontend/src/components/BotInviteEmbed.tsx`
- `frontend/src/pages/BotInvitePage.tsx`

**Modified frontend files:**
- `frontend/src/components/theme/ThemeLinkEmbed.tsx` — use EmbedCard shell
- `frontend/src/components/settings/ServerSettings.tsx` — add Bots tab
- `frontend/src/App.tsx` — add `/bots/invite/:token` route

---

## Chunk 1: Backend Foundation

### Task 1: Database Migration

**Files:**
- Modify: `internal/db/migrations.go`

- [ ] **Step 1: Append new migration entry**

Open `internal/db/migrations.go`. At the end of the `Migrations` slice (after the last entry ending with `` `}` ``), append:

```go
`-- Migration 31: Bots & AI Chatbot
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_verified BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS server_bots (
  server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  added_at    TIMESTAMP NOT NULL DEFAULT NOW(),
  PRIMARY KEY (server_id, bot_user_id)
);
CREATE INDEX IF NOT EXISTS idx_server_bots_bot ON server_bots(bot_user_id);

CREATE TABLE IF NOT EXISTS server_ai_config (
  server_id     BIGINT PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  provider      VARCHAR(32)  NOT NULL DEFAULT 'parley',
  model         VARCHAR(128) NOT NULL DEFAULT 'ministral-3:14b',
  api_key_enc   TEXT,
  system_prompt TEXT NOT NULL DEFAULT '',
  updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS server_bot_usage (
  server_id   BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  month       DATE   NOT NULL,
  tokens_used BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (server_id, month)
);

CREATE TABLE IF NOT EXISTS bot_invite_tokens (
  id          BIGSERIAL PRIMARY KEY,
  bot_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token       UUID   NOT NULL UNIQUE DEFAULT gen_random_uuid(),
  created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_invite_tokens_bot ON bot_invite_tokens(bot_user_id);

-- Seed AI Chatbot system user (email nullable since migration 11)
INSERT INTO users (username, display_name, password_hash, is_bot, is_verified)
VALUES ('ai-chatbot', 'AI Chatbot', '', TRUE, TRUE)
ON CONFLICT (username) DO NOTHING;

-- Seed permanent invite token (fixed UUID so it can be referenced in config/docs)
INSERT INTO bot_invite_tokens (bot_user_id, token)
SELECT id, 'aaaaaaaa-0000-0000-0000-000000000001'::uuid
FROM users WHERE username = 'ai-chatbot'
ON CONFLICT DO NOTHING;
`,
```

- [ ] **Step 2: Build to verify migration compiles**

```bash
cd /home/dylan/Developer/parley
go build ./internal/db/...
```
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations.go
git commit -m "feat(bots): add migration for bots, ai_config, and invite tokens"
```

---

### Task 2: Bot Domain Types and Repository

**Files:**
- Create: `internal/bots/models.go`
- Create: `internal/bots/repository.go`

- [ ] **Step 1: Create models.go**

```go
// internal/bots/models.go
package bots

import "time"

// Bot represents a bot user summary.
type Bot struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsVerified  bool      `json:"is_verified"`
	AddedAt     time.Time `json:"added_at"`
}

// AIConfig is the per-server AI chatbot configuration.
type AIConfig struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	APIKeySet    bool   `json:"api_key_set"`
	SystemPrompt string `json:"system_prompt"`
	UpdatedAt    string `json:"updated_at"`
}

// AIUsage is the current-month token usage for a server.
type AIUsage struct {
	TokensUsed  int64  `json:"tokens_used"`
	TokensLimit int64  `json:"tokens_limit"`
	Model       string `json:"model"`
	ResetsAt    string `json:"resets_at"`
}

// BotInviteInfo is returned by GET /api/bots/invite/{token}.
type BotInviteInfo struct {
	BotID       int64  `json:"bot_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	IsVerified  bool   `json:"is_verified"`
}

// ParleyModelAllowances maps stored model value → monthly token allowance.
var ParleyModelAllowances = map[string]int64{
	"ministral-3:14b": 2_000_000,
	"gpt-oss:20b":     1_500_000,
	"gemma3:27b":      1_000_000,
	"gpt-oss:120b":    300_000,
	"qwen3:latest":    100_000,
}
```

- [ ] **Step 2: Create repository.go**

```go
// internal/bots/repository.go
package bots

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	dbpkg "parley/internal/db"
)

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

type Repository struct {
	db     *sql.DB
	dbRepo *dbpkg.Repository
}

// NewRepository accepts the shared *db.Repository so permission checks can use it.
func NewRepository(repo *dbpkg.Repository) *Repository {
	return &Repository{db: repo.DB(), dbRepo: repo}
}

// DBRepo exposes the underlying db.Repository for use by the permissions package.
func (r *Repository) DBRepo() *dbpkg.Repository { return r.dbRepo }

// GetBotUserID returns the user ID of the named bot (cached by caller).
func (r *Repository) GetBotUserID(ctx context.Context, username string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username = $1 AND is_bot = TRUE`, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// ListServerBots returns all bots in a server.
func (r *Repository) ListServerBots(ctx context.Context, serverID int64) ([]Bot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), u.is_verified, sb.added_at
		FROM server_bots sb
		JOIN users u ON u.id = sb.bot_user_id
		WHERE sb.server_id = $1
		ORDER BY sb.added_at`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []Bot
	for rows.Next() {
		var b Bot
		if err := rows.Scan(&b.ID, &b.Username, &b.DisplayName, &b.AvatarURL, &b.IsVerified, &b.AddedAt); err != nil {
			return nil, err
		}
		bots = append(bots, b)
	}
	if bots == nil {
		bots = []Bot{}
	}
	return bots, rows.Err()
}

// IsBotInServer returns true if the given bot is in the server.
func (r *Repository) IsBotInServer(ctx context.Context, serverID, botUserID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM server_bots WHERE server_id=$1 AND bot_user_id=$2`, serverID, botUserID).Scan(&n)
	return n > 0, err
}

// AddBotToServer inserts a server_bots row.
func (r *Repository) AddBotToServer(ctx context.Context, serverID, botUserID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_bots (server_id, bot_user_id) VALUES ($1, $2)`,
		serverID, botUserID)
	if err != nil && isPgUniqueViolation(err) {
		return ErrAlreadyExists
	}
	return err
}

// RemoveBotFromServer deletes a server_bots row.
func (r *Repository) RemoveBotFromServer(ctx context.Context, serverID, botUserID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM server_bots WHERE server_id=$1 AND bot_user_id=$2`, serverID, botUserID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAIConfig returns the AI config for a server, or nil if not set.
func (r *Repository) GetAIConfig(ctx context.Context, serverID int64) (*AIConfig, string, error) {
	var cfg AIConfig
	var apiKeyEnc sql.NullString
	var updatedAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT provider, model, api_key_enc, system_prompt, updated_at
		 FROM server_ai_config WHERE server_id = $1`, serverID).
		Scan(&cfg.Provider, &cfg.Model, &apiKeyEnc, &cfg.SystemPrompt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	cfg.APIKeySet = apiKeyEnc.Valid && apiKeyEnc.String != ""
	cfg.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	rawEnc := ""
	if apiKeyEnc.Valid {
		rawEnc = apiKeyEnc.String
	}
	return &cfg, rawEnc, nil
}

// UpsertAIConfig saves AI config. Pass empty apiKeyEnc to leave existing key unchanged.
func (r *Repository) UpsertAIConfig(ctx context.Context, serverID int64, provider, model, systemPrompt string, apiKeyEnc *string) error {
	if apiKeyEnc != nil {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO server_ai_config (server_id, provider, model, api_key_enc, system_prompt, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT (server_id) DO UPDATE SET
				provider=EXCLUDED.provider, model=EXCLUDED.model,
				api_key_enc=EXCLUDED.api_key_enc, system_prompt=EXCLUDED.system_prompt,
				updated_at=NOW()`,
			serverID, provider, model, *apiKeyEnc, systemPrompt)
		return err
	}
	// Don't overwrite existing api_key_enc
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO server_ai_config (server_id, provider, model, system_prompt, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (server_id) DO UPDATE SET
			provider=EXCLUDED.provider, model=EXCLUDED.model,
			system_prompt=EXCLUDED.system_prompt, updated_at=NOW()`,
		serverID, provider, model, systemPrompt)
	return err
}

// GetMonthlyUsage returns tokens_used for the current month.
func (r *Repository) GetMonthlyUsage(ctx context.Context, serverID int64) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(tokens_used,0) FROM server_bot_usage
		 WHERE server_id=$1 AND month=DATE_TRUNC('month',NOW())::date`, serverID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// AddTokenUsage atomically increments tokens_used for the current month.
func (r *Repository) AddTokenUsage(ctx context.Context, serverID int64, delta int64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO server_bot_usage (server_id, month, tokens_used)
		VALUES ($1, DATE_TRUNC('month',NOW())::date, $2)
		ON CONFLICT (server_id, month) DO UPDATE
		SET tokens_used = server_bot_usage.tokens_used + EXCLUDED.tokens_used`,
		serverID, delta)
	return err
}

// ResolveInviteToken returns bot user ID for a given invite token UUID.
func (r *Repository) ResolveInviteToken(ctx context.Context, token string) (int64, error) {
	var botUserID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT bot_user_id FROM bot_invite_tokens WHERE token=$1::uuid`, token).Scan(&botUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return botUserID, err
}

// GetBotInfo returns BotInviteInfo for a bot user ID.
func (r *Repository) GetBotInfo(ctx context.Context, botUserID int64) (*BotInviteInfo, error) {
	var b BotInviteInfo
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, COALESCE(display_name,''), COALESCE(avatar_url,''), is_verified
		 FROM users WHERE id=$1 AND is_bot=TRUE`, botUserID).
		Scan(&b.BotID, &b.Username, &b.DisplayName, &b.AvatarURL, &b.IsVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

// IsServerMember returns true if userID is a member of the server.
func (r *Repository) IsServerMember(ctx context.Context, serverID, userID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&n)
	return n > 0, err
}

// GetChannelServerID returns the server_id for a channel.
// Note: DM messages use the dm_messages table and go through a completely separate code
// path — they never reach SendMessage and therefore never trigger this. All rows in the
// channels table have a non-null server_id (NOT NULL constraint in migration 0).
func (r *Repository) GetChannelServerID(ctx context.Context, channelID int64) (int64, bool, error) {
	var serverID int64
	err := r.db.QueryRowContext(ctx, `SELECT server_id FROM channels WHERE id=$1`, channelID).Scan(&serverID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return serverID, true, nil
}

// GetServerOwnerID returns the owner_id for a server. Used for permission checks.
func (r *Repository) GetServerOwnerID(ctx context.Context, serverID int64) (int64, error) {
	var ownerID int64
	err := r.db.QueryRowContext(ctx, `SELECT owner_id FROM servers WHERE id=$1`, serverID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return ownerID, err
}

// GetReplyChain returns messages walking the parent_id chain from msgID, oldest first.
// Stops at maxHops hops or when totalChars exceeds charBudget.
func (r *Repository) GetReplyChain(ctx context.Context, msgID int64, maxHops int, charBudget int) ([]ChainMessage, error) {
	var chain []ChainMessage
	current := msgID
	totalChars := 0
	for i := 0; i < maxHops; i++ {
		var cm ChainMessage
		var parentID sql.NullInt64
		err := r.db.QueryRowContext(ctx,
			`SELECT m.id, m.author_id, m.content, m.parent_id, u.is_bot
			 FROM messages m JOIN users u ON u.id = m.author_id
			 WHERE m.id = $1`, current).
			Scan(&cm.ID, &cm.AuthorID, &cm.Content, &parentID, &cm.IsBot)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return nil, err
		}
		chain = append(chain, cm)
		totalChars += len(cm.Content)
		if totalChars > charBudget || !parentID.Valid {
			break
		}
		current = parentID.Int64
	}
	// Reverse to get oldest-first order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// ChainMessage is a lightweight message used for context building.
type ChainMessage struct {
	ID       int64
	AuthorID int64
	Content  string
	IsBot    bool
}

// isPgUniqueViolation checks for Postgres unique constraint violation (code 23505).
// The project uses github.com/lib/pq so we can use the typed error code.
func isPgUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
```

- [ ] **Step 3: Build to verify**

```bash
go build ./internal/bots/...
```
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add internal/bots/
git commit -m "feat(bots): add domain models and repository"
```

---

### Task 3: Bot Service (Encryption + Business Logic)

**Files:**
- Create: `internal/bots/service.go`

- [ ] **Step 1: Create service.go**

```go
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

type Service struct {
	repo      *Repository
	keySecret []byte // 32 bytes for AES-256
}

func NewService(repo *Repository, keySecret []byte) *Service {
	return &Service{repo: repo, keySecret: keySecret}
}

// ListBots returns bots in a server. Any member may call this.
func (s *Service) ListBots(ctx context.Context, serverID int64) ([]Bot, error) {
	return s.repo.ListServerBots(ctx, serverID)
}

// AddBot adds a bot to a server by invite token. Caller must be server admin or owner.
func (s *Service) AddBot(ctx context.Context, serverID, callerID int64, inviteToken string) error {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return err
	}
	botUserID, err := s.repo.ResolveInviteToken(ctx, inviteToken)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.repo.AddBotToServer(ctx, serverID, botUserID)
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
		// Return defaults
		return &AIConfig{Provider: "parley", Model: "ministral-3:14b", SystemPrompt: ""}, nil
	}
	return cfg, nil
}

// SetAIConfig updates the AI config. Pass empty apiKey to keep existing key.
func (s *Service) SetAIConfig(ctx context.Context, serverID, callerID int64, provider, model, systemPrompt, apiKey string) error {
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return err
	}
	var encPtr *string
	if apiKey != "" {
		enc, err := s.encrypt(apiKey)
		if err != nil {
			return fmt.Errorf("encrypt key: %w", err)
		}
		encPtr = &enc
	}
	return s.repo.UpsertAIConfig(ctx, serverID, provider, model, systemPrompt, encPtr)
}

// GetAIUsage returns current month token usage. Caller must be server admin or owner.
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
	limit := ParleyModelAllowances[model]
	if limit == 0 {
		limit = 2_000_000 // fallback
	}
	// Next 1st of month
	now := time.Now().UTC()
	resets := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return &AIUsage{
		TokensUsed:  used,
		TokensLimit: limit,
		Model:       model,
		ResetsAt:    resets.Format(time.RFC3339),
	}, nil
}

// ResolveInvite looks up bot info by invite token (public endpoint).
func (s *Service) ResolveInvite(ctx context.Context, token string) (*BotInviteInfo, error) {
	botUserID, err := s.repo.ResolveInviteToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.repo.GetBotInfo(ctx, botUserID)
}

// AcceptInvite adds a bot to a server via invite link.
// Returns ErrNotFound if token is invalid or caller is not a member (prevents server ID enumeration).
// Returns ErrForbidden if caller is a member but not an admin/owner.
func (s *Service) AcceptInvite(ctx context.Context, token string, serverID, callerID int64) error {
	// Verify caller is a member of the server (returns ErrNotFound if not, preventing enumeration)
	isMember, err := s.repo.IsServerMember(ctx, serverID, callerID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotFound
	}
	// Verify caller has admin permission
	if err := s.requireAdmin(ctx, serverID, callerID); err != nil {
		return err
	}
	botUserID, err := s.repo.ResolveInviteToken(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.repo.AddBotToServer(ctx, serverID, botUserID)
}

// DecryptAPIKey decrypts a stored API key. Used by ai_dispatch.
func (s *Service) DecryptAPIKey(enc string) (string, error) {
	return s.decrypt(enc)
}

// GetRawConfig returns provider/model/encrypted key for dispatch. Not for HTTP clients.
func (s *Service) GetRawConfig(ctx context.Context, serverID int64) (provider, model, encKey string, err error) {
	cfg, rawEnc, e := s.repo.GetAIConfig(ctx, serverID)
	if e != nil {
		return "", "", "", e
	}
	if cfg == nil {
		return "parley", "ministral-3:14b", "", nil
	}
	return cfg.Provider, cfg.Model, rawEnc, nil
}

// requireAdmin checks that callerID has PermAdministrator in the server (or is the owner).
// Uses the same permissions infrastructure as the rest of the codebase.
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
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (s *Service) decrypt(b64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
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
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	return string(plain), err
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/bots/...
```
Expected: no output.

- [ ] **Step 3: Write unit tests for encryption round-trip**

Create `internal/bots/service_test.go`:

```go
package bots

import (
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	svc := &Service{keySecret: key}

	plaintext := "sk-ant-api03-supersecret"
	enc, err := svc.encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plaintext {
		t.Fatal("encrypted value should differ from plaintext")
	}

	got, err := svc.decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesUniqueValues(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	svc := &Service{keySecret: key}
	a, _ := svc.encrypt("same")
	b, _ := svc.encrypt("same")
	if a == b {
		t.Fatal("two encryptions of the same value should differ (random nonce)")
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/bots/... -run TestEncrypt -v
```
Expected: `PASS` for both test cases.

- [ ] **Step 5: Commit**

```bash
git add internal/bots/
git commit -m "feat(bots): add service with AES-256-GCM key encryption"
```

---

### Task 4: AI Provider Dispatch

**Files:**
- Create: `internal/bots/ai_dispatch.go`

- [ ] **Step 1: Create ai_dispatch.go**

```go
// internal/bots/ai_dispatch.go
package bots

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ChatMessage is a provider-agnostic chat message.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
}

// Dispatcher holds dependencies for LLM dispatch.
type Dispatcher struct {
	svc          *Service
	repo         *Repository
	ollamaURL    string
	ollamaAPIKey string
	botUserID    int64
}

func NewDispatcher(svc *Service, repo *Repository, ollamaURL, ollamaAPIKey string, botUserID int64) *Dispatcher {
	return &Dispatcher{svc: svc, repo: repo, ollamaURL: ollamaURL, ollamaAPIKey: ollamaAPIKey, botUserID: botUserID}
}

// TriggerFunc returns a function compatible with message.BotTriggerFunc.
// msgID and channelIDStr are the newly created message; serverID is its server.
// postFn posts a new message as the bot user.
type PostFn func(ctx context.Context, channelID, authorID, content string) error

func (d *Dispatcher) BuildTrigger(postFn PostFn) func(ctx context.Context, msgIDStr, channelIDStr, serverIDStr, authorIDStr, content, parentIDStr string) {
	return func(ctx context.Context, msgIDStr, channelIDStr, serverIDStr, authorIDStr, content, parentIDStr string) {
		// Skip DMs (no server)
		if serverIDStr == "" {
			return
		}
		serverID, err := strconv.ParseInt(serverIDStr, 10, 64)
		if err != nil {
			return
		}
		channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
		if err != nil {
			return
		}
		authorID, _ := strconv.ParseInt(authorIDStr, 10, 64)
		botIDStr := strconv.FormatInt(d.botUserID, 10)

		// Check trigger conditions: mention or reply to bot
		isMention := strings.Contains(content, "<@"+botIDStr+">")
		isReply := parentIDStr != "" // whether this is a reply at all — we also need parent author

		if !isMention && !isReply {
			return
		}

		// If it's a reply, verify the parent is by the bot
		if isReply && !isMention {
			parentID, e := strconv.ParseInt(parentIDStr, 10, 64)
			if e != nil {
				return
			}
			var parentAuthorID int64
			if e := d.repo.db.QueryRowContext(ctx,
				`SELECT author_id FROM messages WHERE id=$1`, parentID).Scan(&parentAuthorID); e != nil {
				return
			}
			if parentAuthorID != d.botUserID {
				return
			}
		}

		// Check bot is in server
		inServer, err := d.repo.IsBotInServer(ctx, serverID, d.botUserID)
		if err != nil || !inServer {
			return
		}

		// Get AI config
		provider, model, encKey, err := d.svc.GetRawConfig(ctx, serverID)
		if err != nil {
			return
		}

		// Decrypt API key if needed
		apiKey := ""
		if encKey != "" {
			apiKey, _ = d.svc.DecryptAPIKey(encKey)
		}

		// Budget check for Parley provider
		if provider == "parley" {
			used, _ := d.repo.GetMonthlyUsage(ctx, serverID)
			limit := ParleyModelAllowances[model]
			if limit == 0 {
				limit = 2_000_000
			}
			if used >= limit {
				return
			}
		}

		// Build context from reply chain
		msgID, _ := strconv.ParseInt(msgIDStr, 10, 64)
		chain, _ := d.repo.GetReplyChain(ctx, msgID, 50, 32_000) // 32000 chars ≈ 8000 tokens

		var messages []ChatMessage
		for _, cm := range chain {
			role := "user"
			if cm.AuthorID == d.botUserID {
				role = "assistant"
			}
			messages = append(messages, ChatMessage{Role: role, Content: cm.Content})
		}

		// Ensure context starts with "user" role (required by most providers)
		if len(messages) > 0 && messages[0].Role == "assistant" {
			messages = append([]ChatMessage{{Role: "user", Content: "..."}}, messages...)
		}
		if len(messages) == 0 {
			messages = []ChatMessage{{Role: "user", Content: content}}
		}

		// Get system prompt
		sysPrompt := "You are a helpful assistant in a chat server."
		cfg, _, _ := d.repo.GetAIConfig(ctx, serverID)
		if cfg != nil && cfg.SystemPrompt != "" {
			sysPrompt = cfg.SystemPrompt
		}

		// Dispatch in goroutine with timeout
		go func() {
			dispatchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			response, tokenDelta, err := d.dispatch(dispatchCtx, provider, model, apiKey, sysPrompt, messages)
			if err != nil {
				log.Printf("bots: dispatch error (server=%d provider=%s): %v", serverID, provider, err)
				return
			}

			botIDStr := strconv.FormatInt(d.botUserID, 10)
			if err := postFn(dispatchCtx, channelIDStr, botIDStr, response); err != nil {
				log.Printf("bots: post response error: %v", err)
				return
			}

			if provider == "parley" || tokenDelta > 0 {
				if err := d.repo.AddTokenUsage(context.Background(), serverID, tokenDelta); err != nil {
					log.Printf("bots: token usage update error: %v", err)
				}
			}
		}()

		_ = authorID // suppress unused warning
		_ = channelID
	}
}

// dispatch sends messages to the configured provider and returns the response text and token count.
func (d *Dispatcher) dispatch(ctx context.Context, provider, model, apiKey, sysPrompt string, messages []ChatMessage) (string, int64, error) {
	switch provider {
	case "parley":
		return d.dispatchOllama(ctx, model+":cloud", messages, sysPrompt)
	case "anthropic":
		return d.dispatchAnthropic(ctx, model, apiKey, sysPrompt, messages)
	case "openai", "xai", "mistral":
		return d.dispatchOpenAICompat(ctx, provider, model, apiKey, sysPrompt, messages)
	case "google":
		return d.dispatchGoogle(ctx, model, apiKey, sysPrompt, messages)
	default:
		return "", 0, fmt.Errorf("unknown provider: %s", provider)
	}
}

// dispatchOllama uses the existing Ollama Cloud API endpoint.
func (d *Dispatcher) dispatchOllama(ctx context.Context, model string, messages []ChatMessage, sysPrompt string) (string, int64, error) {
	type ollamaReq struct {
		Model    string        `json:"model"`
		Messages []ChatMessage `json:"messages"`
		Stream   bool          `json:"stream"`
	}
	type ollamaResp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		EvalCount         int64 `json:"eval_count"`
		PromptEvalCount   int64 `json:"prompt_eval_count"`
	}

	msgs := append([]ChatMessage{{Role: "system", Content: sysPrompt}}, messages...)
	body, _ := json.Marshal(ollamaReq{Model: model, Messages: msgs, Stream: false})

	req, _ := http.NewRequestWithContext(ctx, "POST", d.ollamaURL+"/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if d.ollamaAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.ollamaAPIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("ollama %d: %s", resp.StatusCode, raw)
	}
	var out ollamaResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", 0, err
	}
	tokens := out.EvalCount + out.PromptEvalCount
	if tokens == 0 {
		// Estimate if API doesn't return counts
		total := 0
		for _, m := range messages {
			total += len(m.Content)
		}
		total += len(out.Message.Content)
		tokens = int64(total / 4)
	}
	return out.Message.Content, tokens, nil
}

// dispatchAnthropic calls the Anthropic Messages API.
func (d *Dispatcher) dispatchAnthropic(ctx context.Context, model, apiKey, sysPrompt string, messages []ChatMessage) (string, int64, error) {
	type anthropicReq struct {
		Model     string        `json:"model"`
		MaxTokens int           `json:"max_tokens"`
		System    string        `json:"system,omitempty"`
		Messages  []ChatMessage `json:"messages"`
	}
	type anthropicResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}

	body, _ := json.Marshal(anthropicReq{
		Model: model, MaxTokens: 1024, System: sysPrompt, Messages: messages,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("anthropic %d: %s", resp.StatusCode, raw)
	}
	var out anthropicResp
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Content) == 0 {
		return "", 0, fmt.Errorf("anthropic parse error")
	}
	return out.Content[0].Text, out.Usage.InputTokens + out.Usage.OutputTokens, nil
}

// dispatchOpenAICompat handles OpenAI, xAI, and Mistral (all use OpenAI-compatible API).
func (d *Dispatcher) dispatchOpenAICompat(ctx context.Context, provider, model, apiKey, sysPrompt string, messages []ChatMessage) (string, int64, error) {
	baseURLs := map[string]string{
		"openai":  "https://api.openai.com/v1",
		"xai":     "https://api.x.ai/v1",
		"mistral": "https://api.mistral.ai/v1",
	}
	baseURL := baseURLs[provider]

	type openAIReq struct {
		Model    string        `json:"model"`
		Messages []ChatMessage `json:"messages"`
	}
	type openAIResp struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int64 `json:"total_tokens"`
		} `json:"usage"`
	}

	msgs := append([]ChatMessage{{Role: "system", Content: sysPrompt}}, messages...)
	body, _ := json.Marshal(openAIReq{Model: model, Messages: msgs})
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("%s %d: %s", provider, resp.StatusCode, raw)
	}
	var out openAIResp
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", 0, fmt.Errorf("%s parse error", provider)
	}
	return out.Choices[0].Message.Content, out.Usage.TotalTokens, nil
}

// dispatchGoogle calls the Gemini generateContent API.
func (d *Dispatcher) dispatchGoogle(ctx context.Context, model, apiKey, sysPrompt string, messages []ChatMessage) (string, int64, error) {
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}
	type googleReq struct {
		SystemInstruction *content  `json:"system_instruction,omitempty"`
		Contents          []content `json:"contents"`
	}
	type googleResp struct {
		Candidates []struct {
			Content struct {
				Parts []part `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			TotalTokenCount int64 `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	var contents []content
	for _, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, content{Role: role, Parts: []part{{Text: m.Content}}})
	}
	reqBody := googleReq{
		SystemInstruction: &content{Parts: []part{{Text: sysPrompt}}},
		Contents:          contents,
	}
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("google %d: %s", resp.StatusCode, raw)
	}
	var out googleResp
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", 0, fmt.Errorf("google parse error")
	}
	return out.Candidates[0].Content.Parts[0].Text, out.UsageMetadata.TotalTokenCount, nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/bots/...
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/bots/
git commit -m "feat(bots): add multi-provider AI dispatch"
```

---

## Chunk 2: HTTP Layer + Wiring

### Task 5: HTTP Handlers

**Files:**
- Create: `internal/bots/handler.go`

- [ ] **Step 1: Create handler.go**

```go
// internal/bots/handler.go
package bots

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"parley/internal/server"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func writeErr(w http.ResponseWriter, r *http.Request, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	render.JSON(w, r, map[string]string{"error": msg})
}

func callerID(r *http.Request) (int64, bool) {
	s, ok := r.Context().Value(server.UserIDKey).(string)
	if !ok || s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}

func serverIDParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil
}

func handleSvcErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, r, 404, "not found")
	case errors.Is(err, ErrForbidden):
		writeErr(w, r, 403, "forbidden")
	case errors.Is(err, ErrAlreadyExists):
		writeErr(w, r, 409, "already exists")
	default:
		writeErr(w, r, 500, err.Error())
	}
}

// ListBots handles GET /api/servers/{id}/bots
func (h *Handler) ListBots(w http.ResponseWriter, r *http.Request) {
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id"); return
	}
	bots, err := h.svc.ListBots(r.Context(), sid)
	if err != nil {
		handleSvcErr(w, r, err); return
	}
	render.JSON(w, r, bots)
}

// AddBot handles POST /api/servers/{id}/bots
func (h *Handler) AddBot(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized"); return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id"); return
	}
	var req struct {
		InviteToken string `json:"invite_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.InviteToken == "" {
		writeErr(w, r, 400, "invite_token required"); return
	}
	if err := h.svc.AddBot(r.Context(), sid, uid, req.InviteToken); err != nil {
		handleSvcErr(w, r, err); return
	}
	w.WriteHeader(204)
}

// RemoveBot handles DELETE /api/servers/{id}/bots/{botId}
func (h *Handler) RemoveBot(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized"); return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id"); return
	}
	botID, err := strconv.ParseInt(chi.URLParam(r, "botId"), 10, 64)
	if err != nil {
		writeErr(w, r, 400, "invalid bot id"); return
	}
	if err := h.svc.RemoveBot(r.Context(), sid, botID, uid); err != nil {
		handleSvcErr(w, r, err); return
	}
	w.WriteHeader(204)
}

// GetAIConfig handles GET /api/servers/{id}/ai-config
func (h *Handler) GetAIConfig(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized"); return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id"); return
	}
	cfg, err := h.svc.GetAIConfig(r.Context(), sid, uid)
	if err != nil {
		handleSvcErr(w, r, err); return
	}
	render.JSON(w, r, cfg)
}

// SetAIConfig handles PUT /api/servers/{id}/ai-config
func (h *Handler) SetAIConfig(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized"); return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id"); return
	}
	var req struct {
		Provider     string `json:"provider"`
		Model        string `json:"model"`
		APIKey       string `json:"api_key"`
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, 400, "invalid body"); return
	}
	if req.Provider == "" || req.Model == "" {
		writeErr(w, r, 400, "provider and model required"); return
	}
	if err := h.svc.SetAIConfig(r.Context(), sid, uid, req.Provider, req.Model, req.SystemPrompt, req.APIKey); err != nil {
		handleSvcErr(w, r, err); return
	}
	w.WriteHeader(204)
}

// GetAIUsage handles GET /api/servers/{id}/ai-usage
func (h *Handler) GetAIUsage(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized"); return
	}
	sid, ok := serverIDParam(r)
	if !ok {
		writeErr(w, r, 400, "invalid server id"); return
	}
	usage, err := h.svc.GetAIUsage(r.Context(), sid, uid)
	if err != nil {
		handleSvcErr(w, r, err); return
	}
	render.JSON(w, r, usage)
}

// ResolveInvite handles GET /api/bots/invite/{token} (public)
func (h *Handler) ResolveInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	info, err := h.svc.ResolveInvite(r.Context(), token)
	if err != nil {
		handleSvcErr(w, r, err); return
	}
	render.JSON(w, r, info)
}

// AcceptInvite handles POST /api/bots/invite/{token}/accept
func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	uid, ok := callerID(r)
	if !ok {
		writeErr(w, r, 401, "unauthorized"); return
	}
	token := chi.URLParam(r, "token")
	var req struct {
		ServerID int64 `json:"server_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ServerID == 0 {
		writeErr(w, r, 400, "server_id required"); return
	}
	if err := h.svc.AcceptInvite(r.Context(), token, req.ServerID, uid); err != nil {
		handleSvcErr(w, r, err); return
	}
	w.WriteHeader(204)
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/bots/...
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/bots/handler.go
git commit -m "feat(bots): add HTTP handlers"
```

---

### Task 6: Message Trigger Hook + Route Wiring

**Files:**
- Modify: `internal/message/service.go`
- Modify: `cmd/api/routes.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add BotTriggerFunc to MessageService**

Open `internal/message/service.go`.

After the `SetBroadcaster` method (around line 67), add:

```go
// BotTriggerFunc is called after a message is created. All args are strings for
// loose coupling (no import of the bots package).
type BotTriggerFunc func(ctx context.Context, msgID, channelID, serverID, authorID, content, parentID string)

// SetBotTrigger registers a function to call after each message is created.
func (s *MessageService) SetBotTrigger(fn BotTriggerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.botTrigger = fn
}
```

Add `botTrigger BotTriggerFunc` to the `MessageService` struct fields (after `broadcaster Broadcaster`).

At the end of `SendMessage`, just before `return msg, nil` (after the broadcast block), add:

```go
// Fire bot trigger asynchronously so it never blocks the HTTP response
s.mu.RLock()
trigger := s.botTrigger
s.mu.RUnlock()
if trigger != nil {
    parentIDStr := ""
    if msg.ParentID != nil {
        parentIDStr = strconv.FormatInt(*msg.ParentID, 10)
    }
    // Resolve server ID for this channel
    var serverIDStr string
    var srvID sql.NullInt64
    if e := s.repo.DB().QueryRowContext(ctx, `SELECT server_id FROM channels WHERE id=$1`,
        mustParseInt64(channelID)).Scan(&srvID); e == nil && srvID.Valid {
        serverIDStr = strconv.FormatInt(srvID.Int64, 10)
    }
    trigger(ctx, msg.ID, channelID, serverIDStr, authorID, content, parentIDStr)
}
```

Also add a small helper at the bottom of `service.go` (outside any function):

```go
func mustParseInt64(s string) int64 {
    n, _ := strconv.ParseInt(s, 10, 64)
    return n
}
```

Add `"database/sql"` to imports if not already present.

- [ ] **Step 2: Build message package**

```bash
go build ./internal/message/...
```
Expected: no output.

- [ ] **Step 3: Wire bots into routes.go**

Open `cmd/api/routes.go`. Find the `registerRoutes` function signature. Add `botsHandler *bots.Handler` as a new parameter (after existing params). Inside the function, within the authenticated route group (near the server routes), add:

```go
// Bot routes
botsH := botsHandler
r.Get("/servers/{id}/bots", botsH.ListBots)
r.Post("/servers/{id}/bots", botsH.AddBot)
r.Delete("/servers/{id}/bots/{botId}", botsH.RemoveBot)
r.Get("/servers/{id}/ai-config", botsH.GetAIConfig)
r.Put("/servers/{id}/ai-config", botsH.SetAIConfig)
r.Get("/servers/{id}/ai-usage", botsH.GetAIUsage)

// Public bot invite route (no auth required)
r.Get("/bots/invite/{token}", botsH.ResolveInvite)
// Authenticated bot invite accept (must match the same auth group middleware pattern used above)
r.With(auth.AuthMiddlewareWith(authService), bridgeUserIDMiddleware).
    Post("/bots/invite/{token}/accept", botsH.AcceptInvite)
```

Add `import "parley/internal/bots"` to the routes.go imports.

- [ ] **Step 4: Wire bots in main.go**

Open `cmd/api/main.go`. After the `OllamaModel` field in the `Config` struct, add:

```go
BotKeySecret string // BOT_KEY_SECRET — 32-byte hex or base64, required
```

In the env loading section, add:

```go
botKeySecret := os.Getenv("BOT_KEY_SECRET")
if botKeySecret == "" {
    log.Fatal("BOT_KEY_SECRET is required (32-byte AES key for bot API key encryption)")
}
// Decode as raw bytes (pad or truncate to 32)
botKeyBytes := []byte(botKeySecret)
if len(botKeyBytes) < 32 {
    log.Fatalf("BOT_KEY_SECRET must be at least 32 bytes (got %d)", len(botKeyBytes))
}
botKeyBytes = botKeyBytes[:32] // use exactly 32 bytes for AES-256
```

After the existing services are initialized (near line 240), add:

```go
// Initialize bots service
// `repo` is the existing *db.Repository initialized earlier in main()
botsRepo := bots.NewRepository(repo)
botsSvc := bots.NewService(botsRepo, botKeyBytes)
botsHandler := bots.NewHandler(botsSvc)

// Cache bot user ID at startup (fatal if not found — migration must have run)
botUserID, err := botsRepo.GetBotUserID(context.Background(), "ai-chatbot")
if err != nil {
    log.Fatalf("bot user 'ai-chatbot' not found — run migrations: %v", err)
}

// Wire AI dispatch as a message trigger.
// BuildTrigger is called ONCE here to produce a stable trigger function (not per-message).
dispatcher := bots.NewDispatcher(botsSvc, botsRepo, config.OllamaAPIURL, config.OllamaAPIKey, botUserID)
postFn := func(ctx context.Context, chID, authorID, content string) error {
    _, err := messageService.SendMessage(ctx, chID, authorID, content, "", "", "", "", "")
    return err
}
botTriggerFn := dispatcher.BuildTrigger(postFn)
messageService.SetBotTrigger(botTriggerFn)
```

Update the `registerRoutes` call to pass `botsHandler`:

```go
registerRoutes(...existing args..., botsHandler)
```

Add `import "parley/internal/bots"` to main.go imports.

- [ ] **Step 5: Build everything**

```bash
go build ./...
```
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add internal/message/service.go cmd/api/routes.go cmd/api/main.go
git commit -m "feat(bots): wire trigger hook, routes, and startup initialization"
```

---

## Chunk 3: Frontend

### Task 7: Shared EmbedCard + ThemeLinkEmbed Refactor

**Files:**
- Create: `frontend/src/components/EmbedCard.tsx`
- Create: `frontend/src/components/EmbedCard.css`
- Modify: `frontend/src/components/theme/ThemeLinkEmbed.tsx`

- [ ] **Step 1: Read existing ThemeLinkEmbed**

Read `frontend/src/components/theme/ThemeLinkEmbed.tsx` to understand its current layout before refactoring.

- [ ] **Step 2: Create EmbedCard.css**

```css
/* frontend/src/components/EmbedCard.css */
.embed-card {
  max-width: 440px;
  margin: 40px auto;
  background: var(--parley-bg-secondary, #1e1e1e);
  border: 1px solid var(--parley-border, #333);
  border-radius: 8px;
  overflow: hidden;
  font-family: sans-serif;
  color: var(--parley-text, #eee);
}

.embed-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 16px 0;
}

.embed-card-icon {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  flex-shrink: 0;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  font-weight: 700;
  background: var(--parley-accent, #32CD32);
  color: #fff;
}

.embed-card-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.embed-card-title-group {
  flex: 1;
  min-width: 0;
}

.embed-card-title {
  font-size: 16px;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 6px;
}

.embed-card-subtitle {
  font-size: 13px;
  color: var(--parley-text-muted, #888);
  margin-top: 2px;
}

.embed-card-badge {
  display: inline-flex;
  align-items: center;
  background: var(--parley-accent, #32CD32);
  color: #fff;
  border-radius: 50%;
  width: 16px;
  height: 16px;
  font-size: 10px;
  justify-content: center;
  flex-shrink: 0;
}

.embed-card-preview {
  margin: 12px 16px 0;
  border-radius: 4px;
  overflow: hidden;
}

.embed-card-body {
  padding: 12px 16px 0;
}

.embed-card-actions {
  padding: 12px 16px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
```

- [ ] **Step 3: Create EmbedCard.tsx**

```tsx
// frontend/src/components/EmbedCard.tsx
import React from 'react';
import './EmbedCard.css';

interface EmbedCardProps {
  icon?: React.ReactNode;
  title: string;
  subtitle?: string;
  badge?: boolean;       // shows verified ✓ chip next to title
  preview?: React.ReactNode;
  children?: React.ReactNode;
  actions: React.ReactNode;
}

export const EmbedCard: React.FC<EmbedCardProps> = ({
  icon, title, subtitle, badge, preview, children, actions,
}) => (
  <div className="embed-card">
    {(icon || title) && (
      <div className="embed-card-header">
        {icon && <div className="embed-card-icon">{icon}</div>}
        <div className="embed-card-title-group">
          <div className="embed-card-title">
            {title}
            {badge && <span className="embed-card-badge" title="Verified">✓</span>}
          </div>
          {subtitle && <div className="embed-card-subtitle">{subtitle}</div>}
        </div>
      </div>
    )}
    {preview && <div className="embed-card-preview">{preview}</div>}
    {children && <div className="embed-card-body">{children}</div>}
    <div className="embed-card-actions">{actions}</div>
  </div>
);
```

- [ ] **Step 4: Refactor ThemeLinkEmbed to use EmbedCard**

Read the current `ThemeLinkEmbed.tsx` then replace it. The component keeps all existing logic; only the JSX shell changes to use `EmbedCard`. The existing `.theme-embed` card wrapper CSS can be removed from `ThemeLinkEmbed.css` (or left as override styles).

The `EmbedCard` for themes maps as:
- `icon` = undefined (theme has no avatar)
- `title` = theme name
- `subtitle` = `by {theme.author_username}`
- `preview` = existing `<iframe>` sandboxed preview
- `actions` = existing install button

Replace the return JSX in `ThemeLinkEmbed.tsx` to wrap with `<EmbedCard>` instead of the custom div shell. Keep all state and logic untouched — only the outer JSX changes.

- [ ] **Step 5: Build frontend to verify**

```bash
cd frontend && npm run build 2>&1 | tail -20
```
Expected: `built in Xs` with no TypeScript errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/EmbedCard.tsx frontend/src/components/EmbedCard.css frontend/src/components/theme/ThemeLinkEmbed.tsx
git commit -m "feat(bots): add EmbedCard component, refactor ThemeLinkEmbed to use it"
```

---

### Task 8: Bot API Client + BotsTab + BotConfigPanel

**Files:**
- Create: `frontend/src/api/bots.ts`
- Create: `frontend/src/components/settings/BotsTab.tsx`
- Create: `frontend/src/components/settings/BotConfigPanel.tsx`
- Create: `frontend/src/components/settings/BotsTab.css`
- Modify: `frontend/src/components/settings/ServerSettings.tsx`

- [ ] **Step 1: Create api/bots.ts**

```typescript
// frontend/src/api/bots.ts
import { apiClient } from './client';

export interface BotSummary {
  id: number;
  username: string;
  display_name: string;
  avatar_url?: string;
  is_verified: boolean;
  added_at: string;
}

export interface AIConfig {
  provider: string;
  model: string;
  api_key_set: boolean;
  system_prompt: string;
  updated_at: string;
}

export interface AIUsage {
  tokens_used: number;
  tokens_limit: number;
  model: string;
  resets_at: string;
}

export interface BotInviteInfo {
  bot_id: number;
  username: string;
  display_name: string;
  avatar_url?: string;
  is_verified: boolean;
}

export const listBots = (serverId: number) =>
  apiClient.get<BotSummary[]>(`/servers/${serverId}/bots`);

export const addBot = (serverId: number, inviteToken: string) =>
  apiClient.post<void>(`/servers/${serverId}/bots`, { invite_token: inviteToken });

export const removeBot = (serverId: number, botId: number) =>
  apiClient.delete<void>(`/servers/${serverId}/bots/${botId}`);

export const getAIConfig = (serverId: number) =>
  apiClient.get<AIConfig>(`/servers/${serverId}/ai-config`);

export const setAIConfig = (serverId: number, data: {
  provider: string; model: string; api_key?: string; system_prompt: string;
}) => apiClient.put<void>(`/servers/${serverId}/ai-config`, data);

export const getAIUsage = (serverId: number) =>
  apiClient.get<AIUsage>(`/servers/${serverId}/ai-usage`);

export const resolveBotInvite = (token: string) =>
  apiClient.get<BotInviteInfo>(`/bots/invite/${token}`);

export const acceptBotInvite = (token: string, serverId: number) =>
  apiClient.post<void>(`/bots/invite/${token}/accept`, { server_id: serverId });

export const PROVIDER_MODELS: Record<string, { label: string; value: string }[]> = {
  parley: [
    { label: 'Ministral 3 (14B)', value: 'ministral-3:14b' },
    { label: 'GPT-OSS (20B)',      value: 'gpt-oss:20b' },
    { label: 'Gemma 3 (27B)',      value: 'gemma3:27b' },
    { label: 'GPT-OSS (120B)',     value: 'gpt-oss:120b' },
    { label: 'Qwen3.5 (397B)',     value: 'qwen3:latest' },
  ],
  anthropic: [
    { label: 'Claude Opus 4.5',         value: 'claude-opus-4-5' },
    { label: 'Claude Sonnet 4.5',       value: 'claude-sonnet-4-5' },
    { label: 'Claude Haiku 4.5',        value: 'claude-haiku-4-5-20251001' },
  ],
  openai: [
    { label: 'GPT-4.1',      value: 'gpt-4.1' },
    { label: 'GPT-4.1 Mini', value: 'gpt-4.1-mini' },
    { label: 'GPT-4o',       value: 'gpt-4o' },
    { label: 'o3-mini',      value: 'o3-mini' },
  ],
  xai: [
    { label: 'Grok 3',      value: 'grok-3' },
    { label: 'Grok 3 Mini', value: 'grok-3-mini' },
  ],
  mistral: [
    { label: 'Mistral Large',  value: 'mistral-large-latest' },
    { label: 'Mistral Small',  value: 'mistral-small-latest' },
    { label: 'Codestral',      value: 'codestral-latest' },
  ],
  google: [
    { label: 'Gemini 2.5 Pro',       value: 'gemini-2.5-pro' },
    { label: 'Gemini 2.0 Flash',     value: 'gemini-2.0-flash' },
    { label: 'Gemini 2.0 Flash Lite',value: 'gemini-2.0-flash-lite' },
  ],
};

export const PROVIDER_LABELS: Record<string, string> = {
  parley: 'Parley', anthropic: 'Anthropic', openai: 'OpenAI',
  xai: 'xAI', mistral: 'Mistral', google: 'Google',
};

export const PARLEY_ALLOWANCES: Record<string, number> = {
  'ministral-3:14b': 2_000_000,
  'gpt-oss:20b':     1_500_000,
  'gemma3:27b':      1_000_000,
  'gpt-oss:120b':    300_000,
  'qwen3:latest':    100_000,
};
```

- [ ] **Step 2: Create BotsTab.css**

```css
/* frontend/src/components/settings/BotsTab.css */
.bots-tab {
  display: flex;
  gap: 0;
  height: 100%;
  min-height: 400px;
}

.bots-list-panel {
  width: 200px;
  flex-shrink: 0;
  border-right: 1px solid var(--parley-border, #333);
  padding: 12px 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.bots-list-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: .05em;
  color: var(--parley-text-muted, #888);
  padding: 0 8px;
  margin-bottom: 4px;
}

.bots-list-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  border: none;
  background: none;
  color: var(--parley-text, #eee);
  width: 100%;
  text-align: left;
  font-size: 13px;
}
.bots-list-item:hover { background: var(--parley-bg-hover, #2a2a2a); }
.bots-list-item.active { background: var(--parley-bg-hover, #2a2a2a); }

.bots-list-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--parley-accent, #32CD32);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 700;
  color: #fff;
  flex-shrink: 0;
}

.bots-list-name {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bots-verified {
  font-size: 10px;
  background: var(--parley-accent, #32CD32);
  color: #fff;
  border-radius: 50%;
  width: 14px;
  height: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.bots-add-btn {
  margin-top: 8px;
  background: none;
  border: 1px dashed var(--parley-border, #444);
  color: var(--parley-text-muted, #888);
  border-radius: 4px;
  padding: 6px 8px;
  font-size: 12px;
  cursor: pointer;
  width: 100%;
}
.bots-add-btn:hover { border-color: var(--parley-accent, #32CD32); color: var(--parley-accent, #32CD32); }

.bots-config-panel {
  flex: 1;
  padding: 16px 20px;
  overflow-y: auto;
}

.bots-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--parley-text-muted, #888);
  font-size: 13px;
}

.bots-add-modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.bots-add-modal {
  background: var(--parley-bg-secondary, #2a2a2a);
  border: 1px solid var(--parley-border, #444);
  border-radius: 8px;
  padding: 20px;
  width: 360px;
}
.bots-add-modal h3 { margin: 0 0 12px; font-size: 15px; }
.bots-add-modal input {
  width: 100%;
  box-sizing: border-box;
  background: var(--parley-input, #1a1a1a);
  border: 1px solid var(--parley-border, #444);
  border-radius: 4px;
  color: var(--parley-text, #eee);
  padding: 8px 10px;
  font-size: 13px;
  margin-bottom: 8px;
}
.bots-add-modal-actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 4px; }
.bots-modal-cancel { background: none; border: 1px solid var(--parley-border,#444); color: var(--parley-text,#eee); border-radius:4px; padding:6px 14px; cursor:pointer; font-size:13px; }
.bots-modal-submit { background: var(--parley-accent,#32CD32); border:none; color:#fff; border-radius:4px; padding:6px 14px; cursor:pointer; font-size:13px; font-weight:600; }
```

- [ ] **Step 3: Create BotsTab.tsx**

```tsx
// frontend/src/components/settings/BotsTab.tsx
import React, { useEffect, useState } from 'react';
import { BotSummary, listBots, addBot, removeBot } from '../../api/bots';
import { BotConfigPanel } from './BotConfigPanel';
import './BotsTab.css';

interface Props {
  serverId: number;
  isOwner: boolean;
}

export const BotsTab: React.FC<Props> = ({ serverId, isOwner }) => {
  const [bots, setBots] = useState<BotSummary[]>([]);
  const [selected, setSelected] = useState<BotSummary | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [inviteInput, setInviteInput] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState('');

  useEffect(() => {
    listBots(serverId).then(setBots).catch(() => {});
  }, [serverId]);

  const handleAdd = async () => {
    // Extract token from URL or raw token
    const token = inviteInput.includes('/bots/invite/')
      ? inviteInput.split('/bots/invite/')[1].split('?')[0]
      : inviteInput.trim();
    if (!token) return;
    setAdding(true);
    setAddError('');
    try {
      await addBot(serverId, token);
      const updated = await listBots(serverId);
      setBots(updated);
      setShowAdd(false);
      setInviteInput('');
    } catch {
      setAddError('Failed to add bot. Check the invite link.');
    } finally {
      setAdding(false);
    }
  };

  const handleRemove = async (bot: BotSummary) => {
    if (!window.confirm(`Remove ${bot.display_name} from this server?`)) return;
    await removeBot(serverId, bot.id).catch(() => {});
    setBots(prev => prev.filter(b => b.id !== bot.id));
    if (selected?.id === bot.id) setSelected(null);
  };

  return (
    <div className="bots-tab">
      <div className="bots-list-panel">
        <div className="bots-list-title">Bots</div>
        {bots.map(bot => (
          <button
            key={bot.id}
            className={`bots-list-item${selected?.id === bot.id ? ' active' : ''}`}
            onClick={() => setSelected(bot)}
          >
            <div className="bots-list-avatar">
              {bot.avatar_url
                ? <img src={bot.avatar_url} alt="" style={{ width: '100%', height: '100%', borderRadius: '50%', objectFit: 'cover' }} />
                : bot.display_name.charAt(0).toUpperCase()}
            </div>
            <span className="bots-list-name">{bot.display_name}</span>
            {bot.is_verified && <span className="bots-verified" title="Verified">✓</span>}
          </button>
        ))}
        {isOwner && (
          <button className="bots-add-btn" onClick={() => setShowAdd(true)}>+ Add Bot</button>
        )}
      </div>

      {selected ? (
        <BotConfigPanel
          bot={selected}
          serverId={serverId}
          isOwner={isOwner}
          onRemove={() => handleRemove(selected)}
        />
      ) : (
        <div className="bots-empty">
          {bots.length === 0 ? 'No bots yet. Add a bot to get started.' : 'Select a bot to configure it.'}
        </div>
      )}

      {showAdd && (
        <div className="bots-add-modal-overlay" onClick={() => setShowAdd(false)}>
          <div className="bots-add-modal" onClick={e => e.stopPropagation()}>
            <h3>Add Bot</h3>
            <input
              placeholder="Paste a bot invite link or token"
              value={inviteInput}
              onChange={e => setInviteInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleAdd()}
              autoFocus
            />
            {addError && <div style={{ fontSize: 12, color: 'var(--parley-danger,#f04747)', marginBottom: 4 }}>{addError}</div>}
            <div className="bots-add-modal-actions">
              <button className="bots-modal-cancel" onClick={() => setShowAdd(false)}>Cancel</button>
              <button className="bots-modal-submit" onClick={handleAdd} disabled={adding}>
                {adding ? 'Adding…' : 'Add'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 4: Create BotConfigPanel.tsx**

```tsx
// frontend/src/components/settings/BotConfigPanel.tsx
import React, { useEffect, useState } from 'react';
import {
  BotSummary, AIConfig, AIUsage,
  getAIConfig, setAIConfig, getAIUsage,
  PROVIDER_MODELS, PROVIDER_LABELS, PARLEY_ALLOWANCES,
} from '../../api/bots';

interface Props {
  bot: BotSummary;
  serverId: number;
  isOwner: boolean;
  onRemove: () => void;
}

export const BotConfigPanel: React.FC<Props> = ({ bot, serverId, isOwner, onRemove }) => {
  const isAIChatbot = bot.username === 'ai-chatbot';

  const [config, setConfig] = useState<AIConfig | null>(null);
  const [usage, setUsage] = useState<AIUsage | null>(null);
  const [provider, setProvider] = useState('parley');
  const [model, setModel] = useState('ministral-3:14b');
  const [systemPrompt, setSystemPrompt] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState('');

  useEffect(() => {
    if (!isAIChatbot || !isOwner) return;
    getAIConfig(serverId).then(cfg => {
      setConfig(cfg);
      setProvider(cfg.provider);
      setModel(cfg.model);
      setSystemPrompt(cfg.system_prompt);
    }).catch(() => {});
    getAIUsage(serverId).then(setUsage).catch(() => {});
  }, [serverId, isAIChatbot, isOwner]);

  const handleProviderChange = (p: string) => {
    setProvider(p);
    const models = PROVIDER_MODELS[p];
    if (models?.length) setModel(models[0].value);
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveMsg('');
    try {
      await setAIConfig(serverId, { provider, model, system_prompt: systemPrompt, api_key: apiKey || undefined });
      setSaveMsg('Saved!');
      setApiKey('');
      // Refresh usage
      if (provider === 'parley') {
        getAIUsage(serverId).then(setUsage).catch(() => {});
      }
    } catch {
      setSaveMsg('Save failed.');
    } finally {
      setSaving(false);
      setTimeout(() => setSaveMsg(''), 3000);
    }
  };

  const formatTokens = (n: number) => {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(0) + 'K';
    return String(n);
  };

  const resetDate = usage ? new Date(usage.resets_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : '';

  const sectionTitle: React.CSSProperties = {
    fontSize: 11, fontWeight: 700, textTransform: 'uppercase',
    letterSpacing: '.05em', color: 'var(--parley-text-muted,#888)', marginBottom: 8,
  };
  const label: React.CSSProperties = {
    fontSize: 12, color: 'var(--parley-text-muted,#888)', marginBottom: 4, display: 'block',
  };
  const input: React.CSSProperties = {
    width: '100%', boxSizing: 'border-box',
    background: 'var(--parley-input,#1a1a1a)',
    border: '1px solid var(--parley-border,#444)',
    borderRadius: 4, color: 'var(--parley-text,#eee)',
    padding: '7px 10px', fontSize: 13, marginBottom: 12,
  };
  const select: React.CSSProperties = { ...input, cursor: 'pointer' };

  return (
    <div style={{ flex: 1, padding: '16px 20px', overflowY: 'auto' }}>
      {/* Bot info header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <div style={{
          width: 40, height: 40, borderRadius: '50%',
          background: 'var(--parley-accent,#32CD32)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: 18, fontWeight: 700, color: '#fff',
        }}>
          {bot.display_name.charAt(0).toUpperCase()}
        </div>
        <div>
          <div style={{ fontWeight: 700, fontSize: 15, display: 'flex', alignItems: 'center', gap: 6 }}>
            {bot.display_name}
            {bot.is_verified && (
              <span style={{ background: 'var(--parley-accent,#32CD32)', color: '#fff', borderRadius: '50%', width: 16, height: 16, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 10 }}>✓</span>
            )}
          </div>
          <div style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)' }}>@{bot.username} · Bot</div>
        </div>
      </div>

      {isAIChatbot && isOwner && (
        <>
          <div style={sectionTitle}>AI Provider</div>

          <label style={label}>Provider</label>
          <select style={select} value={provider} onChange={e => handleProviderChange(e.target.value)}>
            {Object.entries(PROVIDER_LABELS).map(([id, label]) => (
              <option key={id} value={id}>{label}</option>
            ))}
          </select>

          <label style={label}>Model</label>
          <select style={select} value={model} onChange={e => setModel(e.target.value)}>
            {(PROVIDER_MODELS[provider] ?? []).map(m => (
              <option key={m.value} value={m.value}>{m.label}</option>
            ))}
          </select>

          {provider !== 'parley' && (
            <>
              <label style={label}>
                API Key {config?.api_key_set && <span style={{ color: 'var(--parley-success,#43b581)' }}>● Set</span>}
              </label>
              <input
                type="password"
                style={input}
                placeholder={config?.api_key_set ? '••••••••••••••••' : 'Enter API key…'}
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                autoComplete="new-password"
              />
            </>
          )}

          <label style={label}>System Prompt</label>
          <textarea
            style={{ ...input, minHeight: 72, resize: 'vertical', fontFamily: 'inherit' }}
            placeholder="You are a helpful assistant."
            value={systemPrompt}
            onChange={e => setSystemPrompt(e.target.value)}
          />

          <button
            onClick={handleSave}
            disabled={saving}
            style={{
              background: 'var(--parley-accent,#32CD32)', border: 'none', color: '#fff',
              borderRadius: 4, padding: '7px 18px', fontWeight: 600, fontSize: 13, cursor: 'pointer',
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
          {saveMsg && <span style={{ marginLeft: 10, fontSize: 12, color: 'var(--parley-text-muted,#888)' }}>{saveMsg}</span>}

          {provider === 'parley' && usage && (
            <div style={{ marginTop: 20 }}>
              <div style={sectionTitle}>Monthly Usage</div>
              <div style={{ fontSize: 12, color: 'var(--parley-text-muted,#888)', marginBottom: 6 }}>
                {formatTokens(usage.tokens_used)} / {formatTokens(usage.tokens_limit)} tokens · Resets {resetDate}
              </div>
              <div style={{ background: 'var(--parley-bg,#111)', borderRadius: 4, height: 8, overflow: 'hidden' }}>
                <div style={{
                  height: '100%',
                  width: `${Math.min(100, (usage.tokens_used / usage.tokens_limit) * 100)}%`,
                  background: 'var(--parley-accent,#32CD32)',
                  borderRadius: 4,
                  transition: 'width .3s',
                }} />
              </div>
            </div>
          )}
        </>
      )}

      {isOwner && (
        <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid var(--parley-border,#333)' }}>
          <button
            onClick={onRemove}
            style={{ background: 'none', border: '1px solid var(--parley-danger,#f04747)', color: 'var(--parley-danger,#f04747)', borderRadius: 4, padding: '6px 14px', fontSize: 12, cursor: 'pointer' }}
          >
            Remove Bot
          </button>
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 5: Add Bots tab to ServerSettings.tsx**

Open `frontend/src/components/settings/ServerSettings.tsx`.

1. Add `'bots'` to the `Tab` type union: `type Tab = 'overview' | 'roles' | 'bots' | 'danger';`

2. Add import: `import { BotsTab } from './BotsTab';`

3. In the nav sidebar, add a button after the Roles button:
```tsx
<button className={`settings-nav-item${activeTab === 'bots' ? ' active' : ''}`} onClick={() => setActiveTab('bots')}>
  Bots
</button>
```

4. In the content area, add a render branch:
```tsx
{activeTab === 'bots' && (
  <BotsTab
    serverId={Number(server.id)}
    isOwner={hasPerm(myPerms, PERM_ADMINISTRATOR)}
  />
)}
```

`myPerms` and `hasPerm`/`PERM_ADMINISTRATOR` are already imported and used in `ServerSettings.tsx` for the roles tab. This correctly covers both the server owner (who gets all permissions via `ComputeBasePermissions`) and any user with the Administrator role.

- [ ] **Step 6: Build to verify**

```bash
cd frontend && npm run build 2>&1 | tail -20
```
Expected: no TypeScript errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/api/bots.ts frontend/src/components/settings/BotsTab.tsx frontend/src/components/settings/BotConfigPanel.tsx frontend/src/components/settings/BotsTab.css frontend/src/components/settings/ServerSettings.tsx
git commit -m "feat(bots): add BotsTab, BotConfigPanel, and API client"
```

---

### Task 9: Bot Invite Embed + Route

**Files:**
- Create: `frontend/src/components/BotInviteEmbed.tsx`
- Create: `frontend/src/pages/BotInvitePage.tsx`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Create BotInviteEmbed.tsx**

```tsx
// frontend/src/components/BotInviteEmbed.tsx
import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { BotInviteInfo, resolveBotInvite, acceptBotInvite } from '../api/bots';
import { EmbedCard } from './EmbedCard';

interface ServerOption {
  id: number;
  name: string;
}

interface Props {
  token: string;
}

export const BotInviteEmbed: React.FC<Props> = ({ token }) => {
  const navigate = useNavigate();
  const [bot, setBot] = useState<BotInviteInfo | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [selectedServer, setSelectedServer] = useState<number>(0);
  const [adding, setAdding] = useState(false);
  const [added, setAdded] = useState(false);
  const [error, setError] = useState('');

  const isLoggedIn = !!localStorage.getItem('token');

  useEffect(() => {
    resolveBotInvite(token)
      .then(setBot)
      .catch(() => setInvalid(true));
  }, [token]);

  useEffect(() => {
    if (!isLoggedIn) return;
    // Fetch user's servers (reuse existing endpoint)
    fetch('/api/servers', {
      headers: { Authorization: `Bearer ${localStorage.getItem('token')}` },
    })
      .then(r => r.json())
      .then((data: { id: number; name: string }[]) => {
        setServers(data);
        if (data.length) setSelectedServer(data[0].id);
      })
      .catch(() => {});
  }, [isLoggedIn]);

  if (invalid) {
    return (
      <EmbedCard title="Bot Not Found" actions={null}>
        <p style={{ fontSize: 13, color: 'var(--parley-text-muted,#888)' }}>This invite link is invalid or has expired.</p>
      </EmbedCard>
    );
  }

  if (!bot) {
    return <div style={{ textAlign: 'center', padding: 40, color: 'var(--parley-text-muted,#888)' }}>Loading…</div>;
  }

  const initial = bot.display_name.charAt(0).toUpperCase();

  const icon = bot.avatar_url
    ? <img src={bot.avatar_url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
    : initial;

  const handleAdd = async () => {
    if (!isLoggedIn) { navigate('/login'); return; }
    if (!selectedServer) return;
    setAdding(true);
    setError('');
    try {
      await acceptBotInvite(token, selectedServer);
      setAdded(true);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed to add bot';
      setError(msg.includes('409') ? 'Bot is already in that server.' : 'Failed to add bot.');
    } finally {
      setAdding(false);
    }
  };

  const serverSelector = !added && isLoggedIn && servers.length > 0 ? (
    <select
      value={selectedServer}
      onChange={e => setSelectedServer(Number(e.target.value))}
      style={{
        width: '100%', boxSizing: 'border-box',
        background: 'var(--parley-input,#1a1a1a)',
        border: '1px solid var(--parley-border,#444)',
        borderRadius: 4, color: 'var(--parley-text,#eee)',
        padding: '7px 10px', fontSize: 13,
      }}
    >
      {servers.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
    </select>
  ) : null;

  const actions = added ? (
    <div style={{ color: 'var(--parley-success,#43b581)', fontWeight: 600, fontSize: 14, textAlign: 'center' }}>
      ✓ Added to server!
    </div>
  ) : (
    <>
      {error && <div style={{ fontSize: 12, color: 'var(--parley-danger,#f04747)', marginBottom: 4 }}>{error}</div>}
      <button
        onClick={handleAdd}
        disabled={adding || (!isLoggedIn ? false : !selectedServer)}
        style={{
          background: 'var(--parley-accent,#32CD32)', border: 'none', color: '#fff',
          borderRadius: 4, padding: '9px', fontSize: 14, fontWeight: 600,
          cursor: 'pointer', width: '100%',
        }}
      >
        {adding ? 'Adding…' : isLoggedIn ? 'Add to Server' : 'Log in to Add'}
      </button>
    </>
  );

  return (
    <EmbedCard
      icon={icon}
      title={bot.display_name}
      subtitle={`@${bot.username} · AI Chatbot`}
      badge={bot.is_verified}
      children={serverSelector}
      actions={actions}
    />
  );
};
```

- [ ] **Step 2: Create BotInvitePage.tsx**

```tsx
// frontend/src/pages/BotInvitePage.tsx
import React from 'react';
import { useParams } from 'react-router-dom';
import { ThemeProvider } from '../context/ThemeContext';
import { BotInviteEmbed } from '../components/BotInviteEmbed';

export const BotInvitePage: React.FC = () => {
  const { token } = useParams<{ token: string }>();
  if (!token) return null;
  return (
    <ThemeProvider>
      <div style={{ minHeight: '100vh', background: 'var(--parley-channel-bg,#000)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
        <BotInviteEmbed token={token} />
      </div>
    </ThemeProvider>
  );
};
```

- [ ] **Step 3: Add route to App.tsx**

Open `frontend/src/App.tsx`. Add import:
```tsx
import { BotInvitePage } from './pages/BotInvitePage';
```

In the `<Routes>` block, add alongside the existing `/theme/:token` and `/themes` routes:
```tsx
<Route path="/bots/invite/:token" element={<BotInvitePage />} />
```

- [ ] **Step 4: Build to verify**

```bash
cd frontend && npm run build 2>&1 | tail -20
```
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/BotInviteEmbed.tsx frontend/src/pages/BotInvitePage.tsx frontend/src/App.tsx
git commit -m "feat(bots): add BotInviteEmbed and /bots/invite/:token route"
```

---

## Final Integration Check

- [ ] **Build entire backend**

```bash
cd /home/dylan/Developer/parley
go build ./...
```
Expected: no output.

- [ ] **Run Go tests**

```bash
go test ./internal/bots/... -v
```
Expected: encryption tests pass.

- [ ] **Build frontend**

```bash
cd frontend && npm run build 2>&1 | tail -5
```
Expected: `built in Xs`.

- [ ] **Final commit**

```bash
git add -A
git status  # verify nothing unexpected
git commit -m "feat(bots): complete Bots & AI Chatbot implementation"
```
