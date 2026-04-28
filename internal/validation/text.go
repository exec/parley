package validation

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NormalizeUsername applies Unicode NFC normalization to a username before any
// further validation. The username regex (UsernameRe) only accepts ASCII, so
// NFC-normalising first prevents homograph or zero-width tricks that try to
// register a visually identical name to an existing user. NFC must run before
// the regex check; the regex must run against the normalized form, and the
// normalized form is what gets stored.
func NormalizeUsername(s string) string {
	return norm.NFC.String(s)
}

// isStrippableRune reports whether r is one of the invisible / directional /
// control characters that we never want to persist in user-supplied display
// text. Specifically:
//
//   - ASCII control chars U+0000–U+001F (the caller decides whether to keep
//     newline / tab — see SanitizeMultiLine).
//   - U+007F DELETE.
//   - Unicode bidi overrides U+202A–U+202E and U+2066–U+2069 (Trojan Source).
//   - Zero-width / formatting code points U+200B–U+200F and U+FEFF (BOM).
func isStrippableRune(r rune) bool {
	if r < 0x20 || r == 0x7f {
		return true
	}
	switch r {
	case 0x200B, 0x200C, 0x200D, 0x200E, 0x200F, 0xFEFF:
		return true
	}
	if r >= 0x202A && r <= 0x202E {
		return true
	}
	if r >= 0x2066 && r <= 0x2069 {
		return true
	}
	return false
}

// SanitizeSingleLine removes ASCII control characters, Unicode bidi-override
// code points, and zero-width / BOM code points from s. Use this for fields
// that must render on a single line (display_name, status_text). Non-strippable
// runes pass through untouched. The result is also NFC-normalized so the
// stored form has a stable byte representation.
func SanitizeSingleLine(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isStrippableRune(r) {
			continue
		}
		if !unicode.IsPrint(r) && r != ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// SanitizeMultiLine is like SanitizeSingleLine but preserves '\n' and '\t'.
// Use this for fields where users legitimately type multiple lines (bio).
// Carriage returns are dropped to avoid CRLF artifacts; consumers see only
// '\n' line breaks.
func SanitizeMultiLine(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if r == '\r' {
			continue
		}
		if isStrippableRune(r) {
			continue
		}
		if !unicode.IsPrint(r) && r != ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}
