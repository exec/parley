# AI Theme Generation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add AI-assisted CSS theme generation to the custom theme editor using a Redis-backed distributed queue and the Ollama cloud API.

**Architecture:** New `internal/ai/` package provides a reusable Redis queue, distributed lock, and Ollama API client. The theme-specific SSE handler in `internal/theme/ai_handler.go` calls into this package. Infrastructure variables (OLLAMA_API_URL, OLLAMA_API_KEY, OLLAMA_MODEL) are threaded through Terraform and the Go config.

**Tech Stack:** Go 1.23, Redis (go-redis/v9), Ollama cloud API (https://ollama.com/api/chat), React/TypeScript, SSE (fetch + ReadableStream)

---

## Chunk 1: internal/ai package — models, queue, client, worker

### Task 1: internal/ai/models.go

**Files:**
- Create: `/home/dylan/Developer/parley/internal/ai/models.go`

**Steps:**

- [ ] Create `/home/dylan/Developer/parley/internal/ai/models.go` with the following content:

```go
package ai

// AIGenJob represents a single queued AI generation request.
type AIGenJob struct {
	JobID       string `json:"job_id"`
	UserMessage string `json:"user_message"`
}

// AIGenResult holds the result of a completed AI generation job.
type AIGenResult struct {
	CSS   string `json:"css,omitempty"`
	Error string `json:"error,omitempty"`
}
```

- [ ] Commit: `git add internal/ai/models.go && git commit -m "feat(ai): add AIGenJob and AIGenResult types"`

---

### Task 2: internal/ai/queue.go

**Files:**
- Create: `/home/dylan/Developer/parley/internal/ai/queue.go`

**Steps:**

- [ ] Create `/home/dylan/Developer/parley/internal/ai/queue.go`:

```go
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
```

- [ ] Commit: `git add internal/ai/queue.go && git commit -m "feat(ai): add Redis-backed AIQueue with distributed lock"`

---

### Task 3: internal/ai/client.go

**Files:**
- Create: `/home/dylan/Developer/parley/internal/ai/client.go`

**Steps:**

- [ ] Create `/home/dylan/Developer/parley/internal/ai/client.go`:

```go
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient calls the native Ollama chat API.
type OllamaClient struct {
	URL    string
	Key    string
	Model  string
	client *http.Client
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Message ollamaMessage `json:"message"`
}

// NewOllamaClient creates a new OllamaClient with an 80-second HTTP timeout.
func NewOllamaClient(url, key, model string) *OllamaClient {
	return &OllamaClient{
		URL:   url,
		Key:   key,
		Model: model,
		client: &http.Client{
			Timeout: 80 * time.Second,
		},
	}
}

// Generate sends a chat request to the Ollama API and returns the assistant's reply.
// It calls POST {URL}/chat with a system message and a user message.
func (c *OllamaClient) Generate(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	payload := ollamaRequest{
		Model: c.Model,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
		Stream: false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ollama: status %d: %s", resp.StatusCode, snippet)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: decode response: %w", err)
	}

	return result.Message.Content, nil
}
```

- [ ] Commit: `git add internal/ai/client.go && git commit -m "feat(ai): add OllamaClient for native Ollama chat API"`

---

### Task 4: internal/ai/worker.go

**Files:**
- Create: `/home/dylan/Developer/parley/internal/ai/worker.go`

**Steps:**

- [ ] Create `/home/dylan/Developer/parley/internal/ai/worker.go`:

```go
package ai

import (
	"context"
	"log"
	"time"
)

// systemPrompt is sent as the system message to every generation request.
const systemPrompt = `You are a CSS theme generator for Parley, a Discord-like chat application.
Generate a CSS theme by setting custom properties on [data-theme] { }.

Parley CSS variable reference:
  --parley-bg              main chat area background
  --parley-bg-secondary    sidebar panels, message hover
  --parley-sidebar         left server/channel sidebar
  --parley-text-normal     primary message text
  --parley-text-muted      secondary/dimmed text
  --parley-accent          buttons, links, active states
  --parley-accent-hover    hover state for accent
  --parley-border          dividers and borders
  --parley-danger          destructive actions, errors
  --parley-success         positive states
  --parley-header-bg       top channel header bar
  --parley-input-bg        message input box background

You may also set these non-prefixed aliases used by some components:
  --bg-primary, --bg-secondary, --bg-tertiary,
  --text-primary, --text-secondary,
  --accent, --accent-hover, --border

Output ONLY raw CSS. No markdown code fences. No explanation. No comments.
Start immediately with [data-theme] { and end with }.`

// StartWorker runs a background goroutine that continuously polls the queue,
// acquires the distributed lock, pops the next job, calls Ollama, and publishes
// the result. It stops when ctx is cancelled.
func StartWorker(ctx context.Context, queue *AIQueue, ollama *OllamaClient) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := processOne(ctx, queue, ollama); err != nil {
				log.Printf("ai worker: %v", err)
			}
		}
	}
}

// processOne attempts to pop and process a single job.
func processOne(ctx context.Context, queue *AIQueue, ollama *OllamaClient) error {
	acquired, err := queue.TryAcquireLock(ctx)
	if err != nil {
		return err
	}
	if !acquired {
		return nil // another node is working
	}

	job, err := queue.PopJob(ctx)
	if err != nil {
		_ = queue.ReleaseLock(ctx)
		return err
	}
	if job == nil {
		// Queue is empty — release immediately.
		return queue.ReleaseLock(ctx)
	}

	log.Printf("ai worker: processing job %s", job.JobID)

	css, genErr := ollama.Generate(ctx, systemPrompt, job.UserMessage)

	var result AIGenResult
	if genErr != nil {
		log.Printf("ai worker: job %s failed: %v", job.JobID, genErr)
		result = AIGenResult{Error: "AI generation failed: " + genErr.Error()}
	} else {
		result = AIGenResult{CSS: css}
	}

	if pubErr := queue.PublishResult(ctx, job.JobID, result); pubErr != nil {
		log.Printf("ai worker: publish result for %s: %v", job.JobID, pubErr)
	}

	return queue.ReleaseLock(ctx)
}
```

- [ ] Commit: `git add internal/ai/worker.go && git commit -m "feat(ai): add StartWorker with 500ms poll loop and Ollama integration"`

---

### Task 5: internal/ai/queue_test.go

**Files:**
- Create: `/home/dylan/Developer/parley/internal/ai/queue_test.go`
- Modify: `/home/dylan/Developer/parley/go.mod` (add miniredis dependency)

**Steps:**

- [ ] Add miniredis to go.mod:

```
Run: cd /home/dylan/Developer/parley && go get github.com/alicebob/miniredis/v2
Expected: go: added github.com/alicebob/miniredis/v2 v2.x.x
```

- [ ] Create `/home/dylan/Developer/parley/internal/ai/queue_test.go`:

```go
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
	// Both queues share the same miniredis but have different nodeIDs
	// (NewAIQueue builds nodeID from hostname+pid; we test exclusion directly).
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

	// q1 acquires the lock
	ok, err := q1.TryAcquireLock(ctx)
	if err != nil || !ok {
		t.Fatalf("q1 failed to acquire lock: err=%v ok=%v", err, ok)
	}

	// q2 tries to release — must be a no-op (Lua script returns 0)
	if err := q2.ReleaseLock(ctx); err != nil {
		t.Fatalf("q2 ReleaseLock returned unexpected error: %v", err)
	}

	// q1 should still hold the lock: q2 should still fail to acquire
	ok2, err := q2.TryAcquireLock(ctx)
	if err != nil {
		t.Fatalf("q2 TryAcquireLock after spurious release: %v", err)
	}
	if ok2 {
		t.Fatal("q2 should NOT have acquired the lock — q1's lock was stolen")
	}

	// q1 properly releases
	if err := q1.ReleaseLock(ctx); err != nil {
		t.Fatalf("q1 ReleaseLock: %v", err)
	}

	// Now q2 should be able to acquire
	ok2, err = q2.TryAcquireLock(ctx)
	if err != nil {
		t.Fatalf("q2 TryAcquireLock after q1 release: %v", err)
	}
	if !ok2 {
		t.Fatal("q2 should have acquired the lock after q1 released it")
	}
}
```

- [ ] Run tests:

```
Run: cd /home/dylan/Developer/parley && go test ./internal/ai/...
Expected: ok  	parley/internal/ai
```

- [ ] Commit: `git add internal/ai/queue_test.go go.mod go.sum && git commit -m "test(ai): add queue tests with miniredis"`

---

## Chunk 2: Theme AI handler and backend wiring

### Task 6: internal/theme/ai_handler.go

**Files:**
- Create: `/home/dylan/Developer/parley/internal/theme/ai_handler.go`

**Steps:**

- [ ] Create `/home/dylan/Developer/parley/internal/theme/ai_handler.go`:

```go
package theme

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"parley/internal/ai"
)

// AIHandler handles AI-assisted theme generation via Server-Sent Events.
type AIHandler struct {
	queue *ai.AIQueue
}

// NewAIHandler creates a new AIHandler. If queue is nil (Redis or Ollama not
// configured), the handler returns 503 for all requests.
func NewAIHandler(queue *ai.AIQueue) *AIHandler {
	return &AIHandler{queue: queue}
}

type generateRequest struct {
	Prompt     string `json:"prompt"`
	CurrentCSS string `json:"current_css"`
}

// Generate handles POST /api/me/themes/generate.
// It enqueues a job and streams SSE events back to the client:
//
//	data: {"status":"queued","position":N}     — every second while waiting
//	data: {"status":"generating"}              — once popped by worker
//	data: {"status":"done","css":"..."}        — on success
//	data: {"status":"error","message":"..."}   — on failure or timeout
func (h *AIHandler) Generate(w http.ResponseWriter, r *http.Request) {
	if h.queue == nil {
		http.Error(w, `{"error":"AI generation not available"}`, http.StatusServiceUnavailable)
		return
	}

	uid, ok := userID(r)
	if !ok {
		writeErr(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}
	_ = uid // authenticated; used for future per-user rate limiting

	// Disable the server-level 15s write deadline so SSE can stream indefinitely.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "could not set write deadline")
		return
	}

	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Prompt) == 0 {
		writeErr(w, r, http.StatusBadRequest, "prompt is required")
		return
	}
	if len(req.Prompt) > 500 {
		req.Prompt = req.Prompt[:500]
	}

	// Build the user message that the worker will send to Ollama.
	userMessage := req.Prompt
	if req.CurrentCSS != "" {
		userMessage = "Here is my current theme CSS, please improve it:\n\n" + req.CurrentCSS + "\n\nAdditional instructions: " + req.Prompt
	}

	jobID := uuid.New().String()
	job := ai.AIGenJob{
		JobID:       jobID,
		UserMessage: userMessage,
	}

	// Enqueue the job.
	position, err := h.queue.Enqueue(r.Context(), job)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "failed to enqueue job")
		return
	}

	// Set up SSE response headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sendEvent := func(data map[string]interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	deadline := time.Now().Add(120 * time.Second)
	ctx := r.Context()

	// Phase 1: poll queue position every second until the job is popped (position → 0).
	positionTicker := time.NewTicker(1 * time.Second)
	defer positionTicker.Stop()

	// Send initial queued event.
	sendEvent(map[string]interface{}{"status": "queued", "position": position})

	for {
		if time.Now().After(deadline) {
			sendEvent(map[string]interface{}{"status": "error", "message": "request timed out"})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-positionTicker.C:
			pos, err := h.queue.GetPosition(ctx, jobID)
			if err != nil {
				sendEvent(map[string]interface{}{"status": "error", "message": "queue error"})
				return
			}
			if pos == 0 {
				// Job was popped by the worker — move to phase 2.
				positionTicker.Stop()
				goto pollResult
			}
			sendEvent(map[string]interface{}{"status": "queued", "position": pos})
		}
	}

pollResult:
	sendEvent(map[string]interface{}{"status": "generating"})

	// Phase 2: poll for result every 500ms.
	resultTicker := time.NewTicker(500 * time.Millisecond)
	defer resultTicker.Stop()

	for {
		if time.Now().After(deadline) {
			sendEvent(map[string]interface{}{"status": "error", "message": "request timed out"})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-resultTicker.C:
			result, found, err := h.queue.GetResult(ctx, jobID)
			if err != nil {
				sendEvent(map[string]interface{}{"status": "error", "message": "result error"})
				return
			}
			if !found {
				continue
			}
			if result.Error != "" {
				sendEvent(map[string]interface{}{"status": "error", "message": result.Error})
				return
			}
			sendEvent(map[string]interface{}{"status": "done", "css": result.CSS})
			return
		}
	}
}
```

- [ ] Commit: `git add internal/theme/ai_handler.go && git commit -m "feat(theme): add SSE AI generation handler"`

---

### Task 7: Config and wiring (cmd/api/main.go + cmd/api/routes.go)

**Files:**
- Modify: `/home/dylan/Developer/parley/cmd/api/main.go`
- Modify: `/home/dylan/Developer/parley/cmd/api/routes.go`

**Steps:**

#### 7a — main.go: extend Config and DefaultConfig

- [ ] In `/home/dylan/Developer/parley/cmd/api/main.go`, replace the `Config` struct (lines 34-38):

**Old:**
```go
// Config holds application configuration
type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
}
```

**New:**
```go
// Config holds application configuration
type Config struct {
	DatabaseURL  string
	JWTSecret    string
	Port         string
	OllamaAPIURL string // OLLAMA_API_URL — base URL for Ollama cloud API
	OllamaAPIKey string // OLLAMA_API_KEY — auth key; empty disables AI generation
	OllamaModel  string // OLLAMA_MODEL — model name, e.g. qwen3.5:9b
}
```

- [ ] Replace `DefaultConfig()` (lines 41-66) with:

**Old:**
```go
// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/parley?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is not set — refusing to start with an insecure default")
	}

	if os.Getenv("ADMIN_IMPERSONATE_SECRET") == "" {
		log.Println("WARNING: ADMIN_IMPERSONATE_SECRET is not set — the impersonation endpoint is disabled")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		DatabaseURL: databaseURL,
		JWTSecret:   jwtSecret,
		Port:        port,
	}
}
```

**New:**
```go
// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/parley?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is not set — refusing to start with an insecure default")
	}

	if os.Getenv("ADMIN_IMPERSONATE_SECRET") == "" {
		log.Println("WARNING: ADMIN_IMPERSONATE_SECRET is not set — the impersonation endpoint is disabled")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ollamaAPIURL := os.Getenv("OLLAMA_API_URL")
	if ollamaAPIURL == "" {
		ollamaAPIURL = "https://ollama.com/api"
	}

	ollamaAPIKey := os.Getenv("OLLAMA_API_KEY")
	if ollamaAPIKey == "" {
		log.Println("WARNING: OLLAMA_API_KEY is not set — AI theme generation is disabled")
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "qwen3.5:9b"
	}

	return &Config{
		DatabaseURL:  databaseURL,
		JWTSecret:    jwtSecret,
		Port:         port,
		OllamaAPIURL: ollamaAPIURL,
		OllamaAPIKey: ollamaAPIKey,
		OllamaModel:  ollamaModel,
	}
}
```

#### 7b — main.go: add `parley/internal/ai` import and thread Ollama config to setupRouter

- [ ] Add `"parley/internal/ai"` to the import block in `main.go`. The full updated import block becomes:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"parley/internal/ai"
	"parley/internal/auth"
	"parley/internal/bin"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/email"
	"parley/internal/message"
	"parley/internal/passkey"
	"parley/internal/server"
	"parley/internal/spaces"
	"parley/internal/voice"
	"parley/internal/websocket"
)
```

- [ ] In `main()`, update the `setupRouter` call (line 219) to pass the three new Ollama config values:

**Old:**
```go
	router := setupRouter(config, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, passkeySvc, parseCDNHost(spacesCDNURL), siteURL)
