# Friend System Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking. **Use code reviewers after each task.**

**Goal:** Add a full Discord-style friend system: send/accept/decline requests by username, friends list with online status, remove friend, open DM from friend, real-time WebSocket updates.

**Architecture:** Single `friend_requests` table (status: pending/accepted); a new `internal/friend` package handles all HTTP; WS events use `hub.SendToUser` to both parties on accept; frontend adds a FriendsView accessible via a button in the DmPanel header.

**Tech Stack:** PostgreSQL, Go 1.21 (chi v5, httputil.JSONError), React 18 + TypeScript, existing WebSocket hub.

---

## File Map

**Created:**
- `internal/friend/handler.go` — HTTP handlers for all friend endpoints
- `internal/friend/service.go` — Business logic + DB queries
- `frontend/src/api/friends.ts` — API client functions
- `frontend/src/components/layout/FriendsView.tsx` — Main friends UI
- `frontend/src/components/layout/FriendsView.css` — Styles

**Modified:**
- `internal/db/migrations.go` — Add migration 36 (friend_requests table)
- `internal/websocket/events.go` — Add FRIEND_REQUEST, FRIEND_ACCEPT, FRIEND_REMOVE constants
- `cmd/api/routes.go` — Wire friend routes
- `frontend/src/api/types.ts` — Add FriendUser, FriendRequest, FriendRequestsResponse
- `frontend/src/context/AppContext.tsx` — Add friends state + actions
- `frontend/src/hooks/useWebSocket.ts` — Add onFriendRequest/Accept/Remove callbacks
- `frontend/src/App.tsx` — Wire WS callbacks, pass friends props
- `frontend/src/components/layout/DmPanel.tsx` — Add Friends button + pending badge
- `frontend/src/components/layout/DmPanel.css` — Friends button styles

---

## Chunk 1: Backend

### Task 1: Database Migration

**Files:**
- Modify: `internal/db/migrations.go` (append to Migrations slice)

- [ ] **Step 1: Add migration 36 to the Migrations slice**

At the end of `internal/db/migrations.go`, before the closing `}` of the slice, append:

```go
	`-- Migration 36: friend requests
CREATE TABLE IF NOT EXISTS friend_requests (
    id          BIGSERIAL PRIMARY KEY,
    sender_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    receiver_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(sender_id, receiver_id)
);
CREATE INDEX IF NOT EXISTS idx_friend_requests_receiver ON friend_requests(receiver_id);
CREATE INDEX IF NOT EXISTS idx_friend_requests_sender   ON friend_requests(sender_id);
`,
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/dylan/Developer/parley && go build ./...
```
Expected: no output (clean build)

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations.go
git commit -m "feat: add friend_requests migration (migration 36)"
```

---

### Task 2: WebSocket Event Constants

**Files:**
- Modify: `internal/websocket/events.go` (add friend event constants)

- [ ] **Step 1: Add friend event constants to events.go**

Add a new section after the `// Bot events` block (after line 49):

```go
	// Friend events
	EventFriendRequest = "FRIEND_REQUEST"
	EventFriendAccept  = "FRIEND_ACCEPT"
	EventFriendRemove  = "FRIEND_REMOVE"
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```
Expected: clean

- [ ] **Step 3: Commit**

```bash
git add internal/websocket/events.go
git commit -m "feat: add friend WebSocket event constants"
```

---

### Task 3: Friend Backend Package

**Files:**
- Create: `internal/friend/service.go`
- Create: `internal/friend/handler.go`

- [ ] **Step 1: Create `internal/friend/service.go`**

