package voice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
)

// ringRepo is the slice of repository methods the ring handler needs.
type ringRepo interface {
	IsDmMember(ctx context.Context, dmID, userID int64) (bool, error)
	GetDmChannelByID(ctx context.Context, dmID int64) (*db.DmChannel, error)
	GetDmMembers(ctx context.Context, dmID int64) ([]db.DmChannelMember, error)
	GetUserByID(ctx context.Context, id int64) (*db.User, error)
}

type RingHandler struct {
	rs   *RingService
	repo ringRepo
}

func NewRingHandler(rs *RingService, repo ringRepo) *RingHandler {
	return &RingHandler{rs: rs, repo: repo}
}

// Ring initiates a 1:1 ring. Errors 400 for group DMs.
func (h *RingHandler) Ring(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	dmID, ok := dmIDFromPath(w, r)
	if !ok {
		return
	}
	isMember, err := h.repo.IsDmMember(r.Context(), dmID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	dm, err := h.repo.GetDmChannelByID(r.Context(), dmID)
	if errors.Is(err, db.ErrNotFound) || dm == nil {
		httputil.JSONError(w, "channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if dm.IsGroup {
		httputil.JSONError(w, "ringing is not supported for group DMs; use /call/start instead", http.StatusBadRequest)
		return
	}
	members, err := h.repo.GetDmMembers(r.Context(), dmID)
	if err != nil || len(members) != 2 {
		httputil.JSONError(w, "invalid 1:1 channel", http.StatusBadRequest)
		return
	}
	var targetID int64
	for _, m := range members {
		if m.UserID != userID {
			targetID = m.UserID
			break
		}
	}
	if targetID == 0 {
		httputil.JSONError(w, "target not found", http.StatusBadRequest)
		return
	}
	caller, _ := h.repo.GetUserByID(r.Context(), userID)
	info := ringCallerInfo{UserID: userID}
	if caller != nil {
		info.Username = caller.Username
		info.DisplayName = caller.DisplayName
		info.AvatarURL = caller.AvatarURL
	}
	id, err := h.rs.Initiate(r.Context(), dmID, userID, targetID, info)
	if err != nil {
		httputil.JSONError(w, "ring failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ring_id": id})
}

func (h *RingHandler) Accept(w http.ResponseWriter, r *http.Request)  { h.terminate(w, r, "accept") }
func (h *RingHandler) Decline(w http.ResponseWriter, r *http.Request) { h.terminate(w, r, "decline") }
func (h *RingHandler) Cancel(w http.ResponseWriter, r *http.Request)  { h.terminate(w, r, "cancel") }

func (h *RingHandler) terminate(w http.ResponseWriter, r *http.Request, op string) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	dmID, ok := dmIDFromPath(w, r)
	if !ok {
		return
	}
	isMember, err := h.repo.IsDmMember(r.Context(), dmID, userID)
	if err != nil || !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	var body struct {
		RingID string `json:"ring_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RingID == "" {
		httputil.JSONError(w, "ring_id required", http.StatusBadRequest)
		return
	}
	switch op {
	case "accept":
		err = h.rs.Accept(r.Context(), body.RingID, userID)
	case "decline":
		err = h.rs.Decline(r.Context(), body.RingID, userID)
	case "cancel":
		err = h.rs.Cancel(r.Context(), body.RingID, userID)
	default:
		err = errors.New("unknown op")
	}
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// userIDFromCtx extracts the authenticated user ID from the request context.
// Distinct from voice.Handler.userFromCtx (which is a method on *Handler).
func userIDFromCtx(w http.ResponseWriter, r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	return id, true
}

func dmIDFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	s := r.PathValue("id")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid dm id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}