```

**New:**
```go
	router := setupRouter(config, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, passkeySvc, redisHub, parseCDNHost(spacesCDNURL), siteURL)
```

- [ ] Update the `setupRouter` function signature (lines 286-300) to accept `redisHub` and the three Ollama params, and forward them to `registerRoutes`:

**Old:**
```go
// setupRouter configures the chi router with all routes and middleware
func setupRouter(
	config *Config,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *websocket.Hub,
	spacesClient *spaces.Client,
	voiceSvc *voice.Service,
	binService *bin.Service,
	passkeySvc *passkey.Service,
	cdnHost string,
	siteURL string,
) *chi.Mux {
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// CORS middleware
	router.Use(corsMiddleware())

	// Mount routes
	tickets := newTicketStore()
	registerRoutes(router, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, tickets, passkeySvc, cdnHost, siteURL)

	return router
}
```

**New:**
```go
// setupRouter configures the chi router with all routes and middleware
func setupRouter(
	config *Config,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *websocket.Hub,
	spacesClient *spaces.Client,
	voiceSvc *voice.Service,
	binService *bin.Service,
	passkeySvc *passkey.Service,
	redisHub *websocket.RedisHub,
	cdnHost string,
	siteURL string,
) *chi.Mux {
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// CORS middleware
	router.Use(corsMiddleware())

	// Mount routes
	tickets := newTicketStore()
	registerRoutes(router, repo, authService, serverService, channelService, messageService, hub, spacesClient, voiceSvc, binService, tickets, passkeySvc, redisHub, config.OllamaAPIURL, config.OllamaAPIKey, config.OllamaModel, cdnHost, siteURL)

	return router
}
```

Note: the `ai` import in `main.go` is not directly used in this file since the `AIQueue` and `OllamaClient` are constructed in `routes.go`. Remove the `"parley/internal/ai"` import from `main.go` if it produces an "imported and not used" error — it is only needed in `routes.go`.

#### 7c — routes.go: update registerRoutes signature, body limit, and wire AI handler

- [ ] In `/home/dylan/Developer/parley/cmd/api/routes.go`, update the `registerRoutes` signature and body to match:

**Old function signature and body-limit middleware (lines 36-61):**
```go
// registerRoutes registers all API routes.
func registerRoutes(
	router *chi.Mux,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *ws.Hub,
	spacesClient *spaces.Client,
	voiceSvc *voice.Service,
	binService *bin.Service,
	tickets *ticketStore,
	passkeySvc *passkey.Service,
	cdnHost string,
	siteURL string,
) {
	// Cap request bodies at 64 KB for all routes except /api/upload,
	// which applies its own 50 MB limit inside the handler.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/upload" {
				r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
			}
			next.ServeHTTP(w, r)
		})
	})