```go
package friend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

// FriendUser is the public profile embedded in friend responses.
type FriendUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// FriendRequest is a friend request row with the other party's profile embedded.
type FriendRequest struct {
	ID         string     `json:"id"`
	SenderID   string     `json:"sender_id"`
	ReceiverID string     `json:"receiver_id"`
	Status     string     `json:"status"`
	User       FriendUser `json:"user"` // always the other party
	CreatedAt  string     `json:"created_at"`
}

// FriendRequestsResponse is the payload for GET /friend-requests.
type FriendRequestsResponse struct {
	Incoming []FriendRequest `json:"incoming"`
	Outgoing []FriendRequest `json:"outgoing"`
}

// Service handles all friend business logic and DB access.
type Service struct {
	db  *sql.DB
	hub *ws.Hub
}

// NewService creates a Service.
func NewService(repo *db.Repository, hub *ws.Hub) *Service {
	return &Service{db: repo.DB(), hub: hub}
}

// GetFriends returns all accepted friends for userID.
func (s *Service) GetFriends(ctx context.Context, userID int64) ([]FriendUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			CASE WHEN fr.sender_id = $1 THEN fr.receiver_id ELSE fr.sender_id END AS friend_id,
			u.username,
			COALESCE(u.display_name, '') AS display_name,
			COALESCE(u.avatar_url, '') AS avatar_url
		FROM friend_requests fr
		JOIN users u ON u.id = CASE WHEN fr.sender_id = $1 THEN fr.receiver_id ELSE fr.sender_id END
		WHERE (fr.sender_id = $1 OR fr.receiver_id = $1) AND fr.status = 'accepted'
		ORDER BY u.username
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []FriendUser
	for rows.Next() {
		var f FriendUser
		var fid int64
		if err := rows.Scan(&fid, &f.Username, &f.DisplayName, &f.AvatarURL); err != nil {
			return nil, err
		}
		f.ID = strconv.FormatInt(fid, 10)
		friends = append(friends, f)
	}
	if friends == nil {
		friends = []FriendUser{}
	}
	return friends, rows.Err()
}

// GetRequests returns pending incoming and outgoing requests for userID.
func (s *Service) GetRequests(ctx context.Context, userID int64) (*FriendRequestsResponse, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fr.id, fr.sender_id, fr.receiver_id, fr.status, fr.created_at,
		       u.username, COALESCE(u.display_name,'') AS display_name, COALESCE(u.avatar_url,'') AS avatar_url
		FROM friend_requests fr
		JOIN users u ON u.id = CASE WHEN fr.sender_id = $1 THEN fr.receiver_id ELSE fr.sender_id END
		WHERE (fr.sender_id = $1 OR fr.receiver_id = $1) AND fr.status = 'pending'
		ORDER BY fr.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resp := &FriendRequestsResponse{
		Incoming: []FriendRequest{},
		Outgoing: []FriendRequest{},
	}
	for rows.Next() {
		var req FriendRequest
		var (
			rid, sid, recid int64
			createdAt       time.Time
		)
		if err := rows.Scan(&rid, &sid, &recid, &req.Status, &createdAt,
			&req.User.Username, &req.User.DisplayName, &req.User.AvatarURL); err != nil {
			return nil, err
		}
		req.ID = strconv.FormatInt(rid, 10)
		req.SenderID = strconv.FormatInt(sid, 10)
		req.ReceiverID = strconv.FormatInt(recid, 10)
		req.CreatedAt = createdAt.Format(time.RFC3339)

		otherID := sid
		if sid == userID {
			otherID = recid
		}
		req.User.ID = strconv.FormatInt(otherID, 10)

		if recid == userID {
			resp.Incoming = append(resp.Incoming, req)
		} else {
			resp.Outgoing = append(resp.Outgoing, req)
		}
	}
	return resp, rows.Err()
}

var (
	ErrSelf           = errors.New("cannot send friend request to yourself")
	ErrAlreadyFriends = errors.New("already friends")
	ErrPending        = errors.New("friend request already pending")
	ErrNotFound       = errors.New("request not found")
	ErrForbidden      = errors.New("not your request")
	ErrUserNotFound   = errors.New("user not found")
)

// SendRequest creates a pending friend request from senderID to the user with the given username.
func (s *Service) SendRequest(ctx context.Context, senderID int64, username string) (*FriendRequest, error) {
	// Resolve username to user ID
	var receiverID int64
	var displayName, avatarURL sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(display_name,''), COALESCE(avatar_url,'') FROM users WHERE username = $1`, username,
	).Scan(&receiverID, &displayName, &avatarURL)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	if senderID == receiverID {
		return nil, ErrSelf
	}

	// Use a transaction with advisory locking to prevent bidirectional race.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Lock both user rows in deterministic order to avoid deadlocks
	lo, hi := senderID, receiverID
	if lo > hi {
		lo, hi = hi, lo
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, lo, hi); err != nil {
		return nil, err
	}

	// Check existing relationship in either direction
	var existingStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM friend_requests
		WHERE (sender_id=$1 AND receiver_id=$2) OR (sender_id=$2 AND receiver_id=$1)
	`, senderID, receiverID).Scan(&existingStatus)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if existingStatus == "accepted" {
		return nil, ErrAlreadyFriends
	}
	if existingStatus == "pending" {
		return nil, ErrPending
	}

	// Insert request
	var reqID int64
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO friend_requests (sender_id, receiver_id, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, created_at
	`, senderID, receiverID).Scan(&reqID, &createdAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Fetch sender profile for the WS payload
	var senderUsername, senderDisplay, senderAvatar string
	_ = s.db.QueryRowContext(ctx, `SELECT username, COALESCE(display_name,''), COALESCE(avatar_url,'') FROM users WHERE id=$1`, senderID).
		Scan(&senderUsername, &senderDisplay, &senderAvatar)

	req := &FriendRequest{
		ID:         strconv.FormatInt(reqID, 10),
		SenderID:   strconv.FormatInt(senderID, 10),
		ReceiverID: strconv.FormatInt(receiverID, 10),
		Status:     "pending",
		User: FriendUser{
			ID:          strconv.FormatInt(senderID, 10),
			Username:    senderUsername,
			DisplayName: senderDisplay,
			AvatarURL:   senderAvatar,
		},
		CreatedAt: createdAt.Format(time.RFC3339),
	}

	// Broadcast to receiver
	s.sendToUser(strconv.FormatInt(receiverID, 10), ws.EventFriendRequest, map[string]interface{}{"request": req})

	return req, nil
}

// AcceptRequest accepts a pending friend request. currentUserID must be the receiver.
func (s *Service) AcceptRequest(ctx context.Context, requestID, currentUserID int64) (*FriendUser, error) {
	var senderID, receiverID int64
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT sender_id, receiver_id, status FROM friend_requests WHERE id=$1`, requestID,
	).Scan(&senderID, &receiverID, &status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if receiverID != currentUserID {
		return nil, ErrForbidden
	}
	if status != "pending" {
		return nil, ErrNotFound
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE friend_requests SET status='accepted', updated_at=NOW() WHERE id=$1`, requestID,
	); err != nil {
		return nil, err
	}

	// Fetch both user profiles for WS payloads
	senderUser := s.fetchUser(ctx, senderID)
	receiverUser := s.fetchUser(ctx, receiverID)

	// Notify sender: they gained a new friend (receiverUser is the new friend)
	s.sendToUser(strconv.FormatInt(senderID, 10), ws.EventFriendAccept, map[string]interface{}{"user": receiverUser})
	// Notify receiver's other sessions (they already accepted, but other tabs need to update)
	s.sendToUser(strconv.FormatInt(receiverID, 10), ws.EventFriendAccept, map[string]interface{}{"user": senderUser})

	return senderUser, nil
}

// DeclineOrCancel deletes a pending request. Actor must be sender or receiver.
func (s *Service) DeclineOrCancel(ctx context.Context, requestID, actorID int64) error {
	var senderID, receiverID int64
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT sender_id, receiver_id, status FROM friend_requests WHERE id=$1`, requestID,
	).Scan(&senderID, &receiverID, &status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if senderID != actorID && receiverID != actorID {
		return ErrForbidden
	}
	if status != "pending" {
		// Accepted friendships must use RemoveFriend
		return ErrNotFound
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM friend_requests WHERE id=$1`, requestID)
	return err
}

