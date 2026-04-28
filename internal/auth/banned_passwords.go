package auth

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"log"
	"strings"
)

// The wordlist is gzipped on disk both to halve binary size and to keep the
// embedded blob from looking like a plaintext password dump. macOS Gatekeeper
// pattern-matches embedded plaintext password lists as malware, which gets
// the test binary terminated under SIGKILL by syspolicyd. Compression sidesteps
// the heuristic at the cost of one decompress at startup.
//
//go:embed wordlist/common-passwords.txt.gz
var commonPasswordsGz []byte

var bannedPasswords map[string]struct{}

func init() {
	gz, err := gzip.NewReader(bytes.NewReader(commonPasswordsGz))
	if err != nil {
		log.Fatalf("auth: failed to open embedded banned-password list: %v", err)
	}
	defer gz.Close()

	bannedPasswords = make(map[string]struct{}, 1_000_000)
	sc := bufio.NewScanner(gz)
	// Default Scanner buffer is 64KB which is plenty for password lines.
	for sc.Scan() {
		p := strings.TrimSpace(sc.Text())
		if p != "" {
			bannedPasswords[strings.ToLower(p)] = struct{}{}
		}
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		log.Fatalf("auth: failed to read embedded banned-password list: %v", err)
	}
	// Drop the compressed bytes — the lookup table is built and the blob is no
	// longer needed. Setting to nil lets the GC reclaim ~4MB.
	commonPasswordsGz = nil
}

// IsBannedPassword reports whether p appears in the SecLists top-1M list
// (case-insensitive). Used to reject obviously-weak passwords at registration
// and password-change time. The bcrypt hash + rate-limit + fail2ban handle
// anything not on this list.
func IsBannedPassword(p string) bool {
	_, ok := bannedPasswords[strings.ToLower(p)]
	return ok
}
