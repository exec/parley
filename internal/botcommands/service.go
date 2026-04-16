package botcommands

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"parley/internal/db"
	"parley/internal/permissions"
)

// Sentinel errors returned by Service methods and mapped to HTTP statuses by Handler.
var (
	ErrBadRequest       = errors.New("bad request")
	ErrForbidden        = errors.New("forbidden")
	ErrCommandNotFound  = errors.New("command not found")
	ErrBotNotInServer   = errors.New("bot is not in this server")
	ErrInteractionState = errors.New("interaction not pending")
	ErrInteractionGone  = errors.New("interaction expired")
)

// MaxOptionsPerCommand caps the number of options per command.
const MaxOptionsPerCommand = 25

// EventInteractionCreate is the WS event name pushed to a bot when one of
// its commands is invoked. Duplicated from internal/websocket to avoid a
// package cycle (websocket doesn't import botcommands and vice versa).
const EventInteractionCreate = "INTERACTION_CREATE"

// MaxChoicesPerOption caps the number of choices per option.
const MaxChoicesPerOption = 25

// nameRegex matches the allowed characters in command/option names.
var nameRegex = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// allowedOptionTypes is the set of option types supported in Phase 1.
var allowedOptionTypes = map[string]struct{}{
	OptionTypeString:  {},
	OptionTypeInteger: {},
	OptionTypeBoolean: {},
}

// Notifier abstracts the WebSocket hub so the service can push INTERACTION_CREATE
// to the bot without importing the websocket package (avoids a cycle).
type Notifier interface {
	PublishToUser(userID int64, event string, payload interface{})
}

// MessageBroadcaster abstracts the per-channel MESSAGE_CREATE broadcast path.
// Matches the signature already used by message.MessageService so callers can
// pass the existing HubBroadcaster implementation.
type MessageBroadcaster interface {
	BroadcastToChannel(channelID string, event string, data interface{})
}

// Service holds business logic for slash commands.
type Service struct {
	repo       *Repository
	dbRepo     *db.Repository
	notifier   Notifier
	broadcast  MessageBroadcaster
	cache      *commandCache
}

// NewService constructs a Service. dbRepo is the shared repository used for
// permission lookups and channel/server metadata.
func NewService(repo *Repository, dbRepo *db.Repository) *Service {
	return &Service{
		repo:   repo,
		dbRepo: dbRepo,
		cache:  newCommandCache(),
	}
}

// SetNotifier wires the WS push side. Call before serving requests.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetBroadcaster wires the per-channel MESSAGE_CREATE fanout. Call before serving requests.
func (s *Service) SetBroadcaster(b MessageBroadcaster) { s.broadcast = b }

// ============ Command CRUD ============

// ListBotCommands returns the commands a bot has registered in the server.
// Caller is already authenticated as the bot (token prefix plk_); we just
// verify the bot is actually in that server.
func (s *Service) ListBotCommands(ctx context.Context, botID, serverID int64) ([]BotCommand, error) {
	ok, err := s.repo.IsBotInServer(ctx, serverID, botID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBotNotInServer
	}
	return s.repo.ListBotCommandsInServer(ctx, botID, serverID)
}

// ListServerCommands returns every command registered in a server — the list
// used by the user-facing autocomplete/palette. Caller must be a server member;
// we fail open here and rely on routes to enforce auth.
func (s *Service) ListServerCommands(ctx context.Context, serverID int64) ([]BotCommand, error) {
	if cached, ok := s.cache.Get(serverID); ok {
		return cached, nil
	}
	cmds, err := s.repo.ListServerCommands(ctx, serverID)
	if err != nil {
		return nil, err
	}
	s.cache.Set(serverID, cmds)
	return cmds, nil
}

// RegisterCommand upserts a single command.
func (s *Service) RegisterCommand(ctx context.Context, botID, serverID int64, req RegisterCommandRequest) (*BotCommand, error) {
	if err := ValidateCommand(req); err != nil {
		return nil, err
	}
	ok, err := s.repo.IsBotInServer(ctx, serverID, botID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBotNotInServer
	}
	cmd, err := s.repo.UpsertCommand(ctx, botID, serverID, req.Name, req.Description, req.Options)
	if err != nil {
		return nil, err
	}
	s.cache.Invalidate(serverID)
	return cmd, nil
}

