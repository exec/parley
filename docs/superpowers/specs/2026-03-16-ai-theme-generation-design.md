# AI Theme Generation — Design Spec

**Date:** 2026-03-16
**Status:** Draft
**Depends on:** `2026-03-16-theming-design.md` (theming system must be fully deployed before this feature can ship)

---

## 1. Goal

Add AI-assisted CSS theme generation to the custom theme editor (`CustomThemeEditor.tsx`). The user types a natural-language prompt describing their desired theme (or leaves the existing CSS in the textarea so the model can improve it), clicks **Generate**, and receives CSS output streamed back over a Server-Sent Events connection.

A Redis-backed distributed queue ensures only one Ollama request is in-flight at a time across all API nodes, preventing rate limit overruns on the Ollama cloud API. The feature is silently disabled (endpoint returns `503`) when the `OLLAMA_API_KEY` environment variable is absent.

---

## 2. Architecture Overview

```
Browser (SSE)
    │  POST /api/me/themes/generate
    │  text/event-stream response
    ▼
ai_handler.go
    │  Enqueue job → Redis list
    │  Open SSE loop, poll position → poll result
    ▼
ai_worker.go  (one goroutine per API node, polls every 500 ms)
    │  TryAcquireLock → PopJob → callOllama → PublishResult → ReleaseLock
    ▼
Ollama Cloud API  (POST {OLLAMA_API_URL}/chat)
```

The distributed lock guarantees that across N running API nodes only one node calls Ollama at a time. If the lock holder's node dies mid-request, the 90 s TTL automatically releases the lock so another node can proceed.

---

## 3. Backend Components

All new backend code lives in `internal/theme/`.

### 3.1 `internal/theme/ai_models.go` — shared types

```go
type AIGenJob struct {
    JobID      string `json:"job_id"`
    UserID     int64  `json:"user_id"`
    Prompt     string `json:"prompt"`
    CurrentCSS string `json:"current_css"`
}

type AIGenResult struct {
    CSS   string `json:"css,omitempty"`
    Error string `json:"error,omitempty"`
}
```

A dedicated models file avoids circular imports between `ai_queue.go`, `ai_worker.go`, and `ai_handler.go`.

### 3.2 `internal/theme/ai_queue.go`

```go
type AIQueue struct {
    redis  *redis.Client
    nodeID string   // unique per process, e.g. hostname+pid
}

func NewAIQueue(client *redis.Client) *AIQueue

// Enqueue pushes a job to the Redis list and adds it to the sorted set.
// Score = current Unix timestamp (jobs processed FIFO).
// Returns the 1-based queue position.
func (q *AIQueue) Enqueue(ctx context.Context, job AIGenJob) (position int, err error)

// GetPosition returns the 1-based position of jobID in the sorted set.
// Returns 0 when the job is no longer queued (either being processed or done).
func (q *AIQueue) GetPosition(ctx context.Context, jobID string) (int, error)

// TryAcquireLock attempts SET NX EX 90 on parley:theme-gen-lock.
// Returns true only if this node acquired the lock.
func (q *AIQueue) TryAcquireLock(ctx context.Context) bool

// ReleaseLock DELs the lock key only if its value still matches q.nodeID,
// using a Lua script for atomicity.
func (q *AIQueue) ReleaseLock(ctx context.Context)

// PopJob atomically removes the oldest job from parley:theme-gen-queue (LPOP)
// and deletes its sorted-set entry in a single pipeline.
// Returns nil, nil when the queue is empty.
func (q *AIQueue) PopJob(ctx context.Context) (*AIGenJob, error)

// PublishResult writes the result JSON to parley:theme-gen-result:<jobID> with a 60 s TTL.
func (q *AIQueue) PublishResult(ctx context.Context, jobID string, result AIGenResult) error

// GetResult fetches and decodes the result key.
// Returns (result, true, nil) when the key exists, (nil, false, nil) when absent.
func (q *AIQueue) GetResult(ctx context.Context, jobID string) (*AIGenResult, bool, error)
```

