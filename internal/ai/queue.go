package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	queueKey     = "parley:theme-gen-queue"
	positionsKey = "parley:theme-gen-positions"
	lockKey      = "parley:theme-gen-lock"
	resultPrefix = "parley:theme-gen-result:"
)

// AIQueue is a Redis-backed FIFO queue with a distributed lock for single-worker
// AI generation jobs.
type AIQueue struct {
	rdb    *goredis.Client
	nodeID string
}

// NewAIQueue creates a new AIQueue using the given Redis client.
// nodeID is derived from hostname + PID so each process has a unique identity
// for the distributed lock.
func NewAIQueue(rdb *goredis.Client) *AIQueue {
	host, _ := os.Hostname()
	nodeID := fmt.Sprintf("%s-%d", host, os.Getpid())
	return &AIQueue{rdb: rdb, nodeID: nodeID}
}

// Enqueue pushes a job onto the queue and records its position in the sorted set.
// Returns the 1-based queue position of the newly enqueued job.
func (q *AIQueue) Enqueue(ctx context.Context, job AIGenJob) (int64, error) {
	data, err := json.Marshal(job)
	if err != nil {
		return 0, fmt.Errorf("ai: marshal job: %w", err)
	}

	pipe := q.rdb.Pipeline()
	pipe.LPush(ctx, queueKey, data)
	pipe.ZAdd(ctx, positionsKey, goredis.Z{
		Score:  float64(time.Now().UnixNano()),
		Member: job.JobID,
	})
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("ai: enqueue pipeline: %w", err)
	}

	pos, err := q.GetPosition(ctx, job.JobID)
	if err != nil {
		return 0, err
	}
	return pos, nil
}

// GetPosition returns the 1-based queue position of jobID.
// Returns 0 if the job is not found (already popped or never enqueued).
func (q *AIQueue) GetPosition(ctx context.Context, jobID string) (int64, error) {
	rank, err := q.rdb.ZRank(ctx, positionsKey, jobID).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ai: zrank: %w", err)
	}
	// rank is 0-based; add 1 for human-readable 1-based position
	return rank + 1, nil
}

// TryAcquireLock attempts to acquire the distributed generation lock using SET NX EX.
// Returns true if this node now holds the lock, false if another node holds it.
func (q *AIQueue) TryAcquireLock(ctx context.Context) (bool, error) {
	ok, err := q.rdb.SetNX(ctx, lockKey, q.nodeID, 90*time.Second).Result()
	if err != nil {
		return false, fmt.Errorf("ai: acquire lock: %w", err)
	}
	return ok, nil
}

// releaseLockScript atomically releases the lock only if this node owns it.
var releaseLockScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)

// ReleaseLock releases the distributed lock. It is a no-op if another node holds the lock.
func (q *AIQueue) ReleaseLock(ctx context.Context) error {
	if err := releaseLockScript.Run(ctx, q.rdb, []string{lockKey}, q.nodeID).Err(); err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("ai: release lock: %w", err)
	}
	return nil
}

// PopJob atomically pops the next job from the queue and removes it from the
// positions sorted set. Returns (nil, nil) when the queue is empty.
func (q *AIQueue) PopJob(ctx context.Context) (*AIGenJob, error) {
	pipe := q.rdb.Pipeline()
	rpopCmd := pipe.RPop(ctx, queueKey)
	// We do not yet know the jobID, so we ZREM after parsing below.
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("ai: pop pipeline exec: %w", err)
	}

	data, err := rpopCmd.Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ai: rpop: %w", err)
	}

	var job AIGenJob
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return nil, fmt.Errorf("ai: unmarshal job: %w", err)
	}

	// Remove from positions set now that we know the jobID.
	if err := q.rdb.ZRem(ctx, positionsKey, job.JobID).Err(); err != nil && !errors.Is(err, goredis.Nil) {
		return nil, fmt.Errorf("ai: zrem positions: %w", err)
	}

	return &job, nil
}

// PublishResult stores the generation result for retrieval by the SSE handler.
// The key expires after 60 seconds.
func (q *AIQueue) PublishResult(ctx context.Context, jobID string, result AIGenResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("ai: marshal result: %w", err)
	}
	key := resultPrefix + jobID
	if err := q.rdb.Set(ctx, key, data, 60*time.Second).Err(); err != nil {
		return fmt.Errorf("ai: publish result: %w", err)
	}
	return nil
}

// GetResult retrieves a previously published result.
// Returns (nil, false, nil) when the key does not exist (not yet ready).
// Returns (result, true, nil) when the result is available.
func (q *AIQueue) GetResult(ctx context.Context, jobID string) (*AIGenResult, bool, error) {
	key := resultPrefix + jobID
	data, err := q.rdb.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("ai: get result: %w", err)
	}

	var result AIGenResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, false, fmt.Errorf("ai: unmarshal result: %w", err)
	}
	return &result, true, nil
}