// RemoveFriend deletes an accepted friendship between currentUserID and otherUserID.
func (s *Service) RemoveFriend(ctx context.Context, currentUserID, otherUserID int64) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM friend_requests
		WHERE ((sender_id=$1 AND receiver_id=$2) OR (sender_id=$2 AND receiver_id=$1))
		  AND status='accepted'
	`, currentUserID, otherUserID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	// Notify the other party
	s.sendToUser(strconv.FormatInt(otherUserID, 10), ws.EventFriendRemove,
		map[string]string{"user_id": strconv.FormatInt(currentUserID, 10)})
	return nil
}

// fetchUser returns a FriendUser for the given ID, logging on error.
func (s *Service) fetchUser(ctx context.Context, userID int64) *FriendUser {
	u := &FriendUser{ID: strconv.FormatInt(userID, 10)}
	_ = s.db.QueryRowContext(ctx,
		`SELECT username, COALESCE(display_name,''), COALESCE(avatar_url,'') FROM users WHERE id=$1`, userID,
	).Scan(&u.Username, &u.DisplayName, &u.AvatarURL)
	return u
}

// sendToUser marshals payload and delivers it via the WS hub.
func (s *Service) sendToUser(userID, event string, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("friend: marshal WS payload: %v", err)
		return
	}
	if s.hub != nil {
		if err := s.hub.SendToUser(userID, event, b); err != nil {
			log.Printf("friend: SendToUser %s: %v", userID, err)
		}
	}
}

// IsFriend returns true if the two users are accepted friends.
func (s *Service) IsFriend(ctx context.Context, userID1, userID2 int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM friend_requests
		WHERE ((sender_id=$1 AND receiver_id=$2) OR (sender_id=$2 AND receiver_id=$1))
		  AND status='accepted'
	`, userID1, userID2).Scan(&count)
	return count > 0, err
}

```

- [ ] **Step 2: Create `internal/friend/handler.go`**

```go
package friend