**Redis keys:**

| Key | Type | Purpose |
|-----|------|---------|
| `parley:theme-gen-queue` | List | FIFO job queue (JSON-encoded `AIGenJob`) |
| `parley:theme-gen-positions` | Sorted set | Per-job position tracking; score = enqueue timestamp |
| `parley:theme-gen-lock` | String | Distributed lock; value = nodeID; TTL 90 s |
| `parley:theme-gen-result:<jobID>` | String | JSON result; TTL 60 s |

**Race condition — dual-pop hazard:** `LPOP` is atomic in Redis; only one caller receives a given job. However, if two nodes simultaneously win the lock check and both attempt `LPOP`, only one gets the job — the other gets `nil` and loops. This is benign because `TryAcquireLock` uses `SET NX`, so only one node can hold the lock at a time. The `LPOP` inside the locked critical section is therefore safe: the lock holder is the only process that will ever pop. The lock must be acquired *before* `LPOP`, not after.

**Sorted-set cleanup:** `PopJob` removes the job's entry from `parley:theme-gen-positions` in the same pipeline as the `LPOP`, so `GetPosition` returns `0` (job no longer queued) as soon as the worker picks it up. This is the signal the SSE handler uses to transition from "queued" to "polling for result".

### 3.3 `internal/theme/ai_worker.go`

```go
// StartAIWorker launches the background goroutine. Call once at startup.
// Exits cleanly when ctx is cancelled.
func StartAIWorker(ctx context.Context, queue *AIQueue, ollamaURL, ollamaKey, model string)
```

Worker loop (pseudo-code):

```
every 500 ms:
    if !TryAcquireLock(): continue
    job = PopJob()
    if job == nil:
        ReleaseLock()
        continue
    result = callOllama(job)
    PublishResult(job.JobID, result)
    ReleaseLock()
```

**Ollama API call** — `POST {ollamaURL}/chat`:

```json
{
  "model": "<model>",
  "messages": [
    {"role": "system", "content": "<system prompt>"},
    {"role": "user",   "content": "<user message>"}
  ],
  "stream": false
}
```

The HTTP client must set `Authorization: Bearer <ollamaKey>` and use a timeout of 80 s (shorter than the lock TTL of 90 s and the SSE timeout of 120 s).

**System prompt** (verbatim; stored as a package-level constant in `ai_worker.go`):

```
You are a CSS theme generator for Parley, a Discord-like chat application.
Generate a CSS theme by setting the following custom properties on [data-theme] { }.

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
Start immediately with [data-theme] { and end with }.
```

**User message construction:**
- If `currentCSS` is non-empty: `"Here is my current theme CSS, please improve it:\n\n" + currentCSS + "\n\nAdditional instructions: " + prompt`
- If `currentCSS` is empty: `prompt` directly

**Error handling in worker:** If `callOllama` returns an error (network failure, non-200 response, timeout, invalid JSON), `PublishResult` is called with `AIGenResult{Error: err.Error()}`. The worker never panics; all errors are recovered and published so the SSE client receives a clean error event rather than hanging.

### 3.4 `internal/theme/ai_handler.go`

```go
// NewAIHandler constructs the handler. queue may be nil when the feature is disabled.
func NewAIHandler(queue *AIQueue) *AIHandler

// Generate handles POST /api/me/themes/generate.
func (h *AIHandler) Generate(w http.ResponseWriter, r *http.Request)
```

**Request body:**
```json
{"prompt": "...", "current_css": "..."}
```

**Validation:**
- `prompt` required, max 500 chars; else `400 {"error": "prompt required, max 500 characters"}`
- `current_css` optional, max 51,200 bytes (50 KB limit, consistent with theme CSS limit)

**Feature-disabled path:** If `h.queue == nil`, respond `503 {"error": "AI theme generation is not configured on this server"}`.

