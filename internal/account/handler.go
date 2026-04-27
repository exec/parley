package account

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/httputil"
)

// Handler exposes the account-deletion HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler bound to svc.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// deleteRequest is the JSON body for DELETE /api/me. The username confirmation
// is the only friction — bearer-token presence already proves session
// ownership; force-logout immediately after kills any stolen-token window.
type deleteRequest struct {
	ConfirmUsername string `json:"confirm_username"`
}

// blockersResponse is the 409 body shape locked in the spec at
// docs/superpowers/specs/2026-04-26-account-deletion-and-data-export-design.md.
// frontend-account consumes the field names as-is; do not rename without
// coordinating the change there.
type blockersResponse struct {
	Error            string        `json:"error"`
	BlockingServers  []BlockerInfo `json:"blocking_servers"`
	BlockingGroupDMs []BlockerInfo `json:"blocking_group_dms"`
}

// Delete handles DELETE /api/me. Status code mapping per spec:
//
//	204 — deletion completed
//	400 — confirm_username missing or didn't match
//	409 — owns servers and/or group DMs that still have other members
//	500 — anything unexpected
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}

	var body deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid_confirmation", http.StatusBadRequest)
		return
	}
	if body.ConfirmUsername == "" {
		httputil.JSONError(w, "invalid_confirmation", http.StatusBadRequest)
		return
	}

	err := h.svc.Delete(r.Context(), uid, body.ConfirmUsername)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
		return
	case errors.Is(err, ErrInvalidConfirmation):
		httputil.JSONError(w, "invalid_confirmation", http.StatusBadRequest)
		return
	case errors.Is(err, ErrHasBlockers):
		var be *BlockersError
		if !errors.As(err, &be) {
			// Shouldn't happen — Service only returns ErrHasBlockers wrapped
			// inside *BlockersError. Fall through to a generic 500 rather
			// than ship an empty 409 body that the client would mis-render.
			log.Printf("account.Delete: ErrHasBlockers without BlockersError payload (uid=%d)", uid)
			httputil.JSONError(w, "internal_server_error", http.StatusInternalServerError)
			return
		}
		writeBlockersResponse(w, be)
		return
	default:
		log.Printf("account.Delete: uid=%d err=%v", uid, err)
		httputil.JSONError(w, "internal_server_error", http.StatusInternalServerError)
		return
	}
}

func writeBlockersResponse(w http.ResponseWriter, be *BlockersError) {
	servers := be.Servers
	if servers == nil {
		servers = []BlockerInfo{}
	}
	groups := be.GroupDMs
	if groups == nil {
		groups = []BlockerInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(blockersResponse{
		Error:            "has_blockers",
		BlockingServers:  servers,
		BlockingGroupDMs: groups,
	})
}

// currentUser extracts the int64 user id from the request context populated
// by auth.AuthMiddleware. Mirrors the helper in internal/friend/handler.go.
func currentUser(r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	if s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}