import (
	"encoding/json"
	"net/http"
	"strconv"

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
			httputil.JSONError(w, "user not found", http.StatusNotFound)
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
```

- [ ] **Step 3: Build to verify**

```bash
cd /home/dylan/Developer/parley && go build ./...
```
Expected: clean (no errors). If there are import errors, check that `parley/internal/friend` matches the module name in `go.mod`.

- [ ] **Step 4: Commit**

```bash
git add internal/friend/
git commit -m "feat: add friend system backend (handler + service)"
```

---

### Task 4: Wire Friend Routes

**Files:**
- Modify: `cmd/api/routes.go` (add friend routes inside protected group)

- [ ] **Step 1: Add the friend import to routes.go**

At the top of `cmd/api/routes.go`, in the import block, add:

```go
"parley/internal/friend"
```

(alongside the existing `"parley/internal/dm"` import)

- [ ] **Step 2: Add friend routes inside the protected `r.Group` block**

After the DM routes section (after line ~207, after `r.Post("/dms/{id}/messages/{messageId}/reactions", ...)`), add:

```go
			// Friend routes
			friendSvc := friend.NewService(repo, hub)
			friendHandler := friend.NewHandler(friendSvc)
			r.Get("/friends", friendHandler.GetFriends)
			r.Get("/friend-requests", friendHandler.GetRequests)
			r.Post("/friend-requests", friendHandler.SendRequest)
			r.Post("/friend-requests/{id}/accept", friendHandler.AcceptRequest)
			r.Delete("/friend-requests/{id}", friendHandler.DeclineOrCancel)
			r.Delete("/friends/{userId}", friendHandler.RemoveFriend)
```

- [ ] **Step 3: Build**

```bash
go build ./...
```
Expected: clean

- [ ] **Step 4: Commit**

```bash
git add cmd/api/routes.go
git commit -m "feat: wire friend routes into API"
```

---

## Chunk 2: Frontend

### Task 5: Frontend Types and API Client

**Files:**
- Modify: `frontend/src/api/types.ts` (add FriendUser, FriendRequest, FriendRequestsResponse)
- Create: `frontend/src/api/friends.ts`

- [ ] **Step 1: Add types to `frontend/src/api/types.ts`**

Append to the end of the file:

```typescript
export interface FriendUser {
  id: string;
  username: string;
  display_name?: string;
  avatar_url?: string;
}

export interface FriendRequest {
  id: string;
  sender_id: string;
  receiver_id: string;
  status: 'pending' | 'accepted';
  user: FriendUser; // always the other party
  created_at: string;
}

export interface FriendRequestsResponse {
  incoming: FriendRequest[];
  outgoing: FriendRequest[];
}
```

- [ ] **Step 2: Create `frontend/src/api/friends.ts`**

```typescript
import { apiClient } from './client';
import { FriendUser, FriendRequest, FriendRequestsResponse } from './types';

export async function getFriends(): Promise<FriendUser[]> {
  return apiClient.get<FriendUser[]>('/friends');
}

export async function getFriendRequests(): Promise<FriendRequestsResponse> {
  return apiClient.get<FriendRequestsResponse>('/friend-requests');
}

export async function sendFriendRequest(username: string): Promise<FriendRequest> {
  return apiClient.post<FriendRequest>('/friend-requests', { username });
}

export async function acceptFriendRequest(requestId: string): Promise<FriendUser> {
  return apiClient.post<FriendUser>(`/friend-requests/${requestId}/accept`);
}

export async function declineOrCancelRequest(requestId: string): Promise<void> {
  return apiClient.delete(`/friend-requests/${requestId}`);
}

export async function removeFriend(userId: string): Promise<void> {
  return apiClient.delete(`/friends/${userId}`);
}
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -30
```
Expected: no errors for these new files

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/types.ts frontend/src/api/friends.ts
git commit -m "feat: add friend system frontend types and API client"
```

---

### Task 6: AppContext State and Actions

**Files:**
- Modify: `frontend/src/context/AppContext.tsx`

**Context:** AppContext manages all app-wide state. The pattern for adding new state is:
1. Add to the `AppState` interface
2. Add to the `AppActions` interface
3. Add `useState` declarations
4. Add `useCallback` action implementations
5. Include in the `value` object passed to the Provider

- [ ] **Step 1: Add `* as friendsApi` to the imports at the top of AppContext.tsx**

In the import block (after `import * as dmsApi from '../api/dms';`), add:

```typescript
import * as friendsApi from '../api/friends';
```

Also add to the type imports line (after `DmMessage, Reaction`):
```typescript
import { User, Server, Channel, Message, ServerMember, DmChannel, DmMessage, Reaction, FriendUser, FriendRequest, FriendRequestsResponse } from '../api/types';
```

- [ ] **Step 2: Add friends state to the `AppState` interface**

After `isLoadingDms: boolean;`, add:

```typescript
  friends: FriendUser[];
  friendRequests: FriendRequestsResponse;
  pendingRequestCount: number;
```

- [ ] **Step 3: Add friend actions to the `AppActions` interface**

After `loadMoreDmMessages: () => Promise<void>;`, add:

```typescript
  loadFriends: () => Promise<void>;
  sendFriendRequest: (username: string) => Promise<void>;
  acceptFriendRequest: (requestId: string) => Promise<void>;
  declineOrCancelRequest: (requestId: string) => Promise<void>;
  removeFriend: (userId: string) => Promise<void>;
  receiveFriendRequest: (req: FriendRequest) => void;
  receiveFriendAccept: (user: FriendUser) => void;
  receiveFriendRemove: (userId: string) => void;
```

- [ ] **Step 4: Add useState declarations**

After the `const [isLoadingDms, setIsLoadingDms] = useState(false);` line, add:

```typescript
  const [friends, setFriends] = useState<FriendUser[]>([]);
  const [friendRequests, setFriendRequests] = useState<FriendRequestsResponse>({ incoming: [], outgoing: [] });
```

- [ ] **Step 5: Load friends on login (alongside servers/DMs)**

In the existing `useEffect` that loads servers/DMs on login (the one that depends on `currentUser?.id`), add friend loading:

Find the block:
```typescript
    dmsApi.getDmChannels()
      .then(data => setDmChannels(data ?? []))
      .catch(console.error);
```

After it, add:
```typescript
    friendsApi.getFriends()
      .then(data => setFriends(data ?? []))
      .catch(console.error);
    friendsApi.getFriendRequests()
      .then(data => setFriendRequests(data ?? { incoming: [], outgoing: [] }))
      .catch(console.error);
```

- [ ] **Step 6: Add friend action callbacks**

After the `loadMoreDmMessages` callback, add:

```typescript
  const loadFriends = useCallback(async () => {
    try {
      const [f, reqs] = await Promise.all([
        friendsApi.getFriends(),
        friendsApi.getFriendRequests(),
      ]);
      setFriends(f ?? []);
      setFriendRequests(reqs ?? { incoming: [], outgoing: [] });
    } catch (err) {
      console.error('loadFriends:', err);
    }
  }, []);

  const sendFriendRequest = useCallback(async (username: string) => {
    const req = await friendsApi.sendFriendRequest(username);
    setFriendRequests(prev => ({ ...prev, outgoing: [...prev.outgoing, req] }));
  }, []);

  const acceptFriendRequest = useCallback(async (requestId: string) => {
    const newFriend = await friendsApi.acceptFriendRequest(requestId);
    setFriendRequests(prev => ({
      ...prev,
      incoming: prev.incoming.filter(r => r.id !== requestId),
    }));
    setFriends(prev => {
      if (prev.some(f => f.id === newFriend.id)) return prev;
      return [...prev, newFriend];
    });
  }, []);

  const declineOrCancelRequest = useCallback(async (requestId: string) => {
    await friendsApi.declineOrCancelRequest(requestId);
    setFriendRequests(prev => ({
      incoming: prev.incoming.filter(r => r.id !== requestId),
      outgoing: prev.outgoing.filter(r => r.id !== requestId),
    }));
  }, []);

  const removeFriend = useCallback(async (userId: string) => {
    await friendsApi.removeFriend(userId);
    setFriends(prev => prev.filter(f => f.id !== userId));
  }, []);

  // WS event handlers
  const receiveFriendRequest = useCallback((req: FriendRequest) => {
    setFriendRequests(prev => {
      if (prev.incoming.some(r => r.id === req.id)) return prev;
      return { ...prev, incoming: [req, ...prev.incoming] };
    });
  }, []);

  const receiveFriendAccept = useCallback((user: FriendUser) => {
    // Add to friends
    setFriends(prev => {
      if (prev.some(f => f.id === user.id)) return prev;
      return [...prev, user];
    });
    // Remove from both incoming and outgoing (handles all-session cases)
    setFriendRequests(prev => ({
      incoming: prev.incoming.filter(r => r.user.id !== user.id),
      outgoing: prev.outgoing.filter(r => r.user.id !== user.id),
    }));
  }, []);

  const receiveFriendRemove = useCallback((userId: string) => {
    setFriends(prev => prev.filter(f => f.id !== userId));
  }, []);
```

- [ ] **Step 7: Expose `pendingRequestCount` as a derived value**

Add this line after the useState declarations (before the useEffects):

```typescript
  const pendingRequestCount = friendRequests.incoming.length;
```

- [ ] **Step 8: Add all new state and actions to the Provider value object**

In the `<AppContext.Provider value={{...}}>` block, add after `loadMoreDmMessages`:

```typescript
      friends,
      friendRequests,
      pendingRequestCount,
      loadFriends,
      sendFriendRequest,
      acceptFriendRequest,
      declineOrCancelRequest,
      removeFriend,
      receiveFriendRequest,
      receiveFriendAccept,
      receiveFriendRemove,
```

- [ ] **Step 9: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -30
```
Expected: no new errors

- [ ] **Step 10: Commit**

```bash
git add frontend/src/context/AppContext.tsx
git commit -m "feat: add friends state and actions to AppContext"
```

---

### Task 7: useWebSocket Friend Event Callbacks

**Files:**
- Modify: `frontend/src/hooks/useWebSocket.ts`

**Context:** `useWebSocket.ts` uses a `useLatest` helper (recently added) that reduces ref+effect boilerplate to a single line per callback. All callbacks follow the same pattern. The hook's `UseWebSocketOptions` interface declares the callback signatures, and `ws.onmessage` dispatches to them.

- [ ] **Step 1: Add callback types to the `UseWebSocketOptions` interface**

After `onDmReactionUpdate?:` (around line 109), add:

```typescript
  onFriendRequest?: (req: import('../api/types').FriendRequest) => void;
  onFriendAccept?: (user: import('../api/types').FriendUser) => void;
  onFriendRemove?: (userId: string) => void;
```

Or, add `FriendUser, FriendRequest` to the existing type imports at line 2:

```typescript
import { Message, DmMessage, Channel, Server, Role, FriendUser, FriendRequest } from '../api/types';
```

Then in the interface:
```typescript
  onFriendRequest?: (req: FriendRequest) => void;
  onFriendAccept?: (user: FriendUser) => void;
  onFriendRemove?: (userId: string) => void;
```

- [ ] **Step 2: Add the parameters to the destructured function signature**

In the `export function useWebSocket({...})` signature (line ~115), add after `onDmReactionUpdate`:
```typescript
onFriendRequest, onFriendAccept, onFriendRemove,
```

- [ ] **Step 3: Add useLatest refs (after the existing onDmReactionUpdateRef line)**

```typescript
  const onFriendRequestRef = useLatest(onFriendRequest);
  const onFriendAcceptRef = useLatest(onFriendAccept);
  const onFriendRemoveRef = useLatest(onFriendRemove);
```

- [ ] **Step 4: Add dispatch cases to `ws.onmessage`**

After the `dm_reaction_add/remove` block (around line 379), add:

```typescript
        } else if (wsMsg.type === 'FRIEND_REQUEST' && onFriendRequestRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { request: FriendRequest };
            if (p.request) onFriendRequestRef.current(p.request);
          }
        } else if (wsMsg.type === 'FRIEND_ACCEPT' && onFriendAcceptRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { user: FriendUser };
            if (p.user) onFriendAcceptRef.current(p.user);
          }
        } else if (wsMsg.type === 'FRIEND_REMOVE' && onFriendRemoveRef.current) {
          if (typeof wsMsg.payload === 'object' && wsMsg.payload !== null) {
            const p = wsMsg.payload as { user_id: string };
            if (p.user_id) onFriendRemoveRef.current(p.user_id);
          }
        }
