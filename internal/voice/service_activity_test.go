package voice

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSetGetEndActivity(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	if got, err := s.GetActivity(ctx, "dm:1"); err != nil || got != nil {
		t.Fatalf("expected nil/no error, got %+v err=%v", got, err)
	}

	if err := s.StartActivity(ctx, "dm:1", "watch_party", 7, json.RawMessage(`{"url":"https://example.com"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetActivity(ctx, "dm:1")
	if err != nil || got == nil {
		t.Fatalf("expected activity, got %+v err=%v", got, err)
	}
	if got.Type != "watch_party" || got.StartedBy != 7 {
		t.Errorf("got %+v", got)
	}

	// EndActivity removes
	if err := s.EndActivity(ctx, "dm:1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetActivity(ctx, "dm:1"); got != nil {
		t.Errorf("expected nil after end, got %+v", got)
	}
}

func TestEndIfEmpty_AlsoClearsActivity(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	_ = s.Join(ctx, "dm:1", "1", "alice", "")
	_ = s.StartActivity(ctx, "dm:1", "watch_party", 1, nil)
	_ = s.Leave(ctx, "dm:1", "1")

	if _, ended, _ := s.EndIfEmpty(ctx, "dm:1"); !ended {
		t.Fatal("expected ended=true on empty room")
	}
	if got, _ := s.GetActivity(ctx, "dm:1"); got != nil {
		t.Errorf("activity must be cleared on call end, got %+v", got)
	}
}