**SSE setup:**
```go
w.Header().Set("Content-Type", "text/event-stream")
w.Header().Set("Cache-Control", "no-cache")
w.Header().Set("X-Accel-Buffering", "no")  // disables nginx buffering
w.Header().Set("Connection", "keep-alive")
w.WriteHeader(http.StatusOK)
flusher, ok := w.(http.Flusher)
```

**SSE event loop:**

```
totalDeadline = time.Now().Add(120s)
jobID = uuid.New()
position, err = queue.Enqueue(ctx, AIGenJob{...})

phase 1 — queue wait (poll every 1 s):
  while time.Now() < totalDeadline:
    pos = queue.GetPosition(ctx, jobID)
    if pos > 0:
      send: data: {"status":"queued","position":pos}\n\n
      flush
      sleep 1s
    else:
      break  // job popped by worker, move to phase 2

phase 2 — result wait (poll every 500 ms):
  send: data: {"status":"generating"}\n\n
  flush
  while time.Now() < totalDeadline:
    result, found = queue.GetResult(ctx, jobID)
    if found:
      if result.Error != "":
        send: data: {"status":"error","message":result.Error}\n\n
      else:
        send: data: {"status":"done","css":result.CSS}\n\n
      flush
      return
    sleep 500ms

// deadline exceeded
send: data: {"status":"error","message":"request timed out"}\n\n
```

**Client disconnect cleanup:** The handler uses `r.Context()` throughout. When the client closes the SSE connection, nginx propagates a context cancellation. All Redis calls in the loop use this context, so they return immediately. The enqueued job remains in Redis (the worker will still process it and publish a result that nobody reads; the result TTL of 60 s ensures cleanup). This is an acceptable trade-off — jobs are small and infrequent.

**WriteTimeout conflict:** The default `http.Server.WriteTimeout` of 15 s (see `cmd/api/main.go`) will close SSE connections before the 120 s limit is reached. The SSE endpoint must either disable `WriteTimeout` for this route or use a per-response `http.ResponseController` to extend the deadline. Recommended approach: use `http.NewResponseController(w).SetWriteDeadline(time.Time{})` at the start of the SSE handler to clear the deadline for this connection.

---

## 4. API Endpoint

**`POST /api/me/themes/generate`**

| Property | Value |
|----------|-------|
| Auth | Bearer JWT (existing middleware) |
| Request `Content-Type` | `application/json` |
| Response `Content-Type` | `text/event-stream` |
| Timeout | 120 s total |

**Request body:**
```json
{
  "prompt": "A dark cyberpunk theme with neon pink and cyan accents",
  "current_css": ""
}
```

**SSE event shapes:**

```
data: {"status":"queued","position":2}

data: {"status":"generating"}

data: {"status":"done","css":"[data-theme] {\n  --parley-bg: #0d0d1a;\n  ..."}

data: {"status":"error","message":"ollama API returned 429: rate limited"}
```

**Rate limiting:** No additional rate limiter is added at the HTTP layer. The distributed lock already serializes processing to one request at a time globally. Users who submit while the queue is deep will see their position in SSE events and can cancel (close the tab) if desired. A per-user rate limiter (e.g. 5 generate calls per minute) can be added in a follow-up if abuse is observed.

---

## 5. Config Changes

### `cmd/api/main.go` — Config struct additions

```go
type Config struct {
    // ... existing fields ...
    OllamaAPIURL string  // env: OLLAMA_API_URL, default: "https://ollama.com/api"
    OllamaAPIKey string  // env: OLLAMA_API_KEY, default: "" (feature disabled if empty)
    OllamaModel  string  // env: OLLAMA_MODEL,   default: "qwen3.5:9b"
}
```

In `DefaultConfig()`:
```go
ollamaAPIURL := os.Getenv("OLLAMA_API_URL")
if ollamaAPIURL == "" {
    ollamaAPIURL = "https://ollama.com/api"
}
ollamaModel := os.Getenv("OLLAMA_MODEL")
if ollamaModel == "" {
    ollamaModel = "qwen3.5:9b"
}
// OllamaAPIKey: read directly, no default, no fatal if empty
```

