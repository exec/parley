package botcommands

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"parley/internal/auth"
	"parley/internal/httputil"
)

// Handler wires the Service into HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler bound to svc.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Service exposes the underlying service for middleware (token validation).
func (h *Handler) Service() *Service { return h.svc }

// ============ Context keys ============

type ctxKey string

const (
	ctxInteractionKey ctxKey = "botcommands.interaction"
)

// InteractionFromContext returns the interaction the caller authenticated as,
// if any. Set by InteractionTokenAuth.
func InteractionFromContext(ctx context.Context) (*BotInteraction, bool) {
	v, ok := ctx.Value(ctxInteractionKey).(*BotInteraction)
	return v, ok
}

// ============ Middleware ============

// InteractionTokenAuth resolves the {token} URL param against bot_interactions,
// rejecting expired or already-responded tokens. On success the handler can
// pull the interaction via InteractionFromContext.
func (h *Handler) InteractionTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if token == "" || len(token) > 128 {
			httputil.JSONError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		i, err := h.svc.GetInteractionByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, ErrCommandNotFound) {
				httputil.JSONError(w, "unknown interaction token", http.StatusUnauthorized)
				return
			}
			httputil.InternalError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), ctxInteractionKey, i)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ============ Bot-authenticated command CRUD ============

// ListMyCommands handles GET /api/bots/@me/servers/{serverID}/commands.
func (h *Handler) ListMyCommands(w http.ResponseWriter, r *http.Request) {
	botID, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	serverID, ok := pathInt64(r, "id")
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	cmds, err := h.svc.ListBotCommands(r.Context(), botID, serverID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	render.JSON(w, r, cmds)
}

// BulkReplaceMyCommands handles PUT /api/bots/@me/servers/{serverID}/commands.
// Body is a JSON array of command definitions. Replaces the entire set.
func (h *Handler) BulkReplaceMyCommands(w http.ResponseWriter, r *http.Request) {
	botID, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	serverID, ok := pathInt64(r, "id")
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	var body []RegisterCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cmds, err := h.svc.BulkReplace(r.Context(), botID, serverID, body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	render.JSON(w, r, cmds)
}

// UpsertMyCommand handles POST /api/bots/@me/servers/{serverID}/commands.
// Body is a single command definition.
func (h *Handler) UpsertMyCommand(w http.ResponseWriter, r *http.Request) {
	botID, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	serverID, ok := pathInt64(r, "id")
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	var body RegisterCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cmd, err := h.svc.RegisterCommand(r.Context(), botID, serverID, body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(cmd)
}

// DeleteMyCommand handles DELETE /api/bots/@me/servers/{serverID}/commands/{name}.
func (h *Handler) DeleteMyCommand(w http.ResponseWriter, r *http.Request) {
	botID, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	serverID, ok := pathInt64(r, "id")
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	name := chi.URLParam(r, "name")
	if !nameRegex.MatchString(name) {
		httputil.JSONError(w, "invalid command name", http.StatusBadRequest)
		return
	}
	if err := h.svc.DeleteCommand(r.Context(), botID, serverID, name); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ============ User-authenticated ============

// ListServerCommands handles GET /api/servers/{serverID}/commands.
func (h *Handler) ListServerCommands(w http.ResponseWriter, r *http.Request) {
	if _, ok := callerID(r); !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	serverID, ok := pathInt64(r, "id")
	if !ok {
		httputil.JSONError(w, "invalid server id", http.StatusBadRequest)
		return
	}
	cmds, err := h.svc.ListServerCommands(r.Context(), serverID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	render.JSON(w, r, cmds)
}

// Invoke handles POST /api/channels/{channelID}/interactions.
func (h *Handler) Invoke(w http.ResponseWriter, r *http.Request) {
	userID, ok := callerID(r)
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	channelID, ok := pathInt64(r, "channelID")
	if !ok {
		httputil.JSONError(w, "invalid channel id", http.StatusBadRequest)
		return
	}
	var body InvokeInteractionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.CommandID == 0 {
		httputil.JSONError(w, "command_id is required", http.StatusBadRequest)
		return
	}
	interaction, err := h.svc.Invoke(r.Context(), userID, channelID, body.CommandID, body.Options)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(InvokeInteractionResponse{
		InteractionID: interaction.Token,
		Status:        interaction.State,
		ExpiresAt:     interaction.ExpiresAt,
	})
}

// Respond handles POST /api/interactions/{token}/respond. Assumes
// InteractionTokenAuth ran before it and placed the interaction on the context.
func (h *Handler) Respond(w http.ResponseWriter, r *http.Request) {
	i, ok := InteractionFromContext(r.Context())
	if !ok {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body RespondRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	msgID, err := h.svc.Respond(r.Context(), i.Token, body.Content)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	render.JSON(w, r, RespondResponse{MessageID: msgID})
}

// ============ helpers ============

// callerID extracts the authenticated user ID (or bot user ID, in the case of
// api-key authenticated bot routes) from the request context.
func callerID(r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	if s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// pathInt64 parses a URL param as int64.
func pathInt64(r *http.Request, key string) (int64, bool) {
	v := chi.URLParam(r, key)
	if v == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// writeServiceError maps sentinel errors to HTTP statuses.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadRequest):
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrForbidden):
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrBotNotInServer):
		httputil.JSONError(w, "bot is not in this server", http.StatusForbidden)
	case errors.Is(err, ErrCommandNotFound):
		httputil.JSONError(w, "command not found", http.StatusNotFound)
	case errors.Is(err, ErrInteractionGone):
		httputil.JSONError(w, "interaction expired", http.StatusGone)
	case errors.Is(err, ErrInteractionState):
		httputil.JSONError(w, "interaction is not pending", http.StatusConflict)
	default:
		httputil.InternalError(w, err)
	}
}