```

- [ ] **Step 5: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -30
```
Expected: clean

- [ ] **Step 6: Commit**

```bash
git add frontend/src/hooks/useWebSocket.ts
git commit -m "feat: add friend WebSocket event handlers to useWebSocket"
```

---

### Task 8: FriendsView Component

**Files:**
- Create: `frontend/src/components/layout/FriendsView.tsx`
- Create: `frontend/src/components/layout/FriendsView.css`

**Context:** This component shows three tabs: All Friends, Pending (with badge), Add Friend. It receives friends, requests, online status, and action callbacks as props. It does NOT use AppContext directly — receives everything as props to stay testable.

- [ ] **Step 1: Create `frontend/src/components/layout/FriendsView.css`**

```css
.friends-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-primary, #313338);
  color: var(--text-primary, #dcddde);
}

.friends-view-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 20px 0;
  border-bottom: 1px solid var(--bg-tertiary, #1e1f22);
}

.friends-tab {
  padding: 8px 12px;
  border-radius: 4px 4px 0 0;
  border: none;
  background: none;
  color: var(--text-muted, #96989d);
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 6px;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
}

.friends-tab:hover {
  background: var(--bg-modifier-hover, #3f4147);
  color: var(--text-primary, #dcddde);
}

.friends-tab.active {
  color: var(--text-primary, #dcddde);
  border-bottom-color: var(--brand, #5865f2);
}

.friends-tab-badge {
  background: var(--status-danger, #ed4245);
  color: #fff;
  border-radius: 10px;
  padding: 0 5px;
  font-size: 11px;
  font-weight: 700;
  min-width: 16px;
  text-align: center;
}

.friends-view-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
}

.friends-section-label {
  text-transform: uppercase;
  font-size: 11px;
  font-weight: 700;
  color: var(--text-muted, #96989d);
  margin-bottom: 8px;
  margin-top: 16px;
}

.friends-section-label:first-child {
  margin-top: 0;
}

.friends-list-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 8px;
  border-radius: 8px;
  cursor: default;
}

.friends-list-item:hover {
  background: var(--bg-modifier-hover, #3f4147);
}

.friends-avatar-wrap {
  position: relative;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
}

.friends-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--bg-tertiary, #1e1f22);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
}

.friends-status-dot {
  position: absolute;
  bottom: -1px;
  right: -1px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid var(--bg-primary, #313338);
  background: var(--status-offline, #80848e);
}

.friends-status-dot.online {
  background: var(--status-online, #23a55a);
}

.friends-name {
  flex: 1;
  font-size: 15px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.friends-actions {
  display: flex;
  gap: 6px;
  opacity: 0;
}

.friends-list-item:hover .friends-actions {
  opacity: 1;
}

.friends-btn {
  padding: 4px 12px;
  border-radius: 4px;
  border: none;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
}

.friends-btn.primary {
  background: var(--brand, #5865f2);
  color: #fff;
}

.friends-btn.primary:hover {
  background: var(--brand-hover, #4752c4);
}

.friends-btn.danger {
  background: transparent;
  color: var(--status-danger, #ed4245);
  border: 1px solid var(--status-danger, #ed4245);
}

.friends-btn.danger:hover {
  background: var(--status-danger, #ed4245);
  color: #fff;
}

.friends-btn.ghost {
  background: var(--bg-secondary, #2b2d31);
  color: var(--text-primary, #dcddde);
}

.friends-btn.ghost:hover {
  background: var(--bg-modifier-hover, #3f4147);
}

.friends-empty {
  text-align: center;
  color: var(--text-muted, #96989d);
  padding: 40px 0;
  font-size: 14px;
}

.add-friend-form {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.add-friend-input {
  flex: 1;
  padding: 10px 14px;
  background: var(--bg-tertiary, #1e1f22);
  border: 1px solid var(--bg-tertiary, #1e1f22);
  border-radius: 6px;
  color: var(--text-primary, #dcddde);
  font-size: 14px;
  outline: none;
}

.add-friend-input:focus {
  border-color: var(--brand, #5865f2);
}

.add-friend-feedback {
  margin-top: 8px;
  font-size: 13px;
  min-height: 18px;
}

.add-friend-feedback.success {
  color: var(--status-online, #23a55a);
}

.add-friend-feedback.error {
  color: var(--status-danger, #ed4245);
}
```