```

**New:**
```go
// registerRoutes registers all API routes.
func registerRoutes(
	router *chi.Mux,
	repo *db.Repository,
	authService *auth.AuthService,
	serverService *server.ServerService,
	channelService *channel.ChannelService,
	messageService *message.MessageService,
	hub *ws.Hub,
	spacesClient *spaces.Client,
	voiceSvc *voice.Service,
	binService *bin.Service,
	tickets *ticketStore,
	passkeySvc *passkey.Service,
	redisHub *ws.RedisHub,
	ollamaAPIURL string,
	ollamaAPIKey string,
	ollamaModel string,
	cdnHost string,
	siteURL string,
) {
	// Cap request bodies at 64 KB for all routes except /api/upload (50 MB) and
	// /api/me/themes/generate (prompt + CSS can exceed 64 KB).
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/upload" && r.URL.Path != "/api/me/themes/generate" {
				r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
			}
			next.ServeHTTP(w, r)
		})
	})
```

- [ ] Add `"parley/internal/ai"` to the imports in `routes.go`. The full updated import block:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"parley/internal/ai"
	"parley/internal/auth"
	"parley/internal/bin"
	"parley/internal/channel"
	"parley/internal/db"
	"parley/internal/dm"
	"parley/internal/message"
	"parley/internal/passkey"
	"parley/internal/server"
	"parley/internal/spaces"
	"parley/internal/theme"
	"parley/internal/voice"
	ws "parley/internal/websocket"
)
```