// BulkReplace replaces the entire command list for a (bot, server) pair.
func (s *Service) BulkReplace(ctx context.Context, botID, serverID int64, reqs []RegisterCommandRequest) ([]BotCommand, error) {
	if len(reqs) > MaxOptionsPerCommand*4 { // sanity cap; no explicit spec
		return nil, fmt.Errorf("%w: too many commands in one replace (max 100)", ErrBadRequest)
	}
	seen := make(map[string]struct{}, len(reqs))
	for _, r := range reqs {
		if err := ValidateCommand(r); err != nil {
			return nil, err
		}
		if _, dup := seen[r.Name]; dup {
			return nil, fmt.Errorf("%w: duplicate command name %q", ErrBadRequest, r.Name)
		}
		seen[r.Name] = struct{}{}
	}
	ok, err := s.repo.IsBotInServer(ctx, serverID, botID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBotNotInServer
	}
	cmds, err := s.repo.BulkReplaceCommands(ctx, botID, serverID, reqs)
	if err != nil {
		return nil, err
	}
	s.cache.Invalidate(serverID)
	return cmds, nil
}

// DeleteCommand removes a single command by name.
func (s *Service) DeleteCommand(ctx context.Context, botID, serverID int64, name string) error {
	ok, err := s.repo.IsBotInServer(ctx, serverID, botID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrBotNotInServer
	}
	if err := s.repo.DeleteCommandByName(ctx, botID, serverID, name); err != nil {
		return err
	}
	s.cache.Invalidate(serverID)
	return nil
}

// ============ Invocation ============

// Invoke handles a user invoking a command. It:
//   - looks up the command,
//   - verifies the bot is still in the server,
//   - verifies the invoker has PermUseBotCommands in the channel,
//   - validates the options against the command's schema,
//   - inserts a bot_interactions row,
//   - pushes INTERACTION_CREATE to the bot's WS connection (best-effort).
func (s *Service) Invoke(ctx context.Context, invokerUserID, channelID, commandID int64, opts map[string]interface{}) (*BotInteraction, error) {
	cmd, err := s.repo.GetCommandByID(ctx, commandID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrCommandNotFound
		}
		return nil, err
	}

	// The channel must live in the command's server.
	ch, err := s.dbRepo.GetChannelByID(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	if ch.ServerID != cmd.ServerID {
		return nil, ErrCommandNotFound
	}

	// Bot must still be in the server.
	ok, err := s.repo.IsBotInServer(ctx, cmd.ServerID, cmd.BotID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBotNotInServer
	}

	// Permission check on the invoker.
	srv, err := s.dbRepo.GetServerByID(ctx, cmd.ServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	allowed, err := permissions.HasChannelPermission(ctx, s.dbRepo, cmd.ServerID, invokerUserID, srv.OwnerID, channelID, permissions.PermUseBotCommands)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}

	// Validate options against the command schema.
	normalized, err := ValidateOptions(cmd.Options, opts)
	if err != nil {
		return nil, err
	}

	token, err := newToken()
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	now := time.Now().UTC()
	interaction := &BotInteraction{
		Token:         token,
		BotID:         cmd.BotID,
		CommandID:     cmd.ID,
		InvokerUserID: invokerUserID,
		ChannelID:     channelID,
		ServerID:      cmd.ServerID,
		Options:       normalized,
		State:         StatePending,
		ExpiresAt:     now.Add(InteractionLifetime),
	}
	if err := s.repo.CreateInteraction(ctx, interaction); err != nil {
		return nil, err
	}

	// Push INTERACTION_CREATE (best-effort — if the bot isn't connected the
	// interaction will still be visible to it on reconnect via the pending
	// state, and will otherwise expire naturally).
	if s.notifier != nil {
		inv, err := s.repo.GetInvoker(ctx, invokerUserID)
		if err != nil {
			// Don't fail the whole invoke if invoker lookup hiccups — we have
			// a persisted interaction row and a non-identifying payload is
			// better than nothing, though in practice this should never fail.
			inv = InteractionInvoker{ID: invokerUserID}
		}
		payload := InteractionCreatePayload{
			Token: token,
			Command: InteractionCreateCommand{
				ID:      cmd.ID,
				Name:    cmd.Name,
				Options: cmd.Options,
			},
			Options:   normalized,
			Invoker:   inv,
			ChannelID: channelID,
			ServerID:  cmd.ServerID,
			CreatedAt: interaction.CreatedAt,
			ExpiresAt: interaction.ExpiresAt,
		}
		s.notifier.PublishToUser(cmd.BotID, EventInteractionCreate, payload)
	}

	return interaction, nil
}

