package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActivityStart_RequiresParticipation(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	hub := newFakeHub()
	h := NewActivityHandler(svc, hub)

	body, _ := json.Marshal(map[string]any{"type": "watch_party", "params": map[string]any{"url": "x"}})
	req := httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/start", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec := httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("non-participant must be 403, got %d", rec.Code)
	}

	// now make the user a participant
	_ = svc.Join(context.Background(), "dm:1", "7", "alice", "")
	body, _ = json.Marshal(map[string]any{"type": "watch_party", "params": map[string]any{"url": "x"}})
	req = httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/start", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec = httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := svc.GetActivity(context.Background(), "dm:1")
	if got == nil || got.Type != "watch_party" {
		t.Errorf("activity not stored: %+v", got)
	}
}

func TestActivityStart_RejectsEmptyType(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	_ = svc.Join(context.Background(), "dm:1", "7", "alice", "")
	hub := newFakeHub()
	h := NewActivityHandler(svc, hub)

	body, _ := json.Marshal(map[string]any{"type": "", "params": map[string]any{}})
	req := httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/start", bytes.NewReader(body))
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec := httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty type, got %d", rec.Code)
	}
}

func TestActivityEnd_BroadcastsAndDeletes(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	_ = svc.Join(context.Background(), "dm:1", "7", "alice", "")
	_ = svc.StartActivity(context.Background(), "dm:1", "watch_party", 7, nil)
	hub := newFakeHub()
	h := NewActivityHandler(svc, hub)

	req := httptest.NewRequest(http.MethodPost, "/api/voice/dm:1/activity/end", nil)
	req = req.WithContext(withFakeUserID(req.Context(), 7))
	req.SetPathValue("vc", "dm:1")
	rec := httptest.NewRecorder()
	h.End(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d", rec.Code)
	}
	if got, _ := svc.GetActivity(context.Background(), "dm:1"); got != nil {
		t.Error("activity not deleted")
	}
	// verify broadcast happened
	hub.mu.Lock()
	defer hub.mu.Unlock()
	found := false
	for _, b := range hub.broadcasts {
		if b.channelID == "dm:1" && b.eventType == "ACTIVITY_END" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ACTIVITY_END broadcast missing")
	}
}

func TestActivityGet_ReturnsActiveActivity(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	_ = svc.StartActivity(context.Background(), "dm:1", "watch_party", 7, nil)
	hub := newFakeHub()
	h := NewActivityHandler(svc, hub)

	req := httptest.NewRequest(http.MethodGet, "/api/voice/dm:1/activity", nil)
	req.SetPathValue("vc", "dm:1")
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var got Activity
	json.NewDecoder(rec.Body).Decode(&got)
	if got.Type != "watch_party" || got.StartedBy != 7 {
		t.Errorf("got %+v", got)
	}
}

func TestActivityGet_NoActivityReturns204(t *testing.T) {
	rdb := newRedisForTest(t)
	svc := &Service{rdb: rdb}
	hub := newFakeHub()
	h := NewActivityHandler(svc, hub)

	req := httptest.NewRequest(http.MethodGet, "/api/voice/dm:99/activity", nil)
	req.SetPathValue("vc", "dm:99")
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}