`OllamaAPIKey` being empty causes the feature to be silently disabled — a `log.Println` warning is emitted at startup.

### `cmd/api/routes.go` — wiring

In `registerRoutes`, after the existing theme handler construction:

```go
var aiQueue *theme.AIQueue
if redisHub != nil && config.OllamaAPIKey != "" {
    aiQueue = theme.NewAIQueue(redisHub.Client())
    go theme.StartAIWorker(ctx, aiQueue, config.OllamaAPIURL, config.OllamaAPIKey, config.OllamaModel)
}
aiHandler := theme.NewAIHandler(aiQueue)

// inside the protected route group:
r.Post("/me/themes/generate", aiHandler.Generate)
```

Note: `registerRoutes` does not currently receive a `context.Context`. The worker needs one for graceful shutdown. Options:
- Pass `context.Background()` to `StartAIWorker` and rely on process termination (acceptable for v1).
- Thread the server's shutdown context through `registerRoutes` in a follow-up.

For v1, passing `context.Background()` to the worker goroutine is acceptable because the API server uses `srv.Shutdown()` with a 30 s grace period on SIGTERM, during which in-flight requests complete normally.

---

## 6. Frontend Changes

### `frontend/src/components/settings/CustomThemeEditor.tsx`

A new "Generate with AI" section is added below the Custom CSS `<textarea>` and above the `{error && ...}` display.

**New state variables:**
```ts
const [aiPrompt, setAiPrompt] = useState('');
const [aiStatus, setAiStatus] = useState<
  | null
  | { type: 'generating'; position?: number }
  | { type: 'error'; message: string }
>( null );
const sseRef = useRef<EventSource | null>(null);
```

Note: `EventSource` does not support custom headers, so it cannot send the `Authorization: Bearer` token. The SSE request must be made with `fetch()` using `ReadableStream` body reading instead. The existing `getAuthHeaders()` pattern (used by other API calls in the frontend) returns `{ Authorization: 'Bearer <token>' }` and is used here directly.

**SSE fetch pattern:**

```ts
const handleGenerate = async () => {
  setAiStatus({ type: 'generating' });
  const resp = await fetch('/api/me/themes/generate', {
    method: 'POST',
    headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt: aiPrompt, current_css: css }),
    signal: abortRef.current.signal,
  });
  if (!resp.ok || !resp.body) {
    setAiStatus({ type: 'error', message: 'Request failed' });
    return;
  }
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    // parse SSE lines from buf
    for (const line of buf.split('\n\n')) {
      if (!line.startsWith('data: ')) continue;
      const event = JSON.parse(line.slice(6));
      if (event.status === 'queued') {
        setAiStatus({ type: 'generating', position: event.position });
      } else if (event.status === 'generating') {
        setAiStatus({ type: 'generating' });
      } else if (event.status === 'done') {
        setCSS(event.css);   // replaces textarea content; triggers debounced preview
        setAiStatus(null);
        return;
      } else if (event.status === 'error') {
        setAiStatus({ type: 'error', message: event.message });
        return;
      }
    }
    buf = buf.split('\n\n').at(-1) ?? '';
  }
};
```

An `AbortController` stored in `abortRef` is used to cancel the in-flight request on component unmount (`useEffect` cleanup) or when the user closes the editor.

**Rendered UI (inserted after the CSS textarea field):**

