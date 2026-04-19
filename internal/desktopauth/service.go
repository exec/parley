// Package desktopauth implements a short-lived code exchange used to hand a
// session from a browser-based passkey login back to a native desktop client
// that cannot invoke WebAuthn reliably (Tauri, Electron, etc).
//
// Flow:
//   1. Desktop generates a random `state` and opens the browser to
//      https://<site>/auth/desktop?state=<state>.
//   2. User signs in with a passkey in the browser.
//   3. Browser frontend POSTs to /api/auth/desktop/issue with {state}; the
//      server returns a one-time `code` bound to (user_id, state).
//   4. Browser redirects to parley://auth?code=<code>&state=<state>.
//   5. Desktop intercepts the deep link, verifies `state` matches its local
//      nonce, and POSTs {code, state} to /api/auth/desktop/exchange. The
//      server validates and returns a normal JWT.
package desktopauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	codeTTL       = 2 * time.Minute
	codeKeyPrefix = "desktopauth:code:"
)

var (
	ErrNotFound  = errors.New("code not found or expired")
	ErrStateMismatch = errors.New("state does not match")
)

type Service struct {
	rdb *goredis.Client
}

type codeEntry struct {
	UserID string `json:"user_id"`
	State  string `json:"state"`
}

func New(rdb *goredis.Client) *Service {
	return &Service{rdb: rdb}
}

// Issue stores a fresh code for (userID, state) and returns the code.
// The caller must already have authenticated the user.
func (s *Service) Issue(ctx context.Context, userID, state string) (string, error) {
	code, err := randomCode()
	if err != nil {
		return "", err
	}
	entry := codeEntry{UserID: userID, State: state}
	raw, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, codeKeyPrefix+code, raw, codeTTL).Err(); err != nil {
		return "", err
	}
	return code, nil
}

// Exchange atomically consumes a code and returns the associated user ID,
// provided the caller-supplied state matches what was stored at issue time.
// The code is single-use: a successful exchange deletes it.
func (s *Service) Exchange(ctx context.Context, code, state string) (string, error) {
	raw, err := s.rdb.GetDel(ctx, codeKeyPrefix+code).Bytes()
	if err == goredis.Nil {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var entry codeEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", err
	}
	if entry.State != state {
		return "", ErrStateMismatch
	}
	return entry.UserID, nil
}

func randomCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