- [ ] After the `themeHandler` construction (after line 118 in original routes.go), add the AI wiring block:

**After this existing block:**
```go
		// Theme handler — constructed once, used by both protected and public routes
		themeRepo := theme.NewRepository(repo.DB())
		themeSvc := theme.NewService(themeRepo, cdnHost, siteURL)
		themeHandler := theme.NewHandler(themeSvc)
```

**Add:**
```go
		// AI theme generation handler — requires Redis and a configured Ollama key.
		// If either is absent, NewAIHandler receives a nil queue and returns 503 for all requests.
		var aiQueue *ai.AIQueue
		if redisHub != nil && ollamaAPIKey != "" {
			aiQueue = ai.NewAIQueue(redisHub.Client())
			ollamaClient := ai.NewOllamaClient(ollamaAPIURL, ollamaAPIKey, ollamaModel)
			go ai.StartWorker(context.Background(), aiQueue, ollamaClient)
		}
		aiHandler := theme.NewAIHandler(aiQueue)
```

- [ ] Register the AI generate route inside the protected group, after the existing theme routes (after `r.Post("/me/themes/install/{token}", themeHandler.InstallTheme)`):

```go
			r.Post("/me/themes/generate", aiHandler.Generate)
```

The complete protected theme route block now reads:

```go
			// Theme routes
			r.Get("/me/preferences", themeHandler.GetPreferences)
			r.Put("/me/preferences/theme", themeHandler.SetActiveTheme)
			r.Post("/me/themes", themeHandler.CreateTheme)
			r.Put("/me/themes/{id}", themeHandler.UpdateTheme)
			r.Delete("/me/themes/{id}", themeHandler.DeleteTheme)
			r.Post("/me/themes/{id}/share", themeHandler.ShareTheme)
			r.Post("/me/themes/install/{token}", themeHandler.InstallTheme)
			r.Post("/me/themes/generate", aiHandler.Generate)
```

