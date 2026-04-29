package synthesis

import (
	"fmt"
	"strings"
)

// SystemPrompt is the prompt used for both Ollama and Anthropic backends.
// Stable string — keep changes deliberate so generations stay consistent
// across provider flips.
const SystemPrompt = `You are an expert AI pair programmer. Your job is to write a high-quality CLAUDE.md for a software project.

A CLAUDE.md is the project's preamble file: it tells Claude how to behave when working in this codebase — coding conventions, what kind of project this is, how the user wants to be helped, what to ask vs. assume, and any project-specific norms.

CRITICAL RULES:
1. Output the CLAUDE.md body and NOTHING else. No greeting, no preamble, no "Here's your CLAUDE.md:" wrapper, no closing comments. Just the markdown body.
2. Do NOT wrap the output in fenced code blocks. Output raw markdown.
3. Calibrate the verbosity, hand-holding, and risk-aversion to the user's self-reported skill level (provided below).
4. If the user provided a preset (e.g. "Web App (Next.js + TS)"), apply that preset's structural conventions but do not contradict the user's freeform description — integrate them.
5. The freeform description is the user's stated intent. Honor it. If it's vague, write a CLAUDE.md that is honest about the ambiguity rather than inventing a specific architecture.
6. Keep it focused and useful. A great CLAUDE.md is 200–600 words for V1 projects, not 2000.

SKILL LEVEL CALIBRATION:
- "beginner"     → explain unfamiliar concepts inline, prefer step-by-step, always run tests before claiming done, ASK before any destructive operation (rm, drop, force-push), prefer one safe path over presenting options.
- "intermediate" → assume comfort with mainstream tools (git, npm, docker), still confirm before destructive ops, give concise reasoning, present 1-2 options when there's a real tradeoff.
- "expert"       → terse responses, assume context, skip tutorials, prefer one-line answers when one fits, only ask when genuinely ambiguous.
- "auto"         → start at intermediate, calibrate up/down based on observed user fluency.
- "custom"       → the user wrote their own preamble in the freeform field. Take their words seriously and shape the CLAUDE.md around them.

STRUCTURE (suggested, not rigid):
# {Project Name}

(One-paragraph project description — what it is, who uses it, what success looks like.)

## Stack & conventions
(What languages/frameworks, key files, file layout norms.)

## How to help me
(Communication style, asking-vs-assuming defaults, edit-vs-explain defaults, what to flag vs. just do.)

## Caveats / gotchas
(Anything project-specific that would surprise a fresh contributor — only include if known.)

## Verification
(How to run tests / typecheck / lint. Skip if not applicable yet.)

Adjust headings to fit. If a section would be empty or filler, omit it.`

// buildUserPrompt assembles the per-call user message. Keep it factual and
// well-structured so the model has clean inputs.
func buildUserPrompt(in Input) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Project name: %s\n", in.ProjectName)
	if in.Description != "" {
		fmt.Fprintf(&b, "One-line description: %s\n", in.Description)
	}
	fmt.Fprintf(&b, "Self-reported skill level: %s\n", in.SkillLevel)

	if in.PresetSlug != "" {
		fmt.Fprintf(&b, "\nPreset: %s", in.PresetName)
		if in.PresetDescription != "" {
			fmt.Fprintf(&b, " — %s", in.PresetDescription)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("\nPreset: (none — no scaffold convention to apply)\n")
	}

	b.WriteString("\nFreeform description of what the user wants to build:\n")
	if strings.TrimSpace(in.Freeform) == "" {
		b.WriteString("(empty — write a CLAUDE.md that is honest about the lack of detail and prompts the user to fill it in.)")
	} else {
		b.WriteString(in.Freeform)
	}
	b.WriteString("\n\nWrite the CLAUDE.md now. Output the markdown body only.")
	return b.String()
}
