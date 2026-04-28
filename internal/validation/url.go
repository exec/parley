package validation

import (
	"errors"
	"net/url"
	"strings"
)

// maxMediaURLLen caps incoming media URLs. Anything longer is almost
// certainly an attempt to abuse audit-log / DB storage rather than a real
// CDN reference (CDN paths are well under 2KB).
const maxMediaURLLen = 2000

// MediaURL ensures rawURL is either empty or an https URL on the configured
// CDN host. This prevents SSRF via arbitrary avatar/banner/icon URLs and
// caps the input length to keep oversized payloads out of the database.
func MediaURL(rawURL, cdnHost string) error {
	if rawURL == "" {
		return nil
	}
	if len(rawURL) > maxMediaURLLen {
		return errors.New("media URL is too long")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL")
	}
	if u.Scheme != "https" {
		return errors.New("media URLs must use https")
	}
	if cdnHost != "" && !strings.EqualFold(u.Host, cdnHost) {
		return errors.New("media URLs must be hosted on the CDN")
	}
	return nil
}
