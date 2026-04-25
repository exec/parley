package voice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	ws "parley/internal/websocket"
)

// RingHub is the small subset of *ws.Hub the ring service needs.
type RingHub interface {
	SendToUser(userID, eventType string, payload []byte) error
}

// DmEmitter is the seam for emitting call_* system messages.
// Mirrors dm.Service's EmitCall* signatures.
type DmEmitter interface {
	Started(ctx context.Context, channelID, actorUserID, startedAtMs int64) error
	Ended(ctx context.Context, channelID, lastLeaverUserID, durationMs, startedAtMs int64) error
	Missed(ctx context.Context, channelID, callerUserID int64) error
	Declined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error
}

// ringCallerInfo carries the display fields the receiver's UI shows on the ring.
type ringCallerInfo struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// Ring is one in-flight 1:1 ring.
type Ring struct {
	ID          string
	DmChannelID int64
	CallerID    int64
	TargetID    int64
	StartedAt   time.Time
	caller      ringCallerInfo
	timer       *time.Timer
}

// RingService owns 1:1 ring lifecycle. GC calls have no ring layer.
type RingService struct {
	mu      sync.Mutex
	rings   map[string]*Ring
	byDM    map[int64]string
	hub     RingHub
	dm      DmEmitter
	svc     *Service
	timeout time.Duration
}

func NewRingService(hub RingHub, dm DmEmitter, svc *Service) *RingService {
	return &RingService{
		rings:   make(map[string]*Ring),
		byDM:    make(map[int64]string),
		hub:     hub,
		dm:      dm,
		svc:     svc,
		timeout: 30 * time.Second,
	}
}

func newRingID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Initiate creates a ring (or returns the existing one for glare). Sends
// CALL_RING to the target's user-WS. Returns the ring ID.
func (rs *RingService) Initiate(ctx context.Context, dmChannelID, callerID, targetID int64, caller ringCallerInfo) (string, error) {
	rs.mu.Lock()
	if existing, ok := rs.byDM[dmChannelID]; ok {
		rs.mu.Unlock()
		return existing, nil // glare: surface existing ring to both parties
	}
	id := newRingID()
	r := &Ring{
		ID:          id,
		DmChannelID: dmChannelID,
		CallerID:    callerID,
		TargetID:    targetID,
		StartedAt:   time.Now(),
		caller:      caller,
	}
	r.timer = time.AfterFunc(rs.timeout, func() {
		_ = rs.timeoutRing(context.Background(), id)
	})
	rs.rings[id] = r
	rs.byDM[dmChannelID] = id
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"ring_id": id,
		"vc":      VirtualChannel{Kind: KindDM, ID: dmChannelID}.String(),
		"caller":  caller,
	})
	go func() {
		_ = rs.hub.SendToUser(strconv.FormatInt(targetID, 10), ws.EventCallRing, payload)
	}()
	return id, nil
}

// timeoutRing fires when no Accept/Decline/Cancel arrives within rs.timeout.
// Removes the ring, sends CALL_TIMEOUT to both parties, and emits call_missed.
func (rs *RingService) timeoutRing(ctx context.Context, ringID string) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return nil // already terminal
	}
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"ring_id": ringID})
	_ = rs.hub.SendToUser(strconv.FormatInt(r.CallerID, 10), ws.EventCallTimeout, payload)
	_ = rs.hub.SendToUser(strconv.FormatInt(r.TargetID, 10), ws.EventCallTimeout, payload)
	if rs.dm != nil {
		_ = rs.dm.Missed(ctx, r.DmChannelID, r.CallerID)
	}
	return nil
}

// Accept resolves a ring as accepted by the target. Caller and target both
// receive CALL_ACCEPT so other sessions of the target dismiss their modal.
func (rs *RingService) Accept(ctx context.Context, ringID string, accepterID int64) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring not found")
	}
	r.timer.Stop()
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"ring_id":          ringID,
		"accepter_user_id": strconv.FormatInt(accepterID, 10),
	})
	_ = rs.hub.SendToUser(strconv.FormatInt(r.CallerID, 10), ws.EventCallAccept, payload)
	_ = rs.hub.SendToUser(strconv.FormatInt(r.TargetID, 10), ws.EventCallAccept, payload)
	if rs.dm != nil {
		_ = rs.dm.Started(ctx, r.DmChannelID, accepterID, time.Now().UnixMilli())
	}
	return nil
}

// Decline resolves a ring as declined by the receiver. Caller is notified;
// no event is sent to the receiver (their modal closes locally on click).
func (rs *RingService) Decline(ctx context.Context, ringID string, declinerID int64) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring not found")
	}
	r.timer.Stop()
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"ring_id":          ringID,
		"decliner_user_id": strconv.FormatInt(declinerID, 10),
	})
	_ = rs.hub.SendToUser(strconv.FormatInt(r.CallerID, 10), ws.EventCallDecline, payload)
	if rs.dm != nil {
		_ = rs.dm.Declined(ctx, r.DmChannelID, r.CallerID, declinerID)
	}
	return nil
}

// Cancel resolves a ring as cancelled by the caller. Receiver sees
// CALL_CANCEL; system message is call_missed.
func (rs *RingService) Cancel(ctx context.Context, ringID string, callerID int64) error {
	rs.mu.Lock()
	r, ok := rs.rings[ringID]
	if !ok {
		rs.mu.Unlock()
		return errors.New("ring not found")
	}
	if r.CallerID != callerID {
		rs.mu.Unlock()
		return errors.New("only the caller may cancel")
	}
	r.timer.Stop()
	delete(rs.rings, ringID)
	delete(rs.byDM, r.DmChannelID)
	rs.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"ring_id": ringID})
	_ = rs.hub.SendToUser(strconv.FormatInt(r.TargetID, 10), ws.EventCallCancel, payload)
	if rs.dm != nil {
		_ = rs.dm.Missed(ctx, r.DmChannelID, callerID)
	}
	return nil
}

// ActiveRing is a serializable summary of a single in-flight ring.
type ActiveRing struct {
	RingID      string         `json:"ring_id"`
	VC          string         `json:"vc"`
	Caller      ringCallerInfo `json:"caller"`
	StartedAtMs int64          `json:"started_at_ms"`
}

// ActiveRingsForUser returns rings targeting the given user.
// Used by GET /api/calls/active for boot/reload rehydration (Task 13).
func (rs *RingService) ActiveRingsForUser(userID int64) []ActiveRing {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]ActiveRing, 0)
	for _, r := range rs.rings {
		if r.TargetID == userID {
			out = append(out, ActiveRing{
				RingID:      r.ID,
				VC:          VirtualChannel{Kind: KindDM, ID: r.DmChannelID}.String(),
				Caller:      r.caller,
				StartedAtMs: r.StartedAt.UnixMilli(),
			})
		}
	}
	return out
}