- [ ] Build to verify no compilation errors:

```
Run: cd /home/dylan/Developer/parley && go build ./cmd/api/...
Expected: (no output — clean build)
```

- [ ] Commit:

```
git add cmd/api/main.go cmd/api/routes.go && git commit -m "feat(api): wire AI queue, worker, and theme generate endpoint"
```

---

## Chunk 3: Terraform infrastructure

### Task 8: Terraform infrastructure

**Files:**
- Modify: `/home/dylan/Developer/parley/terraform/variables.tf`
- Modify: `/home/dylan/Developer/parley/terraform/main.tf`
- Modify: `/home/dylan/Developer/parley/terraform/userdata-api.sh`

**Steps:**

#### 8a — variables.tf: add three Ollama variables

- [ ] Append to the end of `/home/dylan/Developer/parley/terraform/variables.tf`:

```hcl
variable "ollama_api_url" {
  description = "Ollama API base URL"
  type        = string
  default     = "https://ollama.com/api"
}

variable "ollama_api_key" {
  description = "Ollama API key for AI theme generation"
  type        = string
  sensitive   = true
  default     = ""
}

variable "ollama_model" {
  description = "Ollama model name for AI theme generation"
  type        = string
  default     = "qwen3.5:9b"
}
```

#### 8b — main.tf: pass Ollama vars to templatefile

- [ ] In `/home/dylan/Developer/parley/terraform/main.tf`, inside the `templatefile("${path.module}/userdata-api.sh", { ... })` call for the `parley_api` resource, add the three new key-value pairs after the existing `LIVEKIT_URL` line:

**Find this block (lines 68-92 in main.tf):**
```hcl
  user_data = templatefile("${path.module}/userdata-api.sh", {
    DB_HOST                  = digitalocean_droplet.parley_db.ipv4_address_private
    DB_PORT                  = "5432"
    DB_NAME                  = "parley"
    DB_USER                  = "parley"
    DB_PASSWORD              = var.db_password
    JWT_SECRET               = var.jwt_secret
    PORT                     = "8080"
    REPO_URL                 = var.repo_url
    REDIS_HOST               = digitalocean_droplet.parley_db.ipv4_address_private
    SPACES_ACCESS_KEY        = var.spaces_access_key
    SPACES_SECRET_KEY        = var.spaces_secret_key
    SPACES_BUCKET            = var.spaces_bucket
    SPACES_REGION            = var.region
    SPACES_ENDPOINT          = var.spaces_endpoint
    SPACES_CDN_URL           = var.spaces_cdn_url
    BREVO_API_KEY            = var.brevo_api_key
    BREVO_FROM_EMAIL         = var.brevo_from_email
    SITE_URL                 = var.site_url
    ADMIN_IMPERSONATE_SECRET = var.admin_impersonate_secret
    LIVEKIT_API_KEY          = var.livekit_api_key
    LIVEKIT_API_SECRET       = var.livekit_api_secret
    LIVEKIT_URL              = var.livekit_url
    GIPHY_API_KEY            = var.giphy_api_key
  })
```

