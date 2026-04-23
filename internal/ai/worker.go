package ai

import (
	"context"
	"log"
	"strings"
	"time"

	"parley/internal/theme/cssvalidator"
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

IMPORTANT contrast rules:
  --parley-bg-tertiary and --parley-text-muted (--text-secondary) must have sufficient contrast so that
  text is readable on that background — context menus and dropdowns use this combination.
  Aim for at least a 4:1 contrast ratio between these two values.
  --parley-accent is used as a button background with white (#fff) text on top. It MUST be dark enough
  that white text is legible — never use yellow, light green, light pink, or any light/pastel color as
  the accent. If the theme calls for a light accent, darken it significantly for this variable.

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

Background image & frosted glass (optional — only set these when the user wants a background image effect):
  --parley-app-bg              background of .main-layout and .main-content — set to transparent to reveal body background image
  --parley-panel-bg            background of .sidebar, .channel-list, .user-sidebar, .dm-panel, .vc-chat-sidebar — use rgba() for transparency
  --parley-panel-blur          backdrop-filter blur on panels — e.g. 14px for frosted glass, 0px for clear
  --parley-panel-header-bg     background of .server-header (top of channel list) — use rgba() matching panel tint
  --parley-panel-footer-bg     background of .user-area and .dm-panel-user-area (user strip at bottom) — use rgba() matching panel tint
  --parley-chat-bg             background of chat area (.chat-container, .chat-window, .chat-header, .message-input-container)
  --parley-input-bg            background of the message input box — use rgba() matching the chat tint (falls back to --parley-input solid color)

To create a frosted glass effect with a background image:
  1. Set body { background-image: url("..."); background-size: cover; background-attachment: fixed; background-repeat: no-repeat; }
  2. Set --parley-app-bg: transparent
  3. Set --parley-panel-bg: rgba(R, G, B, 0.6) using the theme's secondary bg color
  4. Set --parley-panel-blur: 14px
  5. Set --parley-panel-header-bg and --parley-panel-footer-bg to rgba() variants slightly more opaque than the panel
  6. Set --parley-chat-bg: rgba(R, G, B, 0.78) using the theme's channel-bg color
  7. Set --parley-input-bg: rgba(R, G, B, 0.55) using the theme's panel color (slightly less opaque than chat bg)

CRITICAL: When using frosted glass, do NOT redeclare --parley-app-bg, --parley-panel-bg, --parley-panel-blur, --parley-panel-header-bg, --parley-panel-footer-bg, --parley-chat-bg, or --parley-input-bg anywhere else in [data-theme]. Declaring them twice (once as rgba and once as a solid color) will cause the solid color to override the glass effect. Set each of these variables exactly once.

You may use Google Fonts via @import at the top: @import url('https://fonts.googleapis.com/css2?family=...');
No other external URLs are permitted.
If you use a Google Font, also set font-family on [data-theme].
Do NOT use display or decorative fonts that render in all-caps (e.g. Bebas Neue, Russo One, Anton). Only use fonts that have proper mixed-case rendering.

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

// validateGeneratedCSS performs sanity checks on model output. It rejects
// obvious framing mistakes (empty output, fenced markdown, unbalanced braces)
// and runs the same URL allow-list check used on Save, so a prompt-injected
// @import or url() from the model doesn't reach the user. cdnHost is left
// empty: only the static font allow-list is honored here, and Save re-validates
// with the real cdnHost before persistence.
func validateGeneratedCSS(css string) bool {
	s := strings.TrimSpace(css)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "```") {
		return false
	}
	open := strings.Count(s, "{")
	close := strings.Count(s, "}")
	if open == 0 || open != close {
		return false
	}
	if err := cssvalidator.Validate(css, ""); err != nil {
		log.Printf("ai worker: generated CSS rejected: %v", err)
		return false
	}
	return true
}
