package ai

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// newTestQueue creates an AIQueue backed by a fresh miniredis server.
// The caller is responsible for calling mr.Close().
func newTestQueue(t *testing.T) (*AIQueue, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	q := NewAIQueue(rdb)
	return q, mr
}

func TestEnqueue_ReturnsPosition(t *testing.T) {
	ctx := context.Background()
	q, mr := newTestQueue(t)
	defer mr.Close()

	job1 := AIGenJob{JobID: "job-1", UserMessage: "dark theme"}
	pos1, err := q.Enqueue(ctx, job1)
	if err != nil {
		t.Fatalf("Enqueue job1: %v", err)
	}
	if pos1 != 1 {
		t.Errorf("expected position 1, got %d", pos1)
	}

	job2 := AIGenJob{JobID: "job-2", UserMessage: "light theme"}
	pos2, err := q.Enqueue(ctx, job2)
	if err != nil {
		t.Fatalf("Enqueue job2: %v", err)
	}
	if pos2 != 2 {
		t.Errorf("expected position 2, got %d", pos2)
	}
}

func TestGetPosition_ZeroAfterPop(t *testing.T) {
	ctx := context.Background()
	q, mr := newTestQueue(t)
	defer mr.Close()

	job := AIGenJob{JobID: "job-pop", UserMessage: "neon theme"}
	if _, err := q.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	popped, err := q.PopJob(ctx)
	if err != nil {
		t.Fatalf("PopJob: %v", err)
	}
	if popped == nil {
		t.Fatal("expected a job, got nil")
	}
	if popped.JobID != job.JobID {
		t.Errorf("expected job-pop, got %s", popped.JobID)
	}

	pos, err := q.GetPosition(ctx, job.JobID)
	if err != nil {
		t.Fatalf("GetPosition after pop: %v", err)
	}
	if pos != 0 {
		t.Errorf("expected position 0 after pop, got %d", pos)
	}
}

func TestPublishAndGetResult_RoundTrip(t *testing.T) {
	ctx := context.Background()
	q, mr := newTestQueue(t)
	defer mr.Close()

	const jobID = "job-result"
	want := AIGenResult{CSS: "[data-theme] { --parley-accent: hotpink; }"}

	if err := q.PublishResult(ctx, jobID, want); err != nil {
		t.Fatalf("PublishResult: %v", err)
	}

	got, found, err := q.GetResult(ctx, jobID)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if !found {
		t.Fatal("expected result to be found")
	}
	if got.CSS != want.CSS {
		t.Errorf("CSS mismatch: got %q, want %q", got.CSS, want.CSS)
	}
}

func TestGetResult_NotFoundBeforePublish(t *testing.T) {
	ctx := context.Background()
	q, mr := newTestQueue(t)
	defer mr.Close()

	result, found, err := q.GetResult(ctx, "nonexistent-job")
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if found {
		t.Error("expected not found for nonexistent job")
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestTryAcquireLock_MutualExclusion(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	q1 := &AIQueue{rdb: rdb, nodeID: "node-A"}
	q2 := &AIQueue{rdb: rdb, nodeID: "node-B"}

	ok1, err := q1.TryAcquireLock(ctx)
	if err != nil {
		t.Fatalf("q1 TryAcquireLock: %v", err)
	}
	if !ok1 {
		t.Fatal("q1 should have acquired the lock")
	}

	ok2, err := q2.TryAcquireLock(ctx)
	if err != nil {
		t.Fatalf("q2 TryAcquireLock: %v", err)
	}
	if ok2 {
		t.Fatal("q2 should NOT have acquired the lock while q1 holds it")
	}
}

func TestReleaseLock_DoesNotStealAnotherNodesLock(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()

	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	q1 := &AIQueue{rdb: rdb, nodeID: "node-A"}
	q2 := &AIQueue{rdb: rdb, nodeID: "node-B"}

	ok, err := q1.TryAcquireLock(ctx)
	if err != nil || !ok {
		t.Fatalf("q1 failed to acquire lock: err=%v ok=%v", err, ok)
	}

	if err := q2.ReleaseLock(ctx); err != nil {
		t.Fatalf("q2 ReleaseLock returned unexpected error: %v", err)
	}

	ok2, err := q2.TryAcquireLock(ctx)
	if err != nil {
		t.Fatalf("q2 TryAcquireLock after spurious release: %v", err)
	}
	if ok2 {
		t.Fatal("q2 should NOT have acquired the lock — q1's lock was stolen")
	}

	if err := q1.ReleaseLock(ctx); err != nil {
		t.Fatalf("q1 ReleaseLock: %v", err)
	}

	ok2, err = q2.TryAcquireLock(ctx)
	if err != nil {
		t.Fatalf("q2 TryAcquireLock after q1 release: %v", err)
	}
	if !ok2 {
		t.Fatal("q2 should have acquired the lock after q1 released it")
	}
}