```tsx
<div className="theme-editor-field theme-editor-ai">
  <label className="theme-editor-label">Generate with AI</label>
  <textarea
    className="theme-editor-input theme-editor-ai-prompt"
    rows={3}
    maxLength={500}
    placeholder="Describe your theme… e.g. 'Dark purple cyberpunk with neon pink accents'"
    value={aiPrompt}
    onChange={e => setAiPrompt(e.target.value)}
    disabled={aiStatus?.type === 'generating'}
  />
  <div className="theme-editor-ai-row">
    <button
      className="theme-editor-ai-btn"
      onClick={handleGenerate}
      disabled={!aiPrompt.trim() || aiStatus?.type === 'generating'}
    >
      Generate
    </button>
    {aiStatus?.type === 'generating' && (
      <span className="theme-editor-ai-status">
        {aiStatus.position
          ? `${ordinal(aiStatus.position)} in queue…`
          : 'Generating…'}
      </span>
    )}
    {aiStatus?.type === 'error' && (
      <span className="theme-editor-ai-error">{aiStatus.message}</span>
    )}
  </div>
</div>
```

No new API file needed — the `fetch` call lives directly in `CustomThemeEditor.tsx` alongside the existing inline upload logic.

### `frontend/src/components/settings/CustomThemeEditor.css`

Add styles for `.theme-editor-ai`, `.theme-editor-ai-prompt`, `.theme-editor-ai-row`, `.theme-editor-ai-btn`, `.theme-editor-ai-status`, `.theme-editor-ai-error`.

---

## 7. Infrastructure Changes

### New environment variables

| Variable | Default | Secret | Purpose |
|----------|---------|--------|---------|
| `OLLAMA_API_URL` | `https://ollama.com/api` | No | Ollama cloud API base URL |
| `OLLAMA_API_KEY` | _(empty)_ | **Yes** | API key; feature disabled if empty |
| `OLLAMA_MODEL` | `qwen3.5:9b` | No | Model name passed to Ollama |

### Files to update

| File | Change |
|------|--------|
| `cmd/api/main.go` | Add 3 fields to `Config`, read in `DefaultConfig()`, pass to `registerRoutes` |
| `cmd/api/routes.go` | Construct `AIQueue`, start worker goroutine, register `POST /me/themes/generate` |
| `internal/theme/ai_models.go` | New: `AIGenJob`, `AIGenResult` types |
| `internal/theme/ai_queue.go` | New: `AIQueue` implementation |
| `internal/theme/ai_worker.go` | New: `StartAIWorker`, `callOllama` |
| `internal/theme/ai_handler.go` | New: `AIHandler.Generate` SSE handler |
| `frontend/src/components/settings/CustomThemeEditor.tsx` | Add AI generate section |
| `frontend/src/components/settings/CustomThemeEditor.css` | Add AI section styles |
| `terraform/variables.tf` | Add `ollama_api_url` (default), `ollama_api_key` (sensitive), `ollama_model` (default) |
| `terraform/main.tf` | Pass new vars to `templatefile()` call for `userdata-api.sh` |
| `terraform/userdata-api.sh` | Add `OLLAMA_API_URL`, `OLLAMA_API_KEY`, `OLLAMA_MODEL` to `/etc/parley/env` |

**Note on `.github/workflows/deploy.yml`:** Hot deploys via `git pull` on existing droplets rely on `/etc/parley/env` already being present (written during initial Terraform provision). No change is needed to `deploy.yml`. However, on existing servers that were provisioned before this feature, `OLLAMA_API_KEY` must be added manually to `/etc/parley/env` and the service restarted. Document this in the deployment runbook.

---

## 8. Testing Plan

### Unit tests (`internal/theme/ai_queue_test.go`)

Use `github.com/go-redis/redismock/v9` (already a common Go Redis testing pattern) to mock Redis:

- `TestEnqueue_FirstJob` — enqueue one job, assert position = 1
- `TestEnqueue_SecondJob` — enqueue two jobs, assert positions 1 and 2
- `TestGetPosition_AfterPop` — enqueue job, pop it, assert GetPosition returns 0
- `TestPublishResult_GetResult_RoundTrip` — publish a result, get it back, assert fields match
- `TestTryAcquireLock_OnlyOneHolder` — two goroutines race to acquire; assert exactly one succeeds
- `TestReleaseLock_NoStealing` — node A acquires, node B cannot release it (Lua script safety)

### Integration test (`internal/theme/ai_worker_test.go`)

