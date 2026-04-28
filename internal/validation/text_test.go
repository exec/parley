package validation

import "testing"

func TestSanitizeSingleLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii", "hello world", "hello world"},
		{"strips null", "hel\x00lo", "hello"},
		{"strips control range", "a\x01b\x1fc", "abc"},
		{"strips DEL", "a\x7fb", "ab"},
		{"strips zero-width space", "a​b", "ab"},
		{"strips zero-width non-joiner", "a‌b", "ab"},
		{"strips zero-width joiner", "a‍b", "ab"},
		{"strips LRM", "a‎b", "ab"},
		{"strips RLM", "a‏b", "ab"},
		{"strips BOM", "\uFEFFhello", "hello"},
		{"strips LRE", "a‪b", "ab"},
		{"strips RLE", "a‫b", "ab"},
		{"strips PDF", "a‬b", "ab"},
		{"strips LRO", "a‭b", "ab"},
		{"strips RLO", "a‮b", "ab"},
		{"strips LRI / RLI / FSI / PDI", "a⁦b⁧c⁨d⁩e", "abcde"},
		{"strips newline (single line)", "a\nb", "ab"},
		{"strips tab (single line)", "a\tb", "ab"},
		{"keeps unicode letters", "café 日本語", "café 日本語"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeSingleLine(tc.in); got != tc.want {
				t.Errorf("SanitizeSingleLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeMultiLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"keeps newline", "line1\nline2", "line1\nline2"},
		{"keeps tab", "a\tb", "a\tb"},
		{"strips CR", "a\r\nb", "a\nb"},
		{"strips control but keeps newline", "a\x01\nb", "a\nb"},
		{"strips zero-width", "a‍z", "az"},
		{"strips RTL override across lines", "good‮evil\nrest", "goodevil\nrest"},
		{"plain text", "the quick\nbrown fox", "the quick\nbrown fox"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeMultiLine(tc.in); got != tc.want {
				t.Errorf("SanitizeMultiLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeUsername(t *testing.T) {
	// "café" with decomposed combining acute (e + U+0301) should normalize to
	// the pre-composed form. After NFC the byte representation matches the
	// pre-composed string, so two visually identical names collapse to one.
	decomposed := "café"
	composed := "café"
	if got := NormalizeUsername(decomposed); got != composed {
		t.Errorf("NormalizeUsername(decomposed) = %q (% x), want %q (% x)",
			got, []byte(got), composed, []byte(composed))
	}
	// Idempotent on already-normalized input.
	if got := NormalizeUsername(composed); got != composed {
		t.Errorf("NormalizeUsername(composed) = %q, want %q", got, composed)
	}
	if got := NormalizeUsername("alice"); got != "alice" {
		t.Errorf("NormalizeUsername(ascii) = %q, want %q", got, "alice")
	}
}
