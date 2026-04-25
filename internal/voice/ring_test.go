package voice

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeHub struct {
	mu     sync.Mutex
	toUser []sentToUser
}
type sentToUser struct {
	userID    string
	eventType string
	payload   []byte
}

func (h *fakeHub) SendToUser(userID, eventType string, payload []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.toUser = append(h.toUser, sentToUser{userID, eventType, payload})
	return nil
}
func (h *fakeHub) sentTypes() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.toUser))
	for _, s := range h.toUser {
		out = append(out, s.eventType)
	}
	return out
}

type fakeDmEmitter struct {
	mu                                sync.Mutex
	started, ended, missed, declined int
}

func (e *fakeDmEmitter) Started(_ context.Context, _, _, _ int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.started++
	return nil
}
func (e *fakeDmEmitter) Ended(_ context.Context, _, _, _, _ int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ended++
	return nil
}
func (e *fakeDmEmitter) Missed(_ context.Context, _, _ int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.missed++
	return nil
}
func (e *fakeDmEmitter) Declined(_ context.Context, _, _, _ int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.declined++
	return nil
}

func newRingTestService() (*RingService, *fakeHub, *fakeDmEmitter) {
	hub := &fakeHub{}
	emit := &fakeDmEmitter{}
	rs := NewRingService(hub, emit, &Service{})
	rs.timeout = 50 * time.Millisecond // shorten for tests
	return rs, hub, emit
}

func TestInitiate_SendsRingAndStoresState(t *testing.T) {
	rs, hub, _ := newRingTestService()
	id, err := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty ring id")
	}
	rs.mu.Lock()
	_, ok1 := rs.rings[id]
	rid, ok2 := rs.byDM[10]
	rs.mu.Unlock()
	if !ok1 || !ok2 || rid != id {
		t.Fatalf("rings/byDM not populated correctly: ok1=%v ok2=%v rid=%q id=%q", ok1, ok2, rid, id)
	}

	// Wait briefly for the goroutine that sends the WS event.
	time.Sleep(5 * time.Millisecond)
	got := hub.sentTypes()
	found := false
	for _, ty := range got {
		if ty == "CALL_RING" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CALL_RING sent, got %v", got)
	}
}

func TestInitiate_GlareReturnsExistingRing(t *testing.T) {
	rs, _, _ := newRingTestService()
	id1, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})
	id2, err := rs.Initiate(context.Background(), 10, 2, 1, ringCallerInfo{}) // reverse glare
	if err != nil {
		t.Fatalf("glare must not error, got %v", err)
	}
	if id2 != id1 {
		t.Errorf("expected glare to surface existing ring %q, got %q", id1, id2)
	}
}
