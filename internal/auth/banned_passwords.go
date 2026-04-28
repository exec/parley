package auth

import (
	_ "embed"
	"strings"
)

//go:embed wordlist/common-passwords.txt
var commonPasswordsRaw string

var bannedPasswords map[string]struct{}

func init() {
	lines := strings.Split(commonPasswordsRaw, "\n")
	bannedPasswords = make(map[string]struct{}, len(lines))
	for _, line := range lines {
		p := strings.TrimSpace(line)
		if p != "" {
			bannedPasswords[strings.ToLower(p)] = struct{}{}
		}
	}
	// Drop the raw embedded string — the lookup table is built and the
	// ~9MB blob is no longer needed. Setting to "" lets the GC reclaim it.
	commonPasswordsRaw = ""
}

// IsBannedPassword reports whether p appears in the SecLists top-1M list
// (case-insensitive). Used to reject obviously-weak passwords at registration
// and password-change time. The bcrypt hash + rate-limit + fail2ban handle
// anything not on this list.
func IsBannedPassword(p string) bool {
	_, ok := bannedPasswords[strings.ToLower(p)]
	return ok
}
