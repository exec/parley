package auth

import "testing"

func TestIsBannedPassword(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		banned bool
	}{
		{"common lowercase", "password", true},
		{"common titlecase", "Password", true},
		{"common uppercase", "PASSWORD", true},
		{"trivial digits", "12345678", true},
		{"unique strong password", "MyV3ryUniqueP@ssw0rd2026!", false},
		{"empty string is not banned", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsBannedPassword(tc.input)
			if got != tc.banned {
				t.Errorf("IsBannedPassword(%q) = %v, want %v", tc.input, got, tc.banned)
			}
		})
	}
}
