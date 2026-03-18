package client

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ProvisionResult holds the shared test environment created for a scenario run.
type ProvisionResult struct {
	Users     []*VirtualUser
	ServerID  int64
	ChannelID int64
	Prefix    string
	Host      string
	Secret    string
}

// Provision creates N virtual users with a shared server and channel.
// It tries the fast /internal/bench/provision endpoint first; if the server
// returns 404 (normal build), it falls back to REST-based registration.
// Note: the REST fallback is rate-limited to 10 req/min, making it very slow
// for large counts. The stresstest server build is strongly recommended.
func Provision(ctx context.Context, host, secret, prefix string, count int) (*ProvisionResult, error) {
	base := NewHTTPClient(host, "", secret)

	resp, err := base.Provision(ctx, count, prefix)
	if err != nil && errors.Is(err, ErrProvisionNotFound) {
		fmt.Printf("WARNING: fast provision endpoint not available — falling back to REST registration (slow for >10 users)\n")
		return provisionViaREST(ctx, host, secret, prefix, count)
	}
	if err != nil {
		return nil, fmt.Errorf("provision: %w", err)
	}

	users := make([]*VirtualUser, len(resp.Users))
	for i, u := range resp.Users {
		users[i] = NewVirtualUser(u.ID, u.Username, u.Token, host, secret)
	}
	return &ProvisionResult{
		Users:     users,
		ServerID:  resp.ServerID,
		ChannelID: resp.ChannelID,
		Prefix:    prefix,
		Host:      host,
		Secret:    secret,
	}, nil
}

// Cleanup deletes all test data for a given prefix.
func Cleanup(ctx context.Context, host, secret, prefix string) (int64, error) {
	base := NewHTTPClient(host, "", secret)
	return base.Cleanup(ctx, prefix)
}

// provisionViaREST registers users one by one via the normal auth endpoint,
// then creates a server and channel. Rate-limited: 10 req/min per IP.
func provisionViaREST(ctx context.Context, host, secret, prefix string, count int) (*ProvisionResult, error) {
	base := NewHTTPClient(host, "", secret)

	users := make([]*VirtualUser, 0, count)
	for i := 0; i < count; i++ {
		username := fmt.Sprintf("%s%d", prefix, i)
		email := fmt.Sprintf("%s@bench.invalid", username)
		password := "benchtest"

		resp, status, err := base.Register(ctx, username, email, password)
		if err != nil {
			return nil, fmt.Errorf("register user %d: %w", i, err)
		}
		if status != 201 {
			return nil, fmt.Errorf("register user %d: status %d", i, status)
		}

		users = append(users, NewVirtualUser(resp.User.ID, username, resp.Token, host, secret))

		// Rate-limit: 10 req/min = 6s between requests (conservative).
		if i < count-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(6 * time.Second):
			}
		}
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("no users provisioned")
	}

	// Create server using first user's token.
	ownerHTTP := users[0].HTTP
	_, serverStatus, err := ownerHTTP.do(ctx, "POST", "/api/servers",
		map[string]string{"name": prefix + "server"},
	)
	if err != nil || serverStatus != 201 {
		return nil, fmt.Errorf("create server: status %d, err %v", serverStatus, err)
	}

	// For REST fallback, parse server and channel IDs from the create responses.
	// This is simplified — a full implementation would parse the JSON.
	// For production use of the bench tool, use the stresstest server build.
	return nil, fmt.Errorf("REST fallback: server/channel parsing not implemented — use stresstest build")
}