// Respond handles POST /api/interactions/{token}/respond. The token has
// already been validated by middleware; this method re-checks state/expiry
// inside a happy-path guard and races safely on MarkInteractionResponded.
func (s *Service) Respond(ctx context.Context, token, content string) (int64, error) {
	content = trimContent(content)
	if content == "" {
		return 0, fmt.Errorf("%w: content required", ErrBadRequest)
	}
	if len(content) > 4000 {
		return 0, fmt.Errorf("%w: content too long", ErrBadRequest)
	}

	i, err := s.repo.GetInteractionByToken(ctx, token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, ErrCommandNotFound
		}
		return 0, err
	}
	if time.Now().UTC().After(i.ExpiresAt) {
		return 0, ErrInteractionGone
	}
	if i.State != StatePending {
		return 0, ErrInteractionState
	}

	msgID, _, err := s.repo.CreateInteractionResponseMessage(ctx, i.ChannelID, i.BotID, content)
	if err != nil {
		return 0, err
	}

	if err := s.repo.MarkInteractionResponded(ctx, token, msgID); err != nil {
		// Lost a race with expiry or a duplicate response. The message row is
		// already written; surface the conflict so the caller sees it.
		if errors.Is(err, ErrNotFound) {
			return 0, ErrInteractionState
		}
		return 0, err
	}

	// Broadcast MESSAGE_CREATE on the channel so connected clients see it.
	if s.broadcast != nil {
		broadcastPayload, err := s.buildBroadcastMessage(ctx, msgID, i)
		if err == nil {
			s.broadcast.BroadcastToChannel(channelIDString(i.ChannelID), "MESSAGE_CREATE", broadcastPayload)
		}
	}

	return msgID, nil
}

// GetInteractionByToken returns the interaction row for the given token, or
// ErrCommandNotFound if no such token exists. Used by the token-auth middleware.
func (s *Service) GetInteractionByToken(ctx context.Context, token string) (*BotInteraction, error) {
	i, err := s.repo.GetInteractionByToken(ctx, token)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrCommandNotFound
		}
		return nil, err
	}
	return i, nil
}

// buildBroadcastMessage constructs the MESSAGE_CREATE payload matching the
// shape produced by message.MessageService.SendMessage so the frontend can
// render it through the same code path.
func (s *Service) buildBroadcastMessage(ctx context.Context, msgID int64, i *BotInteraction) (map[string]interface{}, error) {
	var username, displayName, avatarURL string
	if err := s.dbRepo.DB().QueryRowContext(ctx,
		`SELECT username, COALESCE(display_name,''), COALESCE(avatar_url,'') FROM users WHERE id = $1`,
		i.BotID,
	).Scan(&username, &displayName, &avatarURL); err != nil {
		return nil, fmt.Errorf("look up bot user: %w", err)
	}

	dbMsg, err := s.dbRepo.GetMessageByID(ctx, msgID)
	if err != nil {
		return nil, fmt.Errorf("load response message: %w", err)
	}

	return map[string]interface{}{
		"id":                  fmt.Sprintf("%d", msgID),
		"channel_id":          channelIDString(i.ChannelID),
		"author_id":           fmt.Sprintf("%d", i.BotID),
		"author_username":     username,
		"author_display_name": displayName,
		"author_avatar_url":   avatarURL,
		"author_is_bot":       true,
		"via_api":             true,
		"kind":                MessageKindInteractionResponse,
		"content":             dbMsg.Content,
		"created_at":          dbMsg.CreatedAt,
		"updated_at":          dbMsg.UpdatedAt,
		"reactions":           []interface{}{},
	}, nil
}

// ============ Validation ============

