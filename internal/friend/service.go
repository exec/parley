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

// FriendNotifyFunc is called to fire a notification. actorID is threaded so
// the notification layer can suppress delivery when the recipient has blocked
// the actor.
type FriendNotifyFunc func(ctx context.Context, recipientID, actorID int64, username, avatarURL string)

// Service handles all friend business logic and DB access.
type Service struct {
	db             *sql.DB
	hub            *ws.Hub
	notifyRequest  FriendNotifyFunc
	notifyAccept   FriendNotifyFunc
}

// NewService creates a Service.
func NewService(repo *db.Repository, hub *ws.Hub) *Service {
	return &Service{db: repo.DB(), hub: hub}
}

// SetNotifyFriendRequest registers a callback for friend request notifications.
func (s *Service) SetNotifyFriendRequest(fn FriendNotifyFunc) { s.notifyRequest = fn }

// SetNotifyFriendAccept registers a callback for friend accept notifications.
func (s *Service) SetNotifyFriendAccept(fn FriendNotifyFunc) { s.notifyAccept = fn }

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
	ErrBlocked        = errors.New("blocked")
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

	// Reject if either side has blocked the other. Done inside the tx so a
	// concurrent block lands deterministically before the request insert.
	var blockExists int
	if err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM user_blocks
		WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)
		LIMIT 1
	`, senderID, receiverID).Scan(&blockExists); err == nil {
		// Don't disclose the direction. Treat both as a generic ErrUserNotFound
		// at the handler layer to avoid an enumeration oracle ("did this user
		// block me, or do they not exist?").
		return nil, ErrBlocked
	} else if err != sql.ErrNoRows {
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

	// Fire notification asynchronously
	if s.notifyRequest != nil {
		fn := s.notifyRequest
		go fn(ctx, receiverID, senderID, senderUsername, senderAvatar)
	}

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

	// Fire accept notification to the original sender
	if s.notifyAccept != nil {
		fn := s.notifyAccept
		go fn(ctx, senderID, receiverID, receiverUser.Username, receiverUser.AvatarURL)
	}

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

// IsBlocked returns true if blockerID has blocked blockedID. Direction-sensitive.
func (s *Service) IsBlocked(ctx context.Context, blockerID, blockedID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2 LIMIT 1`,
		blockerID, blockedID,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// IsEitherBlocked returns true if either party has blocked the other. Used by
// callers (DM, ring, notification) that don't care about direction — they just
// need to know whether to refuse the action.
func (s *Service) IsEitherBlocked(ctx context.Context, userA, userB int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM user_blocks
		WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)
		LIMIT 1
	`, userA, userB).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// Block creates a one-way block from actorID to targetID. Side effects:
//   - any accepted friendship between the two is removed
//   - any pending friend_requests between the two are deleted
//   - FRIEND_REMOVE is broadcast to the target so their UI drops the row
//
// Idempotent: re-blocking is a no-op (returns nil).
func (s *Service) Block(ctx context.Context, actorID, targetID int64) error {
	if actorID == targetID {
		return ErrSelf
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		actorID, targetID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM friend_requests
		WHERE (sender_id=$1 AND receiver_id=$2) OR (sender_id=$2 AND receiver_id=$1)
	`, actorID, targetID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Notify the target so their friends/requests UI drops the row. We don't
	// tell them they were blocked; the FRIEND_REMOVE event is the same one
	// fired on a normal unfriend, so the UX is indistinguishable.
	s.sendToUser(strconv.FormatInt(targetID, 10), ws.EventFriendRemove,
		map[string]string{"user_id": strconv.FormatInt(actorID, 10)})
	return nil
}

// Unblock removes the actorID → targetID block. Idempotent.
func (s *Service) Unblock(ctx context.Context, actorID, targetID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2`,
		actorID, targetID,
	)
	return err
}

// GetBlocks returns the users actorID has blocked.
func (s *Service) GetBlocks(ctx context.Context, actorID int64) ([]FriendUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, COALESCE(u.display_name,''), COALESCE(u.avatar_url,'')
		FROM user_blocks b
		JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1
		ORDER BY u.username
	`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []FriendUser{}
	for rows.Next() {
		var u FriendUser
		var id int64
		if err := rows.Scan(&id, &u.Username, &u.DisplayName, &u.AvatarURL); err != nil {
			return nil, err
		}
		u.ID = strconv.FormatInt(id, 10)
		out = append(out, u)
	}
	return out, rows.Err()
}