**Replace with:**
```hcl
  user_data = templatefile("${path.module}/userdata-api.sh", {
    DB_HOST                  = digitalocean_droplet.parley_db.ipv4_address_private
    DB_PORT                  = "5432"
    DB_NAME                  = "parley"
    DB_USER                  = "parley"
    DB_PASSWORD              = var.db_password
    JWT_SECRET               = var.jwt_secret
    PORT                     = "8080"
    REPO_URL                 = var.repo_url
    REDIS_HOST               = digitalocean_droplet.parley_db.ipv4_address_private
    SPACES_ACCESS_KEY        = var.spaces_access_key
    SPACES_SECRET_KEY        = var.spaces_secret_key
    SPACES_BUCKET            = var.spaces_bucket
    SPACES_REGION            = var.region
    SPACES_ENDPOINT          = var.spaces_endpoint
    SPACES_CDN_URL           = var.spaces_cdn_url
    BREVO_API_KEY            = var.brevo_api_key
    BREVO_FROM_EMAIL         = var.brevo_from_email
    SITE_URL                 = var.site_url
    ADMIN_IMPERSONATE_SECRET = var.admin_impersonate_secret
    LIVEKIT_API_KEY          = var.livekit_api_key
    LIVEKIT_API_SECRET       = var.livekit_api_secret
    LIVEKIT_URL              = var.livekit_url
    GIPHY_API_KEY            = var.giphy_api_key
    OLLAMA_API_URL           = var.ollama_api_url
    OLLAMA_API_KEY           = var.ollama_api_key
    OLLAMA_MODEL             = var.ollama_model
  })
```

#### 8c — userdata-api.sh: add variable declarations and env file entries

- [ ] In `/home/dylan/Developer/parley/terraform/userdata-api.sh`, add three variable declarations after the existing `LIVEKIT_URL` line at the top of the script (lines 15-17):

**Find (lines 15-17 in userdata-api.sh):**
```bash
LIVEKIT_API_KEY="${LIVEKIT_API_KEY}"
LIVEKIT_API_SECRET="${LIVEKIT_API_SECRET}"
LIVEKIT_URL="${LIVEKIT_URL}"
```

**Replace with:**
```bash
LIVEKIT_API_KEY="${LIVEKIT_API_KEY}"
LIVEKIT_API_SECRET="${LIVEKIT_API_SECRET}"
LIVEKIT_URL="${LIVEKIT_URL}"
OLLAMA_API_URL="${OLLAMA_API_URL}"
OLLAMA_API_KEY="${OLLAMA_API_KEY}"
OLLAMA_MODEL="${OLLAMA_MODEL}"
```

- [ ] In the `/etc/parley/env` heredoc (lines 126-145 in userdata-api.sh), add the three Ollama lines after the existing `LIVEKIT_URL` line:

**Find (inside the heredoc):**
```bash
LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}
LIVEKIT_URL=${LIVEKIT_URL}
EOF
```

**Replace with:**
```bash
LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}
LIVEKIT_URL=${LIVEKIT_URL}
OLLAMA_API_URL=${OLLAMA_API_URL}
OLLAMA_API_KEY=${OLLAMA_API_KEY}
OLLAMA_MODEL=${OLLAMA_MODEL}
EOF
```

- [ ] Commit:

```
git add terraform/variables.tf terraform/main.tf terraform/userdata-api.sh && git commit -m "feat(terraform): add Ollama API variables to API droplet provisioning"
```

---

## Chunk 4: Frontend

### Task 9: Frontend — CustomThemeEditor.tsx + CustomThemeEditor.css

**Files:**
- Modify: `/home/dylan/Developer/parley/frontend/src/components/settings/CustomThemeEditor.tsx`
- Modify: `/home/dylan/Developer/parley/frontend/src/components/settings/CustomThemeEditor.css`

**Steps:**

#### 9a — CustomThemeEditor.tsx: add AI state, helpers, handler, and UI

- [ ] Add three new state variables and `abortRef` after the existing `saving` state (line 67):

**Find:**
```tsx
  const [saving, setSaving] = useState(false);
```

**Replace with:**
```tsx
  const [saving, setSaving] = useState(false);
  const [aiPrompt, setAiPrompt] = useState('');
  const [aiStatus, setAiStatus] = useState<
    null | { type: 'queued'; position: number } | { type: 'generating' } | { type: 'error'; message: string }
  >(null);
  const abortRef = useRef<AbortController | null>(null);
```

- [ ] Add a cleanup effect after the existing `useEffect` for the debounced preview (after line 92). Insert before `const handleUpload`:

**Find:**
```tsx
  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
```

