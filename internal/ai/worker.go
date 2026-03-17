package ai

import (
	"context"
	"log"
	"strings"
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

You may use Google Fonts via @import at the top: @import url('https://fonts.googleapis.com/css2?family=...');
No other external URLs are permitted.

Output ONLY raw CSS. No markdown code fences. No explanation. No comments.
Start immediately with [data-theme] { (or a Google Fonts @import followed by [data-theme] {) and end with }.`

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
	} else if !validateGeneratedCSS(css) {
		// First attempt produced invalid CSS — retry once with a clarifying prompt.
		log.Printf("ai worker: job %s produced invalid CSS, retrying", job.JobID)
		retryMsg := "Your previous response was not valid CSS. " +
			"Output ONLY raw CSS starting with [data-theme] { and ending with }. " +
			"No markdown code fences, no explanations, no comments.\n\n" +
			"Original request: " + job.UserMessage
		css2, retryErr := ollama.Generate(ctx, systemPrompt, retryMsg)
		if retryErr != nil {
			result = AIGenResult{Error: "AI generation failed on retry: " + retryErr.Error()}
		} else if !validateGeneratedCSS(css2) {
			result = AIGenResult{Error: "AI returned invalid CSS after retry. Please try rephrasing your prompt."}
		} else {
			result = AIGenResult{CSS: css2}
		}
	} else {
		result = AIGenResult{CSS: css}
	}

	if pubErr := queue.PublishResult(ctx, job.JobID, result); pubErr != nil {
		log.Printf("ai worker: publish result for %s: %v", job.JobID, pubErr)
	}

	return queue.ReleaseLock(ctx)
}

// validateGeneratedCSS performs simple sanity checks on model output.
// It does NOT check URL allowlists — that runs on Save in the theme service.
func validateGeneratedCSS(css string) bool {
	s := strings.TrimSpace(css)
	if s == "" {
		return false
	}
	// Reject markdown code fences the model accidentally included.
	if strings.HasPrefix(s, "```") {
		return false
	}
	// Must contain at least one rule block.
	open := strings.Count(s, "{")
	close := strings.Count(s, "}")
	return open > 0 && open == close
}
