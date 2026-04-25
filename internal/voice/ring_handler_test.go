package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"parley/internal/auth"
)

func withFakeUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, auth.UserIDKey, strconv.FormatInt(id, 10))
}

func TestRingHandler_Initiate_OneOnOne(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/ring", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Ring(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		RingID string `json:"ring_id"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.RingID == "" {
		t.Error("ring_id missing")
	}
}

func TestRingHandler_Initiate_RejectsGC(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true, 3: true}},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/ring", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Ring(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for GC, got %d", rec.Code)
	}
}

func TestRingHandler_Initiate_RejectsNonMember(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/ring", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 99))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Ring(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRingHandler_Accept(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	body, _ := json.Marshal(map[string]string{"ring_id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/accept", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Accept(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestRingHandler_Decline(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	body, _ := json.Marshal(map[string]string{"ring_id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/decline", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Decline(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestRingHandler_Cancel_HappyPath(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	body, _ := json.Marshal(map[string]string{"ring_id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/cancel", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 1)) // caller cancels
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Cancel(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRingHandler_Cancel_NotCaller_Returns403(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	id, _ := rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{})

	body, _ := json.Marshal(map[string]string{"ring_id": id})
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/cancel", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 2)) // NON-caller (target) attempts cancel
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Cancel(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-caller cancel, got %d", rec.Code)
	}
}

func TestRingHandler_Accept_MissingBody_Returns400(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/accept", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Accept(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing body, got %d", rec.Code)
	}
}

func TestRingHandler_Accept_EmptyRingID_Returns400(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	body, _ := json.Marshal(map[string]string{"ring_id": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/accept", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Accept(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty ring_id, got %d", rec.Code)
	}
}

type fakeCallStarter struct {
	mu    sync.Mutex
	count int
}

func (f *fakeCallStarter) Started(_ context.Context, _, _, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count++
	return nil
}

func TestStart_GC_EmitsStarted(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true, 3: true}},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)
	starter := &fakeCallStarter{}
	h.SetCallStarter(starter)

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/start", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Start(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d body=%s", rec.Code, rec.Body.String())
	}
	starter.mu.Lock()
	count := starter.count
	starter.mu.Unlock()
	if count != 1 {
		t.Errorf("expected starter called once, got %d", count)
	}
}

func TestStart_RejectsOneOnOne(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)
	h.SetCallStarter(&fakeCallStarter{})

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/start", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 1))
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Start(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for 1:1, got %d", rec.Code)
	}
}

func TestStart_RejectsNonMember(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	req := httptest.NewRequest(http.MethodPost, "/api/dm/10/call/start", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 99)) // non-member
	req.SetPathValue("id", "10")
	rec := httptest.NewRecorder()
	h.Start(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestActive_ReturnsRingsForUser(t *testing.T) {
	repo := &authRepoFake{}
	rs := NewRingService(newFakeHub(), &fakeDmEmitter{}, &Service{})
	h := NewRingHandler(rs, repo)

	rs.Initiate(context.Background(), 10, 1, 2, ringCallerInfo{Username: "alice"})
	rs.Initiate(context.Background(), 11, 3, 2, ringCallerInfo{Username: "charlie"})
	rs.Initiate(context.Background(), 12, 1, 4, ringCallerInfo{Username: "alice"})

	req := httptest.NewRequest(http.MethodGet, "/api/calls/active", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 2))
	rec := httptest.NewRecorder()
	h.Active(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp struct {
		Rings  []ActiveRing `json:"rings"`
		InCall []any        `json:"in_call"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Rings) != 2 {
		t.Errorf("expected 2 rings for user 2, got %d", len(resp.Rings))
	}
	// in_call should be present and empty (not nil) — JSON `[]` not `null`
	if resp.InCall == nil {
		t.Error("in_call should be empty array, not null")
	}
}