- [ ] **Step 2: Create `frontend/src/components/layout/FriendsView.tsx`**

```tsx
import React, { useState } from 'react';
import { FriendUser, FriendRequest, FriendRequestsResponse } from '../../api/types';
import './FriendsView.css';

type Tab = 'all' | 'pending' | 'add';

interface FriendsViewProps {
  friends: FriendUser[];
  friendRequests: FriendRequestsResponse;
  onlineUserIds: Set<string>;
  currentUserId: string;
  onMessage: (userId: string) => void;
  onAccept: (requestId: string) => Promise<void>;
  onDeclineOrCancel: (requestId: string) => Promise<void>;
  onRemove: (userId: string) => Promise<void>;
  onSendRequest: (username: string) => Promise<void>;
}

const FriendAvatar: React.FC<{ user: FriendUser; online: boolean }> = ({ user, online }) => (
  <div className="friends-avatar-wrap">
    <div className="friends-avatar">
      {user.avatar_url
        ? <img src={user.avatar_url} alt={user.username} style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: '50%' }} />
        : (user.display_name || user.username).charAt(0).toUpperCase()}
    </div>
    <span className={`friends-status-dot ${online ? 'online' : ''}`} />
  </div>
);

const FriendsView: React.FC<FriendsViewProps> = ({
  friends,
  friendRequests,
  onlineUserIds,
  onMessage,
  onAccept,
  onDeclineOrCancel,
  onRemove,
  onSendRequest,
}) => {
  const [tab, setTab] = useState<Tab>('all');
  const [addUsername, setAddUsername] = useState('');
  const [addFeedback, setAddFeedback] = useState<{ msg: string; ok: boolean } | null>(null);
  const [addLoading, setAddLoading] = useState(false);

  const pendingCount = friendRequests.incoming.length;

  const handleSendRequest = async () => {
    if (!addUsername.trim()) return;
    setAddLoading(true);
    setAddFeedback(null);
    try {
      await onSendRequest(addUsername.trim());
      setAddFeedback({ msg: `Friend request sent to ${addUsername}!`, ok: true });
      setAddUsername('');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to send request';
      setAddFeedback({ msg, ok: false });
    } finally {
      setAddLoading(false);
    }
  };

  const sortedFriends = [...friends].sort((a, b) => {
    const aOnline = onlineUserIds.has(a.id);
    const bOnline = onlineUserIds.has(b.id);
    if (aOnline !== bOnline) return aOnline ? -1 : 1;
    return (a.display_name || a.username).localeCompare(b.display_name || b.username);
  });

  return (
    <div className="friends-view">
      <div className="friends-view-header">
        <button className={`friends-tab ${tab === 'all' ? 'active' : ''}`} onClick={() => setTab('all')}>
          All Friends
        </button>
        <button className={`friends-tab ${tab === 'pending' ? 'active' : ''}`} onClick={() => setTab('pending')}>
          Pending
          {pendingCount > 0 && <span className="friends-tab-badge">{pendingCount}</span>}
        </button>
        <button className={`friends-tab ${tab === 'add' ? 'active' : ''}`} onClick={() => setTab('add')}>
          Add Friend
        </button>
      </div>

      <div className="friends-view-body">
        {tab === 'all' && (
          <>
            {sortedFriends.length === 0 ? (
              <div className="friends-empty">No friends yet. Add some using the Add Friend tab!</div>
            ) : (
              <>
                <div className="friends-section-label">All Friends — {sortedFriends.length}</div>
                {sortedFriends.map(f => (
                  <div key={f.id} className="friends-list-item">
                    <FriendAvatar user={f} online={onlineUserIds.has(f.id)} />
                    <span className="friends-name">{f.display_name || f.username}</span>
                    <div className="friends-actions">
                      <button className="friends-btn primary" onClick={() => onMessage(f.id)}>Message</button>
                      <button className="friends-btn danger" onClick={() => onRemove(f.id)}>Remove</button>
                    </div>
                  </div>
                ))}
              </>
            )}
          </>
        )}

        {tab === 'pending' && (
          <>
            {friendRequests.incoming.length === 0 && friendRequests.outgoing.length === 0 && (
              <div className="friends-empty">No pending friend requests.</div>
            )}

            {friendRequests.incoming.length > 0 && (
              <>
                <div className="friends-section-label">Incoming — {friendRequests.incoming.length}</div>
                {friendRequests.incoming.map(req => (
                  <div key={req.id} className="friends-list-item">
                    <FriendAvatar user={req.user} online={onlineUserIds.has(req.user.id)} />
                    <span className="friends-name">{req.user.display_name || req.user.username}</span>
                    <div className="friends-actions" style={{ opacity: 1 }}>
                      <button className="friends-btn primary" onClick={() => onAccept(req.id)}>Accept</button>
                      <button className="friends-btn danger" onClick={() => onDeclineOrCancel(req.id)}>Decline</button>
                    </div>
                  </div>
                ))}
              </>
            )}

            {friendRequests.outgoing.length > 0 && (
              <>
                <div className="friends-section-label">Outgoing — {friendRequests.outgoing.length}</div>
                {friendRequests.outgoing.map(req => (
                  <div key={req.id} className="friends-list-item">
                    <FriendAvatar user={req.user} online={onlineUserIds.has(req.user.id)} />
                    <span className="friends-name">{req.user.display_name || req.user.username}</span>
                    <div className="friends-actions" style={{ opacity: 1 }}>
                      <button className="friends-btn ghost" onClick={() => onDeclineOrCancel(req.id)}>Cancel</button>
                    </div>
                  </div>
                ))}
              </>
            )}
          </>
        )}

        {tab === 'add' && (
          <>
            <div className="friends-section-label">Add a Friend</div>
            <p style={{ fontSize: 14, color: 'var(--text-muted, #96989d)', marginBottom: 12 }}>
              Enter their exact username to send a friend request.
            </p>
            <div className="add-friend-form">
              <input
                className="add-friend-input"
                type="text"
                placeholder="Enter a username"
                value={addUsername}
                onChange={e => setAddUsername(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSendRequest()}
                autoFocus
              />
              <button
                className="friends-btn primary"
                onClick={handleSendRequest}
                disabled={addLoading || !addUsername.trim()}
              >
                {addLoading ? '...' : 'Send Request'}
              </button>
            </div>
            {addFeedback && (
              <div className={`add-friend-feedback ${addFeedback.ok ? 'success' : 'error'}`}>
                {addFeedback.msg}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
};

export default FriendsView;
```

