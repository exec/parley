package friend

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"parley/internal/auth"
	"parley/internal/httputil"
)

// Handler handles HTTP requests for the friend system.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func currentUser(r *http.Request) (int64, bool) {
	s := auth.GetUserIDFromContext(r)
	if s == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil
}

// GetFriends handles GET /friends
func (h *Handler) GetFriends(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	friends, err := h.svc.GetFriends(r.Context(), uid)
	if err != nil {
		httputil.JSONError(w, "failed to get friends", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(friends) //nolint:errcheck
}

// GetRequests handles GET /friend-requests
func (h *Handler) GetRequests(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	resp, err := h.svc.GetRequests(r.Context(), uid)
	if err != nil {
		httputil.JSONError(w, "failed to get requests", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// SendRequest handles POST /friend-requests
func (h *Handler) SendRequest(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	var body struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" {
		httputil.JSONError(w, "username is required", http.StatusBadRequest)
		return
	}

	req, err := h.svc.SendRequest(r.Context(), uid, body.Username)
	if err != nil {
		switch err {
		case ErrSelf:
			httputil.JSONError(w, "cannot send friend request to yourself", http.StatusBadRequest)
		case ErrAlreadyFriends:
			httputil.JSONError(w, "already friends", http.StatusBadRequest)
		case ErrPending:
			httputil.JSONError(w, "friend request already pending", http.StatusBadRequest)
		case ErrUserNotFound:
			// Don't disclose whether the username exists. Combined with the
			// per-user rate limit, this kills username enumeration via friend
			// requests. Return a synthetic-looking pending request so the
			// client UX is identical to a real send; on refresh it disappears
			// because nothing was persisted.
			log.Printf("friend.SendRequest: target username does not exist (sender=%d)", uid)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"id":          "0",
				"sender_id":   strconv.FormatInt(uid, 10),
				"receiver_id": "0",
				"status":      "pending",
				"user":        map[string]any{"id": "0", "username": body.Username},
				"created_at":  time.Now().UTC().Format(time.RFC3339),
			})
			return
		default:
			httputil.JSONError(w, "failed to send request", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req) //nolint:errcheck
}

// AcceptRequest handles POST /friend-requests/{id}/accept
func (h *Handler) AcceptRequest(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	reqID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid request id", http.StatusBadRequest)
		return
	}

	newFriend, err := h.svc.AcceptRequest(r.Context(), reqID, uid)
	if err != nil {
		switch err {
		case ErrNotFound:
			httputil.JSONError(w, "request not found", http.StatusNotFound)
		case ErrForbidden:
			httputil.JSONError(w, "not your request", http.StatusForbidden)
		default:
			httputil.JSONError(w, "failed to accept request", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newFriend) //nolint:errcheck
}

// DeclineOrCancel handles DELETE /friend-requests/{id}
func (h *Handler) DeclineOrCancel(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	reqID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid request id", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeclineOrCancel(r.Context(), reqID, uid); err != nil {
		switch err {
		case ErrNotFound:
			httputil.JSONError(w, "request not found", http.StatusNotFound)
		case ErrForbidden:
			httputil.JSONError(w, "not your request", http.StatusForbidden)
		default:
			httputil.JSONError(w, "failed to process request", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveFriend handles DELETE /friends/{userId}
func (h *Handler) RemoveFriend(w http.ResponseWriter, r *http.Request) {
	uid, ok := currentUser(r)
	if !ok {
		httputil.JSONError(w, "user not authenticated", http.StatusUnauthorized)
		return
	}
	otherID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		httputil.JSONError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	if err := h.svc.RemoveFriend(r.Context(), uid, otherID); err != nil {
		switch err {
		case ErrNotFound:
			httputil.JSONError(w, "not friends with this user", http.StatusNotFound)
		default:
			httputil.JSONError(w, "failed to remove friend", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
