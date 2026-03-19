package theme

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"parley/internal/ai"
	"parley/internal/httputil"
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
		httputil.JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = uid // authenticated; used for future per-user rate limiting

	// Disable the server-level 15s write deadline so SSE can stream indefinitely.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		httputil.InternalError(w, err)
		return
	}

	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Prompt) == 0 {
		httputil.JSONError(w, "prompt is required", http.StatusBadRequest)
		return
	}
	if len(req.Prompt) > 500 {
		req.Prompt = req.Prompt[:500]
	}

	// Build the user message that the worker will send to Ollama.
	userMessage := req.Prompt
	if req.CurrentCSS != "" {
		userMessage = "Here is my current theme CSS. Modify it according to the instructions below — " +
			"you may change any values, add or remove variables, or rewrite it entirely as needed.\n\n" +
			req.CurrentCSS + "\n\nInstructions: " + req.Prompt
	}

	jobID := uuid.New().String()
	job := ai.AIGenJob{
		JobID:       jobID,
		UserMessage: userMessage,
	}

	// Enqueue the job.
	position, err := h.queue.Enqueue(r.Context(), job)
	if err != nil {
		httputil.InternalError(w, err)
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