- [ ] **Step 3: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -30
```
Expected: clean

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/layout/FriendsView.tsx frontend/src/components/layout/FriendsView.css
git commit -m "feat: add FriendsView component (All / Pending / Add Friend tabs)"
```

---

### Task 9: DmPanel Friends Button

**Files:**
- Modify: `frontend/src/components/layout/DmPanel.tsx`
- Modify: `frontend/src/components/layout/DmPanel.css`

**Context:** DmPanel shows the left sidebar in the home/DM view. It has a header ("Direct Messages"), a list, and a user area at the bottom. We're adding a "Friends" nav item to the header with a pending badge.

- [ ] **Step 1: Add `onOpenFriends` and `pendingRequestCount` to `DmPanelProps`**

Update the interface:

```typescript
interface DmPanelProps {
  dmChannels: DmChannel[];
  activeDmChannelId: string | null;
  currentUser?: User | null;
  onSelectDm: (channelId: string) => void;
  onLogout?: () => void;
  onOpenSettings?: () => void;
  dmUnreadCounts?: Record<string, number>;
  onlineUserIds?: Set<string>;
  onOpenFriends?: () => void;       // new
  pendingRequestCount?: number;     // new
  isFriendsActive?: boolean;        // new — highlights Friends button when active
}
```

- [ ] **Step 2: Destructure the new props**

Update the function signature:

```typescript
const DmPanel: React.FC<DmPanelProps> = ({
  dmChannels,
  activeDmChannelId,
  currentUser,
  onSelectDm,
  onLogout,
  onOpenSettings,
  dmUnreadCounts = {},
  onlineUserIds,
  onOpenFriends,
  pendingRequestCount = 0,
  isFriendsActive = false,
}) => {
```

- [ ] **Step 3: Add the Friends button to the header**

Replace the existing header block:

```tsx
      <div className="dm-panel-header">
        <span className="dm-panel-title">Direct Messages</span>
      </div>
```

With:

```tsx
      <div className="dm-panel-header">
        <button
          className={`dm-friends-btn ${isFriendsActive ? 'active' : ''}`}
          onClick={onOpenFriends}
          title="Friends"
        >
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
            <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/>
          </svg>
          Friends
          {pendingRequestCount > 0 && (
            <span className="dm-friends-badge">{pendingRequestCount > 99 ? '99+' : pendingRequestCount}</span>
          )}
        </button>
        <span className="dm-panel-title">Direct Messages</span>
      </div>
```

- [ ] **Step 4: Add styles to DmPanel.css**

Append to `frontend/src/components/layout/DmPanel.css`:

```css
.dm-friends-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 8px 10px;
  background: none;
  border: none;
  border-radius: 6px;
  color: var(--text-muted, #96989d);
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  text-align: left;
  margin-bottom: 4px;
}

.dm-friends-btn:hover {
  background: var(--bg-modifier-hover, #3f4147);
  color: var(--text-primary, #dcddde);
}

.dm-friends-btn.active {
  background: var(--bg-modifier-selected, #404249);
  color: var(--text-primary, #dcddde);
}

.dm-friends-badge {
  background: var(--status-danger, #ed4245);
  color: #fff;
  border-radius: 10px;
  padding: 0 5px;
  font-size: 11px;
  font-weight: 700;
  min-width: 16px;
  text-align: center;
  margin-left: auto;
}
```