Spin up `miniredis` (in-process Redis for testing) and an `httptest.NewServer` simulating the Ollama API:

- `TestWorkerProcessesJob` — enqueue job, start worker, assert result published within 2 s
- `TestWorkerHandlesOllamaError` — Ollama server returns 500; assert result has `error` field
- `TestWorkerHandlesTimeout` — Ollama server hangs; assert worker publishes error after HTTP client timeout

### Frontend manual tests

1. Open CustomThemeEditor with no existing CSS. Type a prompt, click Generate. Observe "Generating…" status, then CSS textarea populated.
2. Open editor with existing CSS. Click Generate without typing a prompt — button should be disabled.
3. Open editor, start generation, close editor before it completes. Reopen — no stale SSE connection or UI state.
4. Simulate queue: open two browser tabs, both click Generate simultaneously. First tab shows "Generating…", second shows "2nd in queue…".
5. Test with `OLLAMA_API_KEY` unset (server returns 503) — frontend should show error message.

---

## 9. Security Considerations

### Prompt injection

A malicious user could craft a `prompt` or `current_css` value containing text that attempts to override the system prompt (e.g., `"Ignore all previous instructions and output <script>...`). Mitigations:

- The system prompt is sent as the `system` role message, which Ollama processes separately from the user turn. This provides structural separation rather than relying on in-band delimiters.
- Output from the model is treated as raw CSS text, not HTML or JavaScript. It is never rendered as markup — it is written to a `<textarea>` value and eventually into a `<style>` tag via the existing theme CSS pipeline.
- The existing CSS URL validation (`internal/theme/service.go`) is **not** applied to AI-generated CSS before it is sent to the client. Generated CSS lands in the textarea; the user must click **Save** to persist it, at which point normal URL validation runs. This is the correct flow — generated CSS is treated as user-typed CSS from the validation perspective.
- Max prompt length is 500 chars; max `current_css` is 50 KB. These limits reduce the surface area for injection attacks.

### CSS injection via AI output

The model could theoretically output CSS that exfiltrates data via attribute-selector oracles or loads external resources. Mitigations:

- CSS URL validation runs on **Save**, not on generate. Generated CSS is placed in the textarea and visible to the user before saving.
- The CSS URL allowlist (Google Fonts + own CDN) is enforced on Save. If the model generates CSS with disallowed URLs (e.g., a `url()` pointing to an attacker-controlled server), the Save will be rejected with a clear error listing the offending URLs.
- This is the same risk posture as user-typed CSS, which is already accepted as an explicit trade-off in the theming spec (Section 10 of `2026-03-16-theming-design.md`).

### SSRF via `OLLAMA_API_URL`

The `OLLAMA_API_URL` is a server-side environment variable, not user-supplied. No SSRF vector exists from client input.

### Ollama API key exposure

`OLLAMA_API_KEY` is:
- Marked `sensitive = true` in Terraform (excluded from plan output)
- Written to `/etc/parley/env` with mode `0600` (existing pattern for all secrets)
- Never returned in any API response
- Not logged

### Per-user abuse

A single user cannot submit more than one in-flight generate request at a time because the SSE connection blocks until the result arrives. Multiple users can enqueue simultaneously; each waits their turn. No additional per-user rate limiting is added in v1.

---

## 10. Self-Review Findings and Fixes

This section documents issues found during the self-review of this spec, and how each was addressed.

### Finding 1: Missing `ai_models.go` file

**Issue:** The original design listed only `ai_queue.go`, `ai_worker.go`, and `ai_handler.go`. The `AIGenJob` and `AIGenResult` types would need to be defined in one of those files, creating a dependency ordering problem — `ai_handler.go` and `ai_worker.go` both need both types, but `ai_queue.go` only needs `AIGenJob`. Putting all types in `ai_queue.go` creates an implicit dependency chain that is easy to break.

**Fix:** Added `internal/theme/ai_models.go` to the files-to-update table (Section 7) as the canonical home for both types. This is consistent with how `models.go` is already used in `internal/theme/`.

