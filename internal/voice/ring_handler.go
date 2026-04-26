package voice

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"parley/internal/auth"
	"parley/internal/db"
	"parley/internal/httputil"
)

// ringRepo is the slice of repository methods the ring handler needs.
type ringRepo interface {
	IsDmMember(ctx context.Context, dmID, userID int64) (bool, error)
	GetDmChannelByID(ctx context.Context, dmID int64) (*db.DmChannel, error)
	GetDmMembers(ctx context.Context, dmID int64) ([]db.DmChannelMember, error)
	GetUserDmChannels(ctx context.Context, userID int64) ([]db.DmChannel, error)
	GetUserByID(ctx context.Context, id int64) (*db.User, error)
}

// CallStarter is the seam for emitting call_started from the GC start path.
// dm.Service satisfies this via DmEmitterFromService(...).Started.
type CallStarter interface {
	Started(ctx context.Context, channelID, actorUserID, startedAtMs int64) error
}

type RingHandler struct {
	rs      *RingService
	repo    ringRepo
	starter CallStarter // optional; set via SetCallStarter from cmd/api wiring
}

func NewRingHandler(rs *RingService, repo ringRepo) *RingHandler {
	return &RingHandler{rs: rs, repo: repo}
}

// SetCallStarter is invoked from cmd/api wiring to provide the dm emit adapter.
func (h *RingHandler) SetCallStarter(c CallStarter) { h.starter = c }

// resolveDm fetches the channel and resolves whether userID is a member, handling
// 1:1 DMs (membership lives in user1_id/user2_id) and group DMs (membership lives
// in dm_channel_members) uniformly. Mirrors the WS subscription auth path.
func (h *RingHandler) resolveDm(ctx context.Context, dmID, userID int64) (*db.DmChannel, bool, error) {
	dm, err := h.repo.GetDmChannelByID(ctx, dmID)
	if err != nil || dm == nil {
		return nil, false, err
	}
	if dm.IsGroup {
		isMember, err := h.repo.IsDmMember(ctx, dmID, userID)
		return dm, isMember, err
	}
	// 1:1 DMs store membership on user1_id/user2_id; the join table is empty.
	if dm.User1ID == userID || dm.User2ID == userID {
		return dm, true, nil
	}
	// Defensive fall-through: tolerate test/legacy data that uses the join table for 1:1.
	isMember, err := h.repo.IsDmMember(ctx, dmID, userID)
	return dm, isMember, err
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
	dm, isMember, err := h.resolveDm(r.Context(), dmID, userID)
	if err != nil || dm == nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	if dm.IsGroup {
		httputil.JSONError(w, "ringing is not supported for group DMs; use /call/start instead", http.StatusBadRequest)
		return
	}
	var targetID int64
	switch {
	case dm.User1ID == userID && dm.User2ID != 0:
		targetID = dm.User2ID
	case dm.User2ID == userID && dm.User1ID != 0:
		targetID = dm.User1ID
	default:
		// Fall back to the join-table view (legacy/test fixtures).
		members, err := h.repo.GetDmMembers(r.Context(), dmID)
		if err != nil || len(members) != 2 {
			httputil.JSONError(w, "invalid 1:1 channel", http.StatusBadRequest)
			return
		}
		for _, m := range members {
			if m.UserID != userID {
				targetID = m.UserID
				break
			}
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
	_, isMember, err := h.resolveDm(r.Context(), dmID, userID)
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
	if errors.Is(err, ErrCancelByNonCaller) {
		httputil.JSONError(w, "only the caller may cancel", http.StatusForbidden)
		return
	}
	if errors.Is(err, ErrRingNotFound) {
		httputil.JSONError(w, "ring not found", http.StatusNotFound)
		return
	}
	if err != nil {
		httputil.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Start emits call_started for a GC (no ring layer). 1:1 DMs should use /ring instead.
// POST /api/dm/{id}/call/start
func (h *RingHandler) Start(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	dmID, ok := dmIDFromPath(w, r)
	if !ok {
		return
	}
	dm, isMember, err := h.resolveDm(r.Context(), dmID, userID)
	if err != nil || dm == nil {
		httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !isMember {
		httputil.JSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	if !dm.IsGroup {
		httputil.JSONError(w, "use /call/ring for 1:1 DMs", http.StatusBadRequest)
		return
	}
	if h.starter == nil {
		log.Printf("ring handler: call starter not wired; cannot start call for dm:%d", dmID)
		httputil.JSONError(w, "call starter not configured", http.StatusInternalServerError)
		return
	}
	// Only emit the `call_started` system message when the room is empty —
	// joining an in-progress GC call shouldn't print "started a call." We
	// dedup via Redis SetNX on the per-channel started_at key (also set by
	// Join() on first joiner). Two concurrent /call/start requests both
	// trying to claim a fresh call: only the SetNX winner emits.
	startedAt := time.Now().UnixMilli()
	channelID := "dm:" + strconv.FormatInt(dmID, 10)
	fresh, err := h.rs.svc.ClaimCallStarted(r.Context(), channelID, startedAt)
	if err != nil {
		log.Printf("ring handler: ClaimCallStarted failed for %s: %v", channelID, err)
		// Fall through and emit anyway — over-emit beats silent loss.
		fresh = true
	}
	if fresh {
		if err := h.starter.Started(r.Context(), dmID, userID, startedAt); err != nil {
			httputil.JSONError(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Active returns rings targeting the current user plus per-DM live participant
// counts so the UI can rehydrate banners + phone icons after a page reload.
// GET /api/calls/active
func (h *RingHandler) Active(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	rings := h.rs.ActiveRingsForUser(userID)

	type inCallEntry struct {
		DmChannelID      string `json:"dm_channel_id"`
		ParticipantCount int    `json:"participant_count"`
	}
	inCall := []inCallEntry{}
	if dms, err := h.repo.GetUserDmChannels(r.Context(), userID); err == nil {
		for _, dm := range dms {
			vc := "dm:" + strconv.FormatInt(dm.ID, 10)
			parts, err := h.rs.svc.Participants(r.Context(), vc)
			if err != nil || len(parts) == 0 {
				continue
			}
			inCall = append(inCall, inCallEntry{
				DmChannelID:      strconv.FormatInt(dm.ID, 10),
				ParticipantCount: len(parts),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"rings":   rings,
		"in_call": inCall,
	})
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
