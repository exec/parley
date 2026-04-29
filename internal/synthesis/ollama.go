package synthesis

import (
	"context"
	"strings"

	"parley/internal/ai"
)

// OllamaProvider wraps the existing parley ai.OllamaClient with the
// synthesis-specific system prompt + post-processing.
type OllamaProvider struct {
	client *ai.OllamaClient
}

// NewOllamaProvider builds a provider that hits the parley Ollama instance.
// url, key, and model match cmd/api Config OllamaAPIURL/Key/Model.
func NewOllamaProvider(url, key, model string) *OllamaProvider {
	return &OllamaProvider{client: ai.NewOllamaClient(url, key, model)}
}

func (p *OllamaProvider) Name() string { return "ollama" }

// SynthesizeClaudeMD calls the Ollama chat API and trims the result.
// The system prompt instructs the model not to wrap output in fences;
// we still strip a leading ```markdown / trailing ``` defensively in case
// the model ignores that instruction.
func (p *OllamaProvider) SynthesizeClaudeMD(ctx context.Context, in Input) (string, error) {
	out, err := p.client.Generate(ctx, SystemPrompt, buildUserPrompt(in))
	if err != nil {
		return "", err
	}
	return cleanFences(out), nil
}

// cleanFences strips a leading triple-backtick fence (with optional language
// tag) and the matching trailing fence. Idempotent: if no fence is present
// the input is returned unchanged.
func cleanFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	rest := s[3:]
	// Skip the optional language tag up to the first newline.
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		// No body at all — input was just an opening fence (maybe ```lang).
		// Fall through so the trailing TrimSuffix can handle a """``` ``` """ pair too.
		rest = ""
	}
	rest = strings.TrimRight(rest, " \t\n")
	rest = strings.TrimSuffix(rest, "```")
	return strings.TrimSpace(rest)
}
