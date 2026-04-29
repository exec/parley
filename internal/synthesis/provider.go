// Package synthesis turns user intent (preset + skill + freeform) into a
// CLAUDE.md tuned for that project.
//
// V1 (Phase A.A2) ships an Ollama-backed Provider; the Anthropic Provider
// is compiled and tested alongside it but is intentionally not wired into
// the live `Service` until funding/quality demands it. See spec §5.2.
package synthesis

import (
	"context"
	"errors"
)

// Input is the synthesis call payload assembled by the project handler.
//
// PresetName/PresetDescription are pre-resolved (not just slugs) so the
// system prompt can quote the human-readable label. Description is the
// optional one-liner the user typed when picking the project name;
// Freeform is the longer "describe what you want to build" body.
type Input struct {
	PresetSlug         string
	PresetName         string
	PresetDescription  string
	SkillLevel         string
	ProjectName        string
	Description        string
	Freeform           string
}

// Provider is the synthesis backend interface. Implementations must be
// safe for concurrent use by multiple goroutines.
type Provider interface {
	// SynthesizeClaudeMD returns the generated CLAUDE.md body (markdown).
	// The output should NOT be fenced — it is stored verbatim into the
	// project's claude_md column and rendered as markdown by the client.
	SynthesizeClaudeMD(ctx context.Context, in Input) (string, error)

	// Name identifies the provider for log/telemetry purposes.
	// Stable string, lowercase, e.g. "ollama" / "anthropic".
	Name() string
}

// ErrProviderUnavailable is returned by Service.SynthesizeClaudeMD if no
// provider is configured. Treated as 503 by the handler.
var ErrProviderUnavailable = errors.New("synthesis provider unavailable")