**Replace with:**
```tsx
  // Abort any in-flight AI generation when the component unmounts.
  useEffect(() => {
    return () => { abortRef.current?.abort(); };
  }, []);

  // ordinal returns a human-readable ordinal string, e.g. 1 → "1st", 2 → "2nd".
  function ordinal(n: number): string {
    const s = ['th', 'st', 'nd', 'rd'];
    const v = n % 100;
    return n + (s[(v - 20) % 10] || s[v] || s[0]);
  }

  const handleGenerate = async () => {
    if (!aiPrompt.trim()) return;
    abortRef.current?.abort();
    abortRef.current = new AbortController();
    setAiStatus({ type: 'generating' });
    try {
      const resp = await fetch('/api/me/themes/generate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${localStorage.getItem('token') || ''}`,
        },
        body: JSON.stringify({ prompt: aiPrompt, current_css: css }),
        signal: abortRef.current.signal,
      });
      if (!resp.ok || !resp.body) {
        const msg = resp.status === 503 ? 'AI generation not available' : 'Request failed';
        setAiStatus({ type: 'error', message: msg });
        return;
      }
      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split('\n\n');
        buf = parts.pop() ?? '';
        for (const part of parts) {
          if (!part.startsWith('data: ')) continue;
          try {
            const event = JSON.parse(part.slice(6));
            if (event.status === 'queued') {
              setAiStatus({ type: 'queued', position: event.position });
            } else if (event.status === 'generating') {
              setAiStatus({ type: 'generating' });
            } else if (event.status === 'done') {
              setCSS(event.css);
              setAiStatus(null);
              return;
            } else if (event.status === 'error') {
              setAiStatus({ type: 'error', message: event.message });
              return;
            }
          } catch { /* ignore parse errors */ }
        }
      }
    } catch (e) {
      if ((e as Error).name !== 'AbortError') {
        setAiStatus({ type: 'error', message: 'Connection lost' });
      }
    }
  };

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
```

- [ ] Insert the AI section UI between the Custom CSS field div and the error display. In the JSX, find:

**Find (lines 161-168):**
```tsx
      <div className="theme-editor-field">
        <label className="theme-editor-label">Custom CSS</label>
        <textarea className="theme-editor-textarea" value={css} onChange={e => setCSS(e.target.value)}
          placeholder={`/* Use [data-theme] to override variables — not :root */\n[data-theme] {\n  --parley-accent: hotpink;\n  --accent-rgb: 255, 105, 180;\n}\n\n/* Google Fonts allowed */\n@import url('https://fonts.googleapis.com/css2?family=Inter');`} />
        <div className="theme-editor-hint">Google Fonts allowed. All other external URLs are blocked.</div>
      </div>

      {error && <div className="theme-editor-error">{error}</div>}
```

**Replace with:**
```tsx
      <div className="theme-editor-field">
        <label className="theme-editor-label">Custom CSS</label>
        <textarea className="theme-editor-textarea" value={css} onChange={e => setCSS(e.target.value)}
          placeholder={`/* Use [data-theme] to override variables — not :root */\n[data-theme] {\n  --parley-accent: hotpink;\n  --accent-rgb: 255, 105, 180;\n}\n\n/* Google Fonts allowed */\n@import url('https://fonts.googleapis.com/css2?family=Inter');`} />
        <div className="theme-editor-hint">Google Fonts allowed. All other external URLs are blocked.</div>
      </div>

      <div className="theme-editor-field theme-editor-ai-section">
        <label className="theme-editor-label">Generate with AI</label>
        <textarea
          className="theme-editor-textarea theme-editor-ai-prompt"
          rows={3}
          maxLength={500}
          placeholder="Describe your theme… e.g. 'Dark purple cyberpunk with neon pink accents'"
          value={aiPrompt}
          onChange={e => setAiPrompt(e.target.value)}
          disabled={aiStatus?.type === 'queued' || aiStatus?.type === 'generating'}
        />
        <div className="theme-editor-ai-row">
          <button
            className="theme-editor-ai-btn"
            onClick={handleGenerate}
            disabled={!aiPrompt.trim() || aiStatus?.type === 'queued' || aiStatus?.type === 'generating'}
          >
            Generate
          </button>
          {(aiStatus?.type === 'queued' || aiStatus?.type === 'generating') && (
            <span className="theme-editor-ai-status">
              {aiStatus.type === 'queued'
                ? `${ordinal(aiStatus.position)} in queue…`
                : 'Generating…'}
            </span>
          )}
          {aiStatus?.type === 'error' && (
            <span className="theme-editor-ai-error">{aiStatus.message}</span>
          )}
        </div>
      </div>

      {error && <div className="theme-editor-error">{error}</div>}
