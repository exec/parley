package auth

import (
	"os"
	"testing"
)

// TestMain requires JWT_SECRET to be set in the environment before running
// tests. config.go's package-level DefaultConfig() calls log.Fatal if it is
// missing, so the env var must be present when the test binary starts:
//
//	JWT_SECRET=test go test ./internal/auth/...
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
