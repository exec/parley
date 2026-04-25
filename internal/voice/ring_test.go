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
	gotMsg chan struct{}
}
type sentToUser struct {
	userID    string
	eventType string
	payload   []byte
}

func newFakeHub() *fakeHub {
	return &fakeHub{gotMsg: make(chan struct{}, 16)}
}

func (h *fakeHub) SendToUser(userID, eventType string, payload []byte) error {
	h.mu.Lock()
	h.toUser = append(h.toUser, sentToUser{userID, eventType, payload})
	h.mu.Unlock()
	select {
	case h.gotMsg <- struct{}{}:
	default:
	}
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
	hub := newFakeHub()
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

	select {
	case <-hub.gotMsg:
	case <-time.After(time.Second):
		t.Fatal("CALL_RING never sent")
	}
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

func TestInitiate_TimesOutAfterDuration(t *testing.T) {
	rs, hub, emit := newRingTestService()
	id, err := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})
	if err != nil {
		t.Fatal(err)
	}

	// timeout is 50ms; wait long enough for it to fire + both SendToUser goroutines to run.
	deadline := time.After(2 * time.Second)
	want := 3 // CALL_RING (initial) + CALL_TIMEOUT (caller) + CALL_TIMEOUT (target)
	for {
		hub.mu.Lock()
		count := len(hub.toUser)
		hub.mu.Unlock()
		if count >= want {
			break
		}
		select {
		case <-hub.gotMsg:
		case <-deadline:
			hub.mu.Lock()
			defer hub.mu.Unlock()
			t.Fatalf("only saw %d/%d events: %+v", count, want, hub.toUser)
		}
	}

	// Verify both CALL_TIMEOUT events landed (one per party).
	timeouts := 0
	for _, ty := range hub.sentTypes() {
		if ty == "CALL_TIMEOUT" {
			timeouts++
		}
	}
	if timeouts != 2 {
		t.Errorf("expected 2 CALL_TIMEOUT events (caller + target), got %d", timeouts)
	}

	// Ring should be removed from both maps.
	rs.mu.Lock()
	_, stillInRings := rs.rings[id]
	_, stillInByDM := rs.byDM[10]
	rs.mu.Unlock()
	if stillInRings || stillInByDM {
		t.Errorf("ring not cleaned up: rings=%v byDM=%v", stillInRings, stillInByDM)
	}

	// `call_missed` system event emitted.
	emit.mu.Lock()
	missed := emit.missed
	emit.mu.Unlock()
	if missed != 1 {
		t.Errorf("expected fakeDmEmitter.missed=1, got %d", missed)
	}
}