// ValidateCommand enforces the Phase 1 shape rules on a registration request.
func ValidateCommand(req RegisterCommandRequest) error {
	if !nameRegex.MatchString(req.Name) {
		return fmt.Errorf("%w: command name must match %s", ErrBadRequest, nameRegex.String())
	}
	if l := len(req.Description); l < 1 || l > 100 {
		return fmt.Errorf("%w: description must be 1-100 chars", ErrBadRequest)
	}
	if len(req.Options) > MaxOptionsPerCommand {
		return fmt.Errorf("%w: at most %d options per command", ErrBadRequest, MaxOptionsPerCommand)
	}
	seen := make(map[string]struct{}, len(req.Options))
	for idx, opt := range req.Options {
		if err := validateOptionSchema(opt); err != nil {
			return fmt.Errorf("option %d (%q): %w", idx, opt.Name, err)
		}
		if _, dup := seen[opt.Name]; dup {
			return fmt.Errorf("%w: duplicate option name %q", ErrBadRequest, opt.Name)
		}
		seen[opt.Name] = struct{}{}
	}
	return nil
}

// validateOptionSchema enforces the shape of a single option definition.
func validateOptionSchema(opt BotCommandOption) error {
	if !nameRegex.MatchString(opt.Name) {
		return fmt.Errorf("%w: option name must match %s", ErrBadRequest, nameRegex.String())
	}
	if l := len(opt.Description); l < 1 || l > 100 {
		return fmt.Errorf("%w: option description must be 1-100 chars", ErrBadRequest)
	}
	if _, ok := allowedOptionTypes[opt.Type]; !ok {
		return fmt.Errorf("%w: option type must be STRING/INTEGER/BOOLEAN", ErrBadRequest)
	}

	// choices
	if len(opt.Choices) > MaxChoicesPerOption {
		return fmt.Errorf("%w: at most %d choices per option", ErrBadRequest, MaxChoicesPerOption)
	}
	if len(opt.Choices) > 0 && opt.Type == OptionTypeBoolean {
		return fmt.Errorf("%w: BOOLEAN options cannot have choices", ErrBadRequest)
	}
	for idx, ch := range opt.Choices {
		if l := len(ch.Name); l < 1 || l > 100 {
			return fmt.Errorf("%w: choice %d name must be 1-100 chars", ErrBadRequest, idx)
		}
		switch opt.Type {
		case OptionTypeString:
			if _, ok := ch.Value.(string); !ok {
				return fmt.Errorf("%w: choice %d value must be a string", ErrBadRequest, idx)
			}
		case OptionTypeInteger:
			if _, ok := toInt64(ch.Value); !ok {
				return fmt.Errorf("%w: choice %d value must be an integer", ErrBadRequest, idx)
			}
		}
	}

	// value/length bounds apply only to specific types
	if opt.Type != OptionTypeInteger && (opt.MinValue != nil || opt.MaxValue != nil) {
		return fmt.Errorf("%w: min_value/max_value only valid for INTEGER options", ErrBadRequest)
	}
	if opt.Type != OptionTypeString && (opt.MinLength != nil || opt.MaxLength != nil) {
		return fmt.Errorf("%w: min_length/max_length only valid for STRING options", ErrBadRequest)
	}
	if opt.MinValue != nil && opt.MaxValue != nil && *opt.MinValue > *opt.MaxValue {
		return fmt.Errorf("%w: min_value must be <= max_value", ErrBadRequest)
	}
	if opt.MinLength != nil {
		if *opt.MinLength < 0 || *opt.MinLength > 6000 {
			return fmt.Errorf("%w: min_length out of range", ErrBadRequest)
		}
	}
	if opt.MaxLength != nil {
		if *opt.MaxLength < 1 || *opt.MaxLength > 6000 {
			return fmt.Errorf("%w: max_length out of range", ErrBadRequest)
		}
	}
	if opt.MinLength != nil && opt.MaxLength != nil && *opt.MinLength > *opt.MaxLength {
		return fmt.Errorf("%w: min_length must be <= max_length", ErrBadRequest)
	}
	return nil
}

// ValidateOptions coerces and validates user-supplied option values against
// the command's schema. Returns the normalized map that should be persisted.
func ValidateOptions(schema []BotCommandOption, provided map[string]interface{}) (map[string]interface{}, error) {
	if provided == nil {
		provided = map[string]interface{}{}
	}
	byName := make(map[string]BotCommandOption, len(schema))
	for _, o := range schema {
		byName[o.Name] = o
	}

	// Reject unknown keys
	for k := range provided {
		if _, ok := byName[k]; !ok {
			return nil, fmt.Errorf("%w: unknown option %q", ErrBadRequest, k)
		}
	}

	out := make(map[string]interface{}, len(schema))
	for _, opt := range schema {
		raw, ok := provided[opt.Name]
		if !ok {
			if opt.Required {
				return nil, fmt.Errorf("%w: option %q is required", ErrBadRequest, opt.Name)
			}
			continue
		}
		val, err := coerceOption(opt, raw)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", opt.Name, err)
		}
		out[opt.Name] = val
	}
	return out, nil
}

