package ai

import (
	"context"
	"log"
	"strings"
	"time"
)

// systemPrompt is sent as the system message to every generation request.
const systemPrompt = `You are a CSS theme generator for Parley, a Discord-like chat application.
Generate a CSS theme by setting ALL of the custom properties listed below on [data-theme] { }.
You MUST include every variable — missing variables will leave parts of the UI unstyled.

REQUIRED variables (set all of them):

Background layers (darkest to lightest for dark themes, reverse for light):
  --parley-bg              main chat area background
  --parley-bg-primary      same as --parley-bg (alias)
  --parley-bg-secondary    sidebar panels, popover backgrounds
  --parley-bg-tertiary     input areas, deeply nested panels
  --parley-bg-hover        subtle hover highlight on list items
  --parley-hover           same as --parley-bg-hover (alias)
  --parley-dark            darkest shade, used for deep shadows
  --parley-gray            mid-tone gray for muted elements
  --parley-sidebar         left server+channel sidebar background
  --parley-channel-bg      channel list area background
  --parley-input           message input box background

Text:
  --parley-text            primary message/body text
  --parley-text-normal     same as --parley-text (alias)
  --parley-text-muted      secondary/dimmed text, timestamps
  --parley-text-dim        even more dimmed, placeholder text
  --parley-text-link       hyperlink colour

Accent / interactive:
  --parley-accent          buttons, active states, highlighted items
  --parley-accent-hover    hover state for accent elements
  --parley-blurple         brand accent (can match --parley-accent)

Borders:
  --parley-border          main dividers and borders
  --parley-border-light    lighter variant border

Status / semantic:
  --parley-danger          destructive actions, error states
  --parley-danger-hover    hover for danger
  --parley-success         positive/success states
  --parley-green           online indicator, success (can match --parley-success)
  --parley-yellow          warning states
  --parley-red             alert / badge colour (can match --parley-danger)

RGB values (used for rgba() opacity effects — provide as "R, G, B" with no extra text):
  --accent-rgb             R, G, B of --parley-accent
  --parley-danger-rgb      R, G, B of --parley-danger

Layout:
  --sidebar-width          232px  (keep this value)

Non-prefixed aliases used by some components (mirror the --parley-* values):
  --bg-primary             same as --parley-bg
  --bg-secondary           same as --parley-bg-secondary
  --bg-tertiary            same as --parley-bg-tertiary
  --bg-hover               same as --parley-bg-hover
  --text-primary           same as --parley-text
  --text-secondary         same as --parley-text-muted
  --text-muted             same as --parley-text-muted
  --accent                 same as --parley-accent
  --accent-hover           same as --parley-accent-hover
  --border                 same as --parley-border

You may use Google Fonts via @import at the top: @import url('https://fonts.googleapis.com/css2?family=...');
No other external URLs are permitted.
If you use a Google Font, also set font-family on [data-theme].

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
