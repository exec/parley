package cssvalidator

import (
	"errors"
	"strings"
	"testing"
)

const testCDNHost = "cdn.parley.example"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		css     string
		want    string // "ok" or the substring the offending URL should contain
		wantErr bool
	}{
		{
			name:    "url form blocks non-allowlisted host (baseline)",
			css:     `@import url("https://evil.tld/x.css");`,
			wantErr: true,
			want:    "evil.tld",
		},
		{
			name:    "string form blocks non-allowlisted host",
			css:     `@import "https://evil.tld/x.css";`,
			wantErr: true,
			want:    "evil.tld",
		},
		{
			name:    "string form blocks protocol-relative",
			css:     `@import "//evil.tld/x.css";`,
			wantErr: true,
			want:    "evil.tld",
		},
		{
			name:    "string form blocks hex-escaped scheme",
			css:     `@import '\68\74\74\70s://evil.tld/x.css';`,
			wantErr: true,
			// hex-escape detector fires before url.Parse — offending URL is the raw
			// escape-containing string, not the decoded host, so assert on the backslash.
			want: `\68`,
		},
		{
			name:    "string form allows fonts.googleapis.com",
			css:     `@import "https://fonts.googleapis.com/css2?family=Foo";`,
			wantErr: false,
		},
		{
			name:    "url form allows fonts.googleapis.com",
			css:     `@import url("https://fonts.googleapis.com/css2?family=Foo");`,
			wantErr: false,
		},
		{
			name:    "url form allows data: URIs (SVG background)",
			css:     `body { background: url(data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciLz4=); }`,
			wantErr: false,
		},
		{
			name:    "url form allows configured CDN host",
			css:     `body { background: url("https://` + testCDNHost + `/bg.jpg"); }`,
			wantErr: false,
		},
		{
			name:    "url form blocks non-CDN host",
			css:     `body { background: url("https://attacker.example/leak.png"); }`,
			wantErr: true,
			want:    "attacker.example",
		},
		{
			name:    "empty CSS is valid",
			css:     ``,
			wantErr: false,
		},
		{
			name:    "mix of allowed and blocked surfaces blocked one",
			css:     `@import "https://fonts.googleapis.com/css2?family=Foo"; @import "https://evil.tld/x.css";`,
			wantErr: true,
			want:    "evil.tld",
		},
		{
			name:    "relative-path @import ignored (no external host)",
			css:     `@import "./local.css";`,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.css, testCDNHost)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(...) = nil, want error")
				}
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("Validate(...) = %v, want *ValidationError", err)
				}
				if tc.want != "" {
					found := false
					for _, u := range ve.OffendingURLs {
						if strings.Contains(u, tc.want) {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("offending URLs %v did not contain %q", ve.OffendingURLs, tc.want)
					}
				}
			} else if err != nil {
				t.Fatalf("Validate(...) = %v, want nil", err)
			}
		})
	}
}

func TestValidateSizeCap(t *testing.T) {
	huge := strings.Repeat("a", MaxCSSBytes+1)
	err := Validate(huge, testCDNHost)
	if err == nil {
		t.Fatal("Validate oversized CSS = nil, want error")
	}
	if IsValidationError(err) {
		t.Fatalf("size-cap error should not be *ValidationError, got %T", err)
	}
}