### Finding 2: Race condition — `LPOP` without holding the lock

**Issue:** The original design described the lock and pop as separate steps without clarifying their ordering. If two nodes both check the lock, both see it as unacquired, and both call `SET NX` in rapid succession, only one succeeds — but the critical-section description did not explicitly state that `LPOP` must occur *while* the lock is held, not after releasing it.

**Fix:** Section 3.2 now explicitly states: "The lock must be acquired *before* `LPOP`, not after." The worker pseudocode in Section 3.3 shows the correct ordering: acquire → pop → call Ollama → publish → release. `LPOP` atomicity in Redis means only one caller receives a job regardless, but the lock ensures one caller even attempts the pop.

### Finding 3: SSE connection and `WriteTimeout`

**Issue:** `cmd/api/main.go` sets `WriteTimeout: 15 * time.Second` on the `http.Server`. SSE connections that wait in queue for more than 15 s will be silently dropped by the server, with no error event sent to the client.

**Fix:** Section 3.4 now documents this explicitly and prescribes the fix: call `http.NewResponseController(w).SetWriteDeadline(time.Time{})` at the start of the `Generate` handler to clear the write deadline for that connection. This is the idiomatic Go 1.20+ approach and does not affect the timeout of other requests.

### Finding 4: `EventSource` cannot send `Authorization` header

**Issue:** The original design said "SSE connection opened on button click" without specifying the mechanism. The browser `EventSource` API does not support custom headers, making it incompatible with the existing Bearer token auth scheme.

**Fix:** Section 6 now explicitly states that `EventSource` cannot be used and prescribes a `fetch()` + `ReadableStream` reader instead, using the same `getAuthHeaders()` pattern as other API calls. Sample code is provided.

### Finding 5: Client disconnect — orphaned jobs

**Issue:** When a client disconnects mid-queue (closes tab, navigates away), the job remains in the Redis queue and will be processed by the worker. The result is published to Redis and expires after 60 s with no consumer. This is harmless but wastes one Ollama API call per orphan.

**Fix:** This is documented in Section 3.4 as an accepted trade-off for v1. A future improvement could add a "cancel" mechanism (e.g., a separate `DELETE /api/me/themes/generate/:jobID` endpoint that removes the job from the sorted set and list), but this adds significant complexity for a low-frequency operation.

### Finding 6: CSS injection — validation not applied to generated output before display

**Issue:** The original design did not clarify when CSS URL validation runs relative to the generate flow. A reader could infer that validation should happen on the generated output before it is sent to the client.

**Fix:** Section 9 explicitly documents the decision: generated CSS is placed in the textarea (equivalent to user-typed CSS) and validation runs on Save. This is the correct behavior — it would be confusing to block display of generated CSS when the same CSS typed manually would be displayable.

### Finding 7: `registerRoutes` lacks a context for worker lifecycle

**Issue:** `StartAIWorker` takes a `context.Context` for graceful shutdown, but `registerRoutes` does not currently receive one (it is called from `main()` without passing the shutdown context).

**Fix:** Section 5 documents this explicitly and prescribes using `context.Background()` for v1, with a note that the server's 30 s shutdown grace period covers in-flight requests. Threading the shutdown context through is noted as a follow-up improvement.

---

## 11. Open Questions

1. **Model selection:** Is `qwen3.5:9b` available on the Ollama cloud API, or does the default need to be a different model? Verify against the Ollama cloud API model catalog before implementation.
2. **Queue depth limit:** Should there be a maximum queue depth (e.g., reject new jobs when >20 are queued) to prevent a backlog from accumulating? Not implemented in v1; revisit if queue depth becomes an issue.
3. **Worker count:** One worker goroutine per API node. With 3 API nodes, there are 3 goroutines competing for the same lock. This is correct and safe, but if the cluster scales to many nodes the lock contention polling (every 500 ms per node) could add Redis load. The 500 ms interval is low-frequency enough that this is not a concern at current scale.
