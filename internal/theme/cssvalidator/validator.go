// Package cssvalidator enforces the URL allow-list on user-supplied CSS.
// It lives in its own package so both internal/theme (Save path) and
// internal/ai (worker defense-in-depth) can call it without a cycle —
// internal/theme already imports internal/ai for AIQueue.
package cssvalidator

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const MaxCSSBytes = 51200

var (
	urlRe       = regexp.MustCompile(`(?i)url\(\s*['"]?([^'"\s)]+)['"]?\s*\)`)
	importRe    = regexp.MustCompile(`(?i)@import\s+['"]\s*([^'"]+?)\s*['"]`)
	cssEscapeRe = regexp.MustCompile(`\\[0-9a-fA-F]`)
)

var staticAllowed = map[string]bool{
	"fonts.googleapis.com": true,
	"fonts.gstatic.com":    true,
}

// ValidationError lists URLs that referenced hosts outside the allow-list.
type ValidationError struct {
	OffendingURLs []string `json:"offending_urls"`
}

func (e *ValidationError) Error() string { b, _ := json.Marshal(e); return string(b) }

// IsValidationError reports whether err is a *ValidationError.
func IsValidationError(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// Validate checks CSS against the URL allow-list. It scans both url(...) tokens
// and bare-string @import forms — either can reference an external host, and
// both must be pinned to the allow-list or the configured cdnHost.
func Validate(css, cdnHost string) error {
	if len(css) > MaxCSSBytes {
		return fmt.Errorf("CSS exceeds 50 KB limit")
	}
	var bad []string
	for _, m := range urlRe.FindAllStringSubmatch(css, -1) {
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
		if !hostAllowed(parsed.Host, cdnHost) {
			bad = append(bad, raw)
		}
	}
	for _, m := range importRe.FindAllStringSubmatch(css, -1) {
		if len(m) < 2 {
			continue
		}
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}
		// Protocol-relative — external fetch with page's scheme.
		if strings.HasPrefix(raw, "//") {
			bad = append(bad, raw)
			continue
		}
		// CSS hex escapes (e.g. \68\74\74\70 = "http") can smuggle schemes past
		// a naive parser; rather than attempt to un-escape correctly, reject.
		if cssEscapeRe.MatchString(raw) {
			bad = append(bad, raw)
			continue
		}
		// Relative same-origin imports can't beacon an external host.
		if !strings.Contains(raw, "://") {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			bad = append(bad, raw)
			continue
		}
		if !hostAllowed(parsed.Host, cdnHost) {
			bad = append(bad, raw)
		}
	}
	if len(bad) > 0 {
		return &ValidationError{OffendingURLs: bad}
	}
	return nil
}

func hostAllowed(host, cdnHost string) bool {
	h := strings.ToLower(host)
	return staticAllowed[h] || h == strings.ToLower(cdnHost)
}
