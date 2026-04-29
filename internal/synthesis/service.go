package synthesis

import (
	"context"
	"strings"
)

// Service is the synthesis API surface used by the rest of parley. It
// holds a single Provider; callers don't know which one is wired (Ollama
// vs. Anthropic). Swap in cmd/api/main.go when the provider flip happens.
type Service struct {
	provider Provider
}

// NewService builds a Service around the given provider. Pass nil to make
// SynthesizeClaudeMD return ErrProviderUnavailable — useful in tests or if
// the API is starting up without a provider configured (e.g. Ollama URL
// missing). The handler maps that to 503.
func NewService(p Provider) *Service {
	return &Service{provider: p}
}

// SynthesizeClaudeMD delegates to the wrapped provider with light input
// hygiene. The handler is expected to validate skill_level / preset_slug
// upstream; this layer is the LLM-call boundary.
func (s *Service) SynthesizeClaudeMD(ctx context.Context, in Input) (string, error) {
	if s.provider == nil {
		return "", ErrProviderUnavailable
	}
	in.ProjectName = strings.TrimSpace(in.ProjectName)
	in.Description = strings.TrimSpace(in.Description)
	in.Freeform = strings.TrimSpace(in.Freeform)
	if in.SkillLevel == "" {
		in.SkillLevel = "auto"
	}
	return s.provider.SynthesizeClaudeMD(ctx, in)
}

// ProviderName returns the wrapped provider's identifier or "" if none.
// Used in handler responses + telemetry.
func (s *Service) ProviderName() string {
	if s.provider == nil {
		return ""
	}
	return s.provider.Name()
}