// coerceOption coerces raw JSON input to the Go type expected for the option's
// declared type, validating range/length/choice constraints along the way.
func coerceOption(opt BotCommandOption, raw interface{}) (interface{}, error) {
	switch opt.Type {
	case OptionTypeString:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%w: expected string", ErrBadRequest)
		}
		if opt.MinLength != nil && len(s) < *opt.MinLength {
			return nil, fmt.Errorf("%w: min length %d", ErrBadRequest, *opt.MinLength)
		}
		if opt.MaxLength != nil && len(s) > *opt.MaxLength {
			return nil, fmt.Errorf("%w: max length %d", ErrBadRequest, *opt.MaxLength)
		}
		if len(opt.Choices) > 0 {
			if !matchesChoiceString(opt.Choices, s) {
				return nil, fmt.Errorf("%w: value not in allowed choices", ErrBadRequest)
			}
		}
		return s, nil

	case OptionTypeInteger:
		n, ok := toInt64(raw)
		if !ok {
			return nil, fmt.Errorf("%w: expected integer", ErrBadRequest)
		}
		if opt.MinValue != nil && float64(n) < *opt.MinValue {
			return nil, fmt.Errorf("%w: below min_value", ErrBadRequest)
		}
		if opt.MaxValue != nil && float64(n) > *opt.MaxValue {
			return nil, fmt.Errorf("%w: above max_value", ErrBadRequest)
		}
		if len(opt.Choices) > 0 {
			if !matchesChoiceInt(opt.Choices, n) {
				return nil, fmt.Errorf("%w: value not in allowed choices", ErrBadRequest)
			}
		}
		return n, nil

	case OptionTypeBoolean:
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: expected boolean", ErrBadRequest)
		}
		return b, nil
	}
	return nil, fmt.Errorf("%w: unknown option type", ErrBadRequest)
}

// matchesChoiceString reports whether s is listed among the string choices.
func matchesChoiceString(choices []OptionChoice, s string) bool {
	for _, c := range choices {
		if cs, ok := c.Value.(string); ok && cs == s {
			return true
		}
	}
	return false
}

// matchesChoiceInt reports whether n matches any integer choice.
func matchesChoiceInt(choices []OptionChoice, n int64) bool {
	for _, c := range choices {
		if cn, ok := toInt64(c.Value); ok && cn == n {
			return true
		}
	}
	return false
}

// toInt64 coerces common JSON-decoded numeric types (float64, json.Number, int,
// int64) to an int64. Returns ok=false if the value isn't an integer.
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case float64:
		// JSON numbers decode as float64; require no fractional component.
		if n != float64(int64(n)) {
			return 0, false
		}
		return int64(n), true
	case float32:
		if n != float32(int64(n)) {
			return 0, false
		}
		return int64(n), true
	}
	return 0, false
}

// ============ helpers ============

func channelIDString(id int64) string { return fmt.Sprintf("%d", id) }

// trimContent strips leading/trailing whitespace from response content.
func trimContent(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// newToken returns a 64-character hex token generated from 32 random bytes.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ============ cache ============

// commandCache is a tiny in-memory map keyed by server_id. Phase 1 is
// intentionally single-node; cross-node invalidation is a v2 todo.
type commandCache struct {
	mu sync.RWMutex
	m  map[int64][]BotCommand
}

func newCommandCache() *commandCache {
	return &commandCache{m: make(map[int64][]BotCommand)}
}

func (c *commandCache) Get(serverID int64) ([]BotCommand, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[serverID]
	if !ok {
		return nil, false
	}
	// Return a copy so callers mutating the slice don't poison the cache.
	out := make([]BotCommand, len(v))
	copy(out, v)
	return out, true
}

func (c *commandCache) Set(serverID int64, cmds []BotCommand) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stored := make([]BotCommand, len(cmds))
	copy(stored, cmds)
	c.m[serverID] = stored
}

func (c *commandCache) Invalidate(serverID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, serverID)
}
