package voice

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// uses a real-but-isolated Redis. Skip if not available.
func newRedisForTest(t *testing.T) *redis.Client {
	t.Helper()
	addr := "127.0.0.1:6379"
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		DB:          15,
		MaxRetries:  -1,
		DialTimeout: 200 * time.Millisecond,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not available at %s: %v", addr, err)
	}
	rdb.FlushDB(context.Background())
	t.Cleanup(func() { rdb.FlushDB(context.Background()); rdb.Close() })
	return rdb
}

func TestJoinSetsStartedAt_Once(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	if _, err := s.Join(ctx, "dm:1", "1", "alice", ""); err != nil {
		t.Fatal(err)
	}
	v1, err := rdb.Get(ctx, "voice:dm:1:started_at").Result()
	if err != nil {
		t.Fatalf("started_at not set: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := s.Join(ctx, "dm:1", "2", "bob", ""); err != nil {
		t.Fatal(err)
	}
	v2, _ := rdb.Get(ctx, "voice:dm:1:started_at").Result()
	if v1 != v2 {
		t.Errorf("started_at changed on second join: %q -> %q", v1, v2)
	}
}

func TestEndIfEmpty_ReturnsStartedAtOnce(t *testing.T) {
	rdb := newRedisForTest(t)
	s := &Service{rdb: rdb}
	ctx := context.Background()

	_, _ = s.Join(ctx, "dm:1", "1", "alice", "")
	_, _ = s.Join(ctx, "dm:1", "2", "bob", "")
	_ = s.Leave(ctx, "dm:1", "1")

	startedAtMs, ended, err := s.EndIfEmpty(ctx, "dm:1")
	if err != nil {
		t.Fatal(err)
	}
	if ended {
		t.Fatal("room not empty yet, EndIfEmpty must return ended=false")
	}
	if startedAtMs != 0 {
		t.Fatal("startedAtMs should be 0 when not ended")
	}

	_ = s.Leave(ctx, "dm:1", "2")
	startedAtMs, ended, err = s.EndIfEmpty(ctx, "dm:1")
	if err != nil {
		t.Fatal(err)
	}
	if !ended || startedAtMs == 0 {
		t.Fatalf("expected ended=true and startedAtMs > 0, got ended=%v startedAtMs=%d", ended, startedAtMs)
	}

	// second call returns ended=false (deduplicated)
	_, ended2, err := s.EndIfEmpty(ctx, "dm:1")
	if err != nil {
		t.Fatal(err)
	}
	if ended2 {
		t.Fatal("EndIfEmpty must dedupe")
	}
}
