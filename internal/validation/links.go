package validation

import (
	"regexp"
	"strings"
)

// UsernameRe is the allowed character set for usernames: letters, digits,
// underscores, hyphens, and dots; 1–32 characters.
var UsernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{1,32}$`)

// ValidUsername reports whether s is a valid username.
func ValidUsername(s string) bool {
	return UsernameRe.MatchString(s)
}

// mdLinkRe matches markdown links: [display text](url)
var mdLinkRe = regexp.MustCompile(`\[([^\]\n]{1,500})\]\(([^)\n]+)\)`)

// urlLikeRe detects display text that looks like a URL or URL fragment
var urlLikeRe = regexp.MustCompile(`(?i)(?:https?://|ftp://|www\.|[a-z0-9][\w\-]*\.[a-z]{2,6}(?:[/?#\s]|$))`)

// extractHostname normalizes a string to just its hostname for comparison.
func extractHostname(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexAny(s, "/?# \t\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// HasSpoofedLink returns true if the content contains a markdown link where
// the display text visually resembles a different URL than the actual href.
// This blocks attacks like [http](evil.com)[://](evil.com)[goodguy.com](evil.com)
// which renders as "http://goodguy.com" but all three link to evil.com.
func HasSpoofedLink(content string) bool {
	for _, m := range mdLinkRe.FindAllStringSubmatch(content, -1) {
		display := strings.TrimSpace(m[1])
		href := strings.TrimSpace(m[2])
		if !urlLikeRe.MatchString(display) {
			continue
		}
		displayHost := extractHostname(display)
		hrefHost := extractHostname(href)
		// Allow pure-protocol display text like "https://" (empty host after extraction)
		if displayHost == "" {
			continue
		}
		if displayHost != hrefHost {
			return true
		}
	}
	return false
}