- [ ] **Step 5: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -30
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/layout/DmPanel.tsx frontend/src/components/layout/DmPanel.css
git commit -m "feat: add Friends button with pending badge to DmPanel"
```

---

### Task 10: App.tsx Wiring

**Files:**
- Modify: `frontend/src/App.tsx`

**Context:** App.tsx uses `useApp()` to get context state/actions, and passes `useWebSocket` the callbacks. It renders either a server view or the DM panel. We need to:
1. Add `activeFriendsView` state
2. Pass WS friend callbacks
3. Show FriendsView in the main area when friends is active
4. Pass `onOpenFriends`, `pendingRequestCount`, `isFriendsActive` to DmPanel

Read `frontend/src/App.tsx` to find the exact insertion points before editing.

- [ ] **Step 1: Read App.tsx to understand current structure**

```bash
head -100 /home/dylan/Developer/parley/frontend/src/App.tsx
```

- [ ] **Step 2: Add import for FriendsView**

Near the top of App.tsx where components are imported, add:
```typescript
import FriendsView from './components/layout/FriendsView';
```

- [ ] **Step 3: Import friend-related context values**

In the `useApp()` destructuring, add:
```typescript
const {
  // ... existing destructuring ...
  friends,
  friendRequests,
  pendingRequestCount,
  loadFriends,
  sendFriendRequest,
  acceptFriendRequest,
  declineOrCancelRequest,
  removeFriend,
  receiveFriendRequest,
  receiveFriendAccept,
  receiveFriendRemove,
} = useApp();
```

- [ ] **Step 4: Add `activeFriendsView` state**

Near the other `useState` calls at the top of the component:
```typescript
const [activeFriendsView, setActiveFriendsView] = useState(false);
```

- [ ] **Step 5: Pass friend WS callbacks to `useWebSocket`**

In the `useWebSocket({...})` call, add:
```typescript
    onFriendRequest: receiveFriendRequest,
    onFriendAccept: receiveFriendAccept,
    onFriendRemove: receiveFriendRemove,
```

- [ ] **Step 6: Wire `openDmChannel` to close friends view**

Find where `openDmChannel` is called (or defined) and ensure that opening a DM closes the friends view. Look for `selectDmChannel` or `openDmChannel` usage. After calling the DM open action, add:
```typescript
setActiveFriendsView(false);
```

Alternatively, when `setActiveFriendsView(true)` is called, also call `selectServer('__none__')` to deselect any active server.

The `openFriends` handler:
```typescript
const handleOpenFriends = () => {
  setActiveFriendsView(true);
  selectServer('__none__'); // clears activeServer, activeChannel, and activeDmChannel inside AppContext
};
```

`selectServer('__none__')` already clears all active state (AppContext lines ~156-163). Do NOT call `setActiveDmChannel` — it is private to AppContext and not accessible in App.tsx.

- [ ] **Step 7: Handle "Message" click in FriendsView**

```typescript
const handleFriendMessage = async (userId: string) => {
  await openDmChannel(userId);
  setActiveFriendsView(false);
};
```

Where `openDmChannel` is the action from `useApp()`.

- [ ] **Step 8: Pass new props to DmPanel**

Find the `<DmPanel ...>` usage and add:
```typescript
  onOpenFriends={handleOpenFriends}
  pendingRequestCount={pendingRequestCount}
  isFriendsActive={activeFriendsView}
```

- [ ] **Step 9: Render FriendsView in the main content area**

Find the `mainContent` let-variable assignment block in App.tsx. It starts with something like:
```typescript
if (view === 'server') {
  mainContent = ...
} else if (view === 'homepage') {
  mainContent = ...
}
```

Add `activeFriendsView` as the **first branch** of this if-else chain:

```typescript
if (activeFriendsView) {
  mainContent = (
    <FriendsView
      friends={friends}
      friendRequests={friendRequests}
      onlineUserIds={onlineUsers}
      currentUserId={currentUser?.id ?? ''}
      onMessage={handleFriendMessage}
      onAccept={acceptFriendRequest}
      onDeclineOrCancel={declineOrCancelRequest}
      onRemove={removeFriend}
      onSendRequest={sendFriendRequest}
    />
  );
} else if (view === 'server') {
  // existing code unchanged
```

This ensures FriendsView takes priority when active.

- [ ] **Step 9b: Reset `activeFriendsView` when navigating to server or DM**

Add a `useEffect` near the other effects at the top of the component:

```typescript
useEffect(() => {
  if (activeServer || activeDmChannel) {
    setActiveFriendsView(false);
  }
}, [activeServer, activeDmChannel]);
```

This ensures clicking a server or DM from the sidebar automatically closes the friends view. Without this, `activeFriendsView` stays true and FriendsView renders even while viewing a server.

- [ ] **Step 10: TypeScript check**

```bash
cd /home/dylan/Developer/parley/frontend && npx tsc --noEmit 2>&1 | head -50
```
Fix any type errors. Common ones: `openDmChannel` may not be directly accessible (it may go through `selectDmChannel` — check AppContext for the correct action name). Use `openDmChannel` for new DMs with non-existing channels; it's defined in AppContext.

- [ ] **Step 11: Build the frontend**

```bash
cd /home/dylan/Developer/parley/frontend && npm run build 2>&1 | tail -20
```
Expected: successful build

- [ ] **Step 12: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "feat: wire FriendsView into App — friends button, WS events, DM navigation"
```

---

## Chunk 3: Integration and Push

### Task 11: Integration Smoke Test

- [ ] **Step 1: Start the backend**

```bash
cd /home/dylan/Developer/parley && go run ./cmd/api/ &
```

- [ ] **Step 2: Start the frontend**

```bash
cd /home/dylan/Developer/parley/frontend && npm run dev &
```

- [ ] **Step 3: Open http://localhost:5173 and verify**

- Log in as User A → click Friends → see empty friends list
- Click "Add Friend" tab → type User B's username → Send Request
- API call to POST /friend-requests should succeed (200 or 201)
- Log in as User B (different browser/incognito) → click Friends → Pending tab shows incoming request
- Click Accept → both users see each other in All Friends
- Click Message on a friend → opens DM, friends view closes
- On User A's side, Remove a friend → User A's friends list updates, User B loses User A from their list (WS)

- [ ] **Step 4: Kill dev servers**

```bash
kill %1 %2 2>/dev/null; true
```

- [ ] **Step 5: Final full build check**

```bash
cd /home/dylan/Developer/parley && go build ./... && cd frontend && npm run build 2>&1 | tail -5
```

- [ ] **Step 6: Commit if any fixes were needed**

```bash
git add -A && git commit -m "fix: integration smoke test fixes for friend system" --allow-empty
```

---

### Task 12: Push to GitHub

- [ ] **Step 1: Push main branch**

```bash
git push origin main
```
