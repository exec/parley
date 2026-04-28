package auth

import (
	"os"
	"testing"
)

// TestMain requires JWT_SECRET to be set in the environment before running
// tests. config.go's package-level DefaultConfig() calls log.Fatal if it is
// missing or under 32 bytes, so the env var must be present and long enough
// when the test binary starts:
//
//	JWT_SECRET=test-secret-at-least-32-bytes-long-1234 go test ./internal/auth/...
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