```

#### 9b — CustomThemeEditor.css: add AI styles

- [ ] Append to the end of `/home/dylan/Developer/parley/frontend/src/components/settings/CustomThemeEditor.css`:

```css
/* AI Theme Generation */
.theme-editor-ai-section { margin-top: 4px; }
.theme-editor-ai-prompt { min-height: 64px; resize: vertical; }
.theme-editor-ai-row { display: flex; align-items: center; gap: 10px; margin-top: 6px; }
.theme-editor-ai-btn {
  padding: 6px 16px;
  background: var(--parley-accent, #32CD32);
  color: #fff;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.theme-editor-ai-btn:disabled { opacity: .45; cursor: not-allowed; }
.theme-editor-ai-btn:not(:disabled):hover { background: var(--parley-accent-hover, #28a428); }
.theme-editor-ai-status { font-size: 13px; color: var(--parley-text-muted, #666); font-style: italic; }
.theme-editor-ai-error { font-size: 13px; color: var(--parley-danger, #ed4245); }
```

- [ ] Verify the frontend builds:

```
Run: cd /home/dylan/Developer/parley/frontend && npm run build
Expected: (no TypeScript errors; build outputs to dist/)
```

- [ ] Commit:

```
git add frontend/src/components/settings/CustomThemeEditor.tsx frontend/src/components/settings/CustomThemeEditor.css && git commit -m "feat(frontend): add AI theme generation UI to CustomThemeEditor"
```

---

## Chunk 5: Deploy to existing servers

### Task 10: Deploy to existing servers

**Servers:**
- `174.138.51.177`
- `159.203.111.52`
- `167.71.186.109`

**Steps:**

#### 10a — Update /etc/parley/env on each server

Run the following on each of the three API servers. Substitute the correct values:

```
OLLAMA_API_KEY = 9387ccdc56fb4f258b1a9c5b1f72e7d8.GbzZvDlntgjdara1k3_L12Dq
OLLAMA_API_URL = https://ollama.com/api
OLLAMA_MODEL   = qwen3.5:9b
```

- [ ] SSH to server 1 (`174.138.51.177`) and run:

```bash
# Append Ollama env vars
cat >> /etc/parley/env <<'EOF'
OLLAMA_API_URL=https://ollama.com/api
OLLAMA_API_KEY=9387ccdc56fb4f258b1a9c5b1f72e7d8.GbzZvDlntgjdara1k3_L12Dq
OLLAMA_MODEL=qwen3.5:9b
EOF
chmod 600 /etc/parley/env
```

- [ ] SSH to server 2 (`159.203.111.52`) and repeat the same commands.
- [ ] SSH to server 3 (`167.71.186.109`) and repeat the same commands.

#### 10b — Pull latest code, rebuild, and restart on each server

Run the following on each server:

```bash
cd /parley && git pull origin main
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/root/go
export GOMODCACHE=/root/go/pkg/mod
export GOCACHE=/root/.cache/go-build
GONOSUMDB=* go build -mod=mod -o /usr/local/bin/parley-api ./cmd/api
cd frontend && npm ci && npm run build && cp -r dist/* /var/www/parley/
cd /parley
systemctl restart parley-api.service
```

- [ ] Repeat for all 3 servers.

#### 10c — Verify deployment

- [ ] On each server, check service health:

```bash
systemctl is-active parley-api.service
curl -sf http://localhost:8080/health
```

```
Expected: active
Expected: {"status":"ok"}
```

- [ ] Verify AI endpoint is reachable (from any server):

```bash
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/api/me/themes/generate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer REPLACE_WITH_VALID_JWT" \
  -d '{"prompt":"test"}'
```

```
Expected: 200 (SSE stream begins) or 401 (if token is invalid — confirms route is registered)
```

- [ ] Commit infrastructure notes (no code changes needed — already deployed):

```
Run: git -C /home/dylan/Developer/parley status
Expected: nothing to commit (all changes committed in prior tasks)
```

---

## Self-Review Checklist

1. **All imports accounted for:**
   - `routes.go` imports `"parley/internal/ai"` — used for `ai.NewAIQueue`, `ai.NewOllamaClient`, `ai.StartWorker`
   - `routes.go` already imports `ws "parley/internal/websocket"` — `*ws.RedisHub` type used in signature
   - `main.go` does NOT need a direct `internal/ai` import — Ollama config flows in as plain strings
   - `ai_handler.go` imports `"parley/internal/ai"`, `"github.com/google/uuid"` (already in go.mod v1.6.0)
   - `queue.go` imports `goredis "github.com/redis/go-redis/v9"` — same alias as websocket package
   - `queue_test.go` imports `"github.com/alicebob/miniredis/v2"` — added via `go get`

2. **registerRoutes signature propagates correctly:**
   - `main.go` line ~219: `setupRouter(config, ..., redisHub, parseCDNHost(...), siteURL)` — `redisHub` inserted before `cdnHost`
   - `setupRouter` signature updated to accept `redisHub *websocket.RedisHub` before `cdnHost`
   - `setupRouter` calls `registerRoutes(..., redisHub, config.OllamaAPIURL, config.OllamaAPIKey, config.OllamaModel, cdnHost, siteURL)`
   - `registerRoutes` signature matches with `redisHub`, `ollamaAPIURL`, `ollamaAPIKey`, `ollamaModel` before `cdnHost`

3. **miniredis added to go.mod:**
   - Step in Task 5 runs `go get github.com/alicebob/miniredis/v2` before creating the test file
   - `go.sum` updated automatically

4. **Body size exemption works for `/api/me/themes/generate`:**
   - Middleware check changed from `r.URL.Path != "/api/upload"` to `r.URL.Path != "/api/upload" && r.URL.Path != "/api/me/themes/generate"`
   - The route is registered as `r.Post("/me/themes/generate", ...)` inside `r.Route("/api", ...)` so the full path is `/api/me/themes/generate` — matches the exemption exactly

5. **SSE write deadline disabled:**
   - `ai_handler.go` calls `http.NewResponseController(w).SetWriteDeadline(time.Time{})` at the start of `Generate` — overrides the server-level 15s `WriteTimeout`

6. **Worker only starts when both Redis and Ollama are configured:**
   - Guard: `if redisHub != nil && ollamaAPIKey != ""` — safe degradation

7. **Distributed lock safety:**
   - Lua script ensures only the lock owner can release — prevents node B from stealing node A's lock
   - Lock TTL is 90s — longer than the 80s Ollama HTTP timeout, preventing lock expiry mid-generation
