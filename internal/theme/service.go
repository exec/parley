package theme

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const maxCSSBytes = 51200
const maxThemes = 20

var ErrThemeLimit = fmt.Errorf("theme limit reached (%d max)", maxThemes)

var urlRe = regexp.MustCompile(`(?i)url\(\s*['"]?([^'"\s)]+)['"]?\s*\)`)

type ValidationError struct {
	OffendingURLs []string `json:"offending_urls"`
}

func (e *ValidationError) Error() string { b, _ := json.Marshal(e); return string(b) }

type Service struct {
	repo    *Repository
	cdnHost string
	siteURL string
}

func NewService(repo *Repository, cdnHost, siteURL string) *Service {
	return &Service{repo: repo, cdnHost: cdnHost, siteURL: siteURL}
}

var staticAllowed = map[string]bool{
	"fonts.googleapis.com": true,
	"fonts.gstatic.com":    true,
}

func (s *Service) validateCSS(css string) error {
	if len(css) > maxCSSBytes {
		return fmt.Errorf("CSS exceeds 50 KB limit")
	}
	matches := urlRe.FindAllStringSubmatch(css, -1)
	var bad []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		raw := strings.TrimSpace(m[1])
		if strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "#") || !strings.Contains(raw, "://") {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			continue
		}
		h := strings.ToLower(parsed.Host)
		if !staticAllowed[h] && h != strings.ToLower(s.cdnHost) {
			bad = append(bad, raw)
		}
	}
	if len(bad) > 0 {
		return &ValidationError{OffendingURLs: bad}
	}
	return nil
}

func (s *Service) GetPreferences(ctx context.Context, userID int64) (*UserPreferences, error) {
	return s.repo.GetPreferences(ctx, userID)
}

func (s *Service) SetActiveTheme(ctx context.Context, userID int64, theme string, customThemeID *int) error {
	return s.repo.SetActiveTheme(ctx, userID, theme, customThemeID)
}

func (s *Service) CreateTheme(ctx context.Context, userID int64, name, css string, bgURL *string) (*UserTheme, error) {
	if err := s.validateCSS(css); err != nil {
		return nil, err
	}
	n, err := s.repo.CountUserThemes(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= maxThemes {
		return nil, ErrThemeLimit
	}
	return s.repo.CreateTheme(ctx, userID, name, css, bgURL)
}

func (s *Service) UpdateTheme(ctx context.Context, id, userID int64, name, css string, bgURL *string) (*UserTheme, error) {
	if err := s.validateCSS(css); err != nil {
		return nil, err
	}
	return s.repo.UpdateTheme(ctx, id, userID, name, css, bgURL)
}

func (s *Service) DeleteTheme(ctx context.Context, id, userID int64) error {
	return s.repo.DeleteTheme(ctx, id, userID)
}

func (s *Service) ShareTheme(ctx context.Context, id, userID int64) (string, error) {
	token, err := s.repo.GenerateShareToken(ctx, id, userID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/theme/%s", s.siteURL, token), nil
}

func (s *Service) GetPublicTheme(ctx context.Context, token string) (*UserTheme, error) {
	return s.repo.GetThemeByToken(ctx, token)
}

func (s *Service) InstallTheme(ctx context.Context, token string, userID int64) (*UserTheme, error) {
	src, err := s.repo.GetThemeByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := s.validateCSS(src.CSS); err != nil {
		return nil, err
	}
	n, err := s.repo.CountUserThemes(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= maxThemes {
		return nil, ErrThemeLimit
	}
	return s.repo.InstallTheme(ctx, token, userID)
}
