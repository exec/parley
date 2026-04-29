package synthesis

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubProvider lets us assert how Service shapes Input before delegating.
type stubProvider struct {
	gotInput Input
	out      string
	err      error
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) SynthesizeClaudeMD(_ context.Context, in Input) (string, error) {
	s.gotInput = in
	return s.out, s.err
}

func TestService_NilProviderReturnsErrUnavailable(t *testing.T) {
	svc := NewService(nil)
	_, err := svc.SynthesizeClaudeMD(t.Context(), Input{ProjectName: "x"})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("err = %v, want ErrProviderUnavailable", err)
	}
	if svc.ProviderName() != "" {
		t.Errorf("ProviderName() = %q, want \"\"", svc.ProviderName())
	}
}

func TestService_TrimsAndDefaultsSkillLevel(t *testing.T) {
	stub := &stubProvider{out: "ok"}
	svc := NewService(stub)

	_, err := svc.SynthesizeClaudeMD(t.Context(), Input{
		ProjectName: "  My Project  ",
		Description: "  one liner  ",
		Freeform:    "\n\n   build a thing   \n",
		SkillLevel:  "",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if stub.gotInput.ProjectName != "My Project" {
		t.Errorf("ProjectName not trimmed: %q", stub.gotInput.ProjectName)
	}
	if stub.gotInput.Description != "one liner" {
		t.Errorf("Description not trimmed: %q", stub.gotInput.Description)
	}
	if stub.gotInput.Freeform != "build a thing" {
		t.Errorf("Freeform not trimmed: %q", stub.gotInput.Freeform)
	}
	if stub.gotInput.SkillLevel != "auto" {
		t.Errorf("SkillLevel default not applied: %q", stub.gotInput.SkillLevel)
	}
}

func TestService_PassesProviderError(t *testing.T) {
	want := errors.New("boom")
	svc := NewService(&stubProvider{err: want})
	_, err := svc.SynthesizeClaudeMD(t.Context(), Input{ProjectName: "x"})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestService_ProviderName(t *testing.T) {
	svc := NewService(&stubProvider{})
	if got := svc.ProviderName(); got != "stub" {
		t.Errorf("ProviderName() = %q, want \"stub\"", got)
	}
}

// cleanFences should strip leading ```lang and trailing ``` defensively.
func TestCleanFences(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"# Title\n\nbody", "# Title\n\nbody"},
		{"```markdown\n# Title\nbody\n```", "# Title\nbody"},
		{"```\n# Title\nbody\n```", "# Title\nbody"},
		{"   ```md\n# T\nb\n```   ", "# T\nb"},
		{"```", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := cleanFences(c.in)
		if got != c.want {
			t.Errorf("cleanFences(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// buildUserPrompt should always include project name + skill level and
// honor preset/freeform presence vs absence.
func TestBuildUserPrompt(t *testing.T) {
	got := buildUserPrompt(Input{
		ProjectName: "Demo",
		SkillLevel:  "expert",
		PresetSlug:  "web-app",
		PresetName:  "Web App (Next.js + TS)",
		Freeform:    "a todo list",
	})
	if !strings.Contains(got, "Demo") {
		t.Errorf("prompt missing project name: %s", got)
	}
	if !strings.Contains(got, "expert") {
		t.Errorf("prompt missing skill level: %s", got)
	}
	if !strings.Contains(got, "Web App (Next.js + TS)") {
		t.Errorf("prompt missing preset name: %s", got)
	}
	if !strings.Contains(got, "a todo list") {
		t.Errorf("prompt missing freeform body: %s", got)
	}

	got2 := buildUserPrompt(Input{ProjectName: "X", SkillLevel: "auto"})
	if !strings.Contains(got2, "(none — no scaffold convention to apply)") {
		t.Errorf("missing none-preset signal: %s", got2)
	}
	if !strings.Contains(got2, "(empty — write a CLAUDE.md") {
		t.Errorf("missing empty-freeform signal: %s", got2)
	}
}

func TestProviderInterface_OllamaImpl(t *testing.T) {
	// Compile-time check: OllamaProvider satisfies Provider.
	var _ Provider = (*OllamaProvider)(nil)
}

func TestProviderInterface_AnthropicImpl(t *testing.T) {
	var _ Provider = (*AnthropicProvider)(nil)
}

func TestAnthropicProvider_NoAPIKeyErrors(t *testing.T) {
	p := NewAnthropicProvider("", "claude-sonnet-4-6")
	_, err := p.SynthesizeClaudeMD(t.Context(), Input{ProjectName: "x"})
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected API-key error, got %v", err)
	}
}
