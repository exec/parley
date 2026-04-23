package theme

import (
	"errors"
	"testing"
)

// validateCSS is a method on *Service that only reads s.cdnHost — no repo
// access — so we can construct a bare Service to exercise it without a DB.
func newValidator(cdn string) *Service {
	return &Service{cdnHost: cdn}
}

func TestValidateCSS_URLAndImportForms(t *testing.T) {
	tests := []struct {
		name      string
		css       string
		wantOK    bool
		wantBadIn string // substring expected in OffendingURLs when !wantOK
	}{
		{
			name:      "url() external rejected (baseline)",
			css:       `body { background: url("https://evil.tld/x.css"); }`,
			wantOK:    false,
			wantBadIn: "evil.tld",
		},
		{
			name:      "@import bare string rejected",
			css:       `@import "https://evil.tld/x.css";`,
			wantOK:    false,
			wantBadIn: "evil.tld",
		},
		{
			name:      "@import protocol-relative rejected",
			css:       `@import "//evil.tld/x.css";`,
			wantOK:    false,
			wantBadIn: "evil.tld",
		},
		{
			name:      "@import hex-escaped scheme rejected",
			css:       `@import '\68\74\74\70s://evil.tld/x.css';`,
			wantOK:    false,
			wantBadIn: `\68`,
		},
		{
			name:   "@import google fonts bare string accepted",
			css:    `@import "https://fonts.googleapis.com/css2?family=Foo";`,
			wantOK: true,
		},
		{
			name:   "@import google fonts via url() accepted",
			css:    `@import url("https://fonts.googleapis.com/css2?family=Foo");`,
			wantOK: true,
		},
		{
			name:   "data URI still allowed",
			css:    `body { background: url(data:image/svg+xml;base64,PHN2Zy8+) }`,
			wantOK: true,
		},
	}

	s := newValidator("cdn.example.com")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.validateCSS(tc.css)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("expected CSS to validate, got error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if tc.wantBadIn == "" {
				return
			}
			found := false
			for _, u := range ve.OffendingURLs {
				if containsStr(u, tc.wantBadIn) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected offending URL containing %q, got %v", tc.wantBadIn, ve.OffendingURLs)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
