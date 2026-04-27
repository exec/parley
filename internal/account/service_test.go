// internal/account/service_test.go
//
// NOTE: This package imports internal/auth (via handler.go), which calls
// log.Fatal at package init if JWT_SECRET is unset. Run tests with:
//
//	JWT_SECRET=test go test ./internal/account/...
package account

import (
	"context"
	"errors"
	"testing"
)

// fakeStore is an in-memory dataStore used for unit tests. Tracks every call
// so tests can assert on the order/arguments of the deletion pipeline.
type fakeStore struct {
	sentinelID    int64
	sentinelErr   error
	username      string
	usernameErr   error
	avatar        string
	banner        string
	avatarErr     error
	servers       []BlockerInfo
	groupDMs      []BlockerInfo
	serverErr     error
	groupErr      error
	deleteErr     error
	deleteCalls   []deleteCall
	usernameCalls int
	sentinelCalls int
}

type deleteCall struct {
	UserID, SentinelID int64
}

func (f *fakeStore) LookupSentinelID(ctx context.Context) (int64, error) {
	f.sentinelCalls++
	return f.sentinelID, f.sentinelErr
}

func (f *fakeStore) LookupUsername(ctx context.Context, userID int64) (string, error) {
	f.usernameCalls++
	return f.username, f.usernameErr
}

func (f *fakeStore) FindBlockingServers(ctx context.Context, userID int64) ([]BlockerInfo, error) {
	return f.servers, f.serverErr
}

func (f *fakeStore) FindBlockingGroupDMs(ctx context.Context, userID int64) ([]BlockerInfo, error) {
	return f.groupDMs, f.groupErr
}

func (f *fakeStore) LookupAvatarBanner(ctx context.Context, userID int64) (string, string, error) {
	return f.avatar, f.banner, f.avatarErr
}

func (f *fakeStore) DeleteUser(ctx context.Context, userID, sentinelID int64) error {
	f.deleteCalls = append(f.deleteCalls, deleteCall{UserID: userID, SentinelID: sentinelID})
	return f.deleteErr
}

// --- Sentinel error tests ---

func TestSentinelErrorsAreDistinct(t *testing.T) {
	if errors.Is(ErrInvalidConfirmation, ErrHasBlockers) {
		t.Errorf("ErrInvalidConfirmation and ErrHasBlockers should be distinct")
	}
	if errors.Is(ErrHasBlockers, ErrInvalidConfirmation) {
		t.Errorf("ErrHasBlockers and ErrInvalidConfirmation should be distinct")
	}
}

func TestBlockersErrorWrapsSentinel(t *testing.T) {
	be := &BlockersError{Servers: []BlockerInfo{{ID: "1", Name: "x"}}}
	if !errors.Is(be, ErrHasBlockers) {
		t.Fatal("*BlockersError should errors.Is ErrHasBlockers")
	}
	var dst *BlockersError
	if !errors.As(be, &dst) {
		t.Fatal("errors.As(*BlockersError) should succeed")
	}
	if len(dst.Servers) != 1 {
		t.Fatalf("payload not preserved through errors.As")
	}
}

// --- VerifyConfirmation ---

func TestVerifyConfirmation(t *testing.T) {
	cases := []struct {
		name     string
		stored   string
		supplied string
		wantErr  error
	}{
		{"exact match", "alice", "alice", nil},
		{"mismatch", "alice", "Alice", ErrInvalidConfirmation},
		{"empty supplied", "alice", "", ErrInvalidConfirmation},
		{"deleted user (no row)", "", "alice", ErrInvalidConfirmation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{username: tc.stored}
			svc := newServiceWithStore(store, nil, nil, "")
			err := svc.VerifyConfirmation(context.Background(), 42, tc.supplied)
			if !errors.Is(err, tc.wantErr) && err != tc.wantErr {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// --- Blocker detection ---

func TestDeleteRejectsServerBlockers(t *testing.T) {
	store := &fakeStore{
		sentinelID: 1,
		username:   "alice",
		servers:    []BlockerInfo{{ID: "100", Name: "Cool Server"}},
	}
	svc := newServiceWithStore(store, nil, nil, "")
	err := svc.Delete(context.Background(), 42, "alice")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrHasBlockers) {
		t.Fatalf("expected ErrHasBlockers, got %v", err)
	}
	var be *BlockersError
	if !errors.As(err, &be) {
		t.Fatal("expected *BlockersError payload")
	}
	if len(be.Servers) != 1 || be.Servers[0].ID != "100" || be.Servers[0].Name != "Cool Server" {
		t.Fatalf("unexpected blocker payload: %+v", be.Servers)
	}
	if len(store.deleteCalls) != 0 {
		t.Fatal("DeleteUser should NOT be invoked when blockers exist")
	}
}

func TestDeleteRejectsGroupDMBlockers(t *testing.T) {
	store := &fakeStore{
		sentinelID: 1,
		username:   "alice",
		groupDMs:   []BlockerInfo{{ID: "200", Name: "Hangout"}},
	}
	svc := newServiceWithStore(store, nil, nil, "")
	err := svc.Delete(context.Background(), 42, "alice")
	var be *BlockersError
	if !errors.As(err, &be) {
		t.Fatalf("expected *BlockersError, got %v", err)
	}
	if len(be.GroupDMs) != 1 || be.GroupDMs[0].ID != "200" {
		t.Fatalf("expected one group-DM blocker, got %+v", be.GroupDMs)
	}
	if len(store.deleteCalls) != 0 {
		t.Fatal("DeleteUser should NOT be invoked when blockers exist")
	}
}

func TestDeleteRejectsBothBlockerKinds(t *testing.T) {
	store := &fakeStore{
		sentinelID: 1,
		username:   "alice",
		servers:    []BlockerInfo{{ID: "100", Name: "S"}},
		groupDMs:   []BlockerInfo{{ID: "200", Name: "G"}},
	}
	svc := newServiceWithStore(store, nil, nil, "")
	err := svc.Delete(context.Background(), 42, "alice")
	var be *BlockersError
	if !errors.As(err, &be) {
		t.Fatalf("expected *BlockersError, got %v", err)
	}
	if len(be.Servers) != 1 || len(be.GroupDMs) != 1 {
		t.Fatalf("both kinds should be reported, got servers=%d groups=%d", len(be.Servers), len(be.GroupDMs))
	}
}

// --- Happy path ---

func TestDeleteHappyPathInvokesDeleteUserWithSentinel(t *testing.T) {
	store := &fakeStore{
		sentinelID: 7,
		username:   "alice",
	}
	svc := newServiceWithStore(store, nil, nil, "")
	if err := svc.Delete(context.Background(), 42, "alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(store.deleteCalls) != 1 {
		t.Fatalf("expected exactly one DeleteUser call, got %d", len(store.deleteCalls))
	}
	got := store.deleteCalls[0]
	if got.UserID != 42 || got.SentinelID != 7 {
		t.Fatalf("DeleteUser called with wrong args: %+v", got)
	}
}

func TestDeleteRejectsBadConfirmation(t *testing.T) {
	store := &fakeStore{sentinelID: 7, username: "alice"}
	svc := newServiceWithStore(store, nil, nil, "")
	err := svc.Delete(context.Background(), 42, "WRONG")
	if !errors.Is(err, ErrInvalidConfirmation) {
		t.Fatalf("expected ErrInvalidConfirmation, got %v", err)
	}
	if len(store.deleteCalls) != 0 {
		t.Fatal("DeleteUser must not run when confirmation fails")
	}
}

func TestDeleteRefusesToDeleteSentinelItself(t *testing.T) {
	// Defence-in-depth: even if some internal mismatch fires VerifyConfirmation
	// for the sentinel's own id, the service must refuse rather than reassign-
	// and-delete the row that everyone else's history points at.
	store := &fakeStore{sentinelID: 42, username: "deleted"}
	svc := newServiceWithStore(store, nil, nil, "")
	err := svc.Delete(context.Background(), 42, "deleted")
	if !errors.Is(err, ErrInvalidConfirmation) {
		t.Fatalf("expected ErrInvalidConfirmation for sentinel-self delete, got %v", err)
	}
	if len(store.deleteCalls) != 0 {
		t.Fatal("sentinel must never be deleted")
	}
}

// --- Sentinel caching ---

func TestSentinelLookupIsCached(t *testing.T) {
	store := &fakeStore{sentinelID: 7, username: "alice"}
	svc := newServiceWithStore(store, nil, nil, "")
	for i := 0; i < 3; i++ {
		if err := svc.Delete(context.Background(), int64(100+i), "alice"); err != nil {
			t.Fatalf("Delete iter %d: %v", i, err)
		}
		store.username = "alice" // reset for next iter
	}
	if store.sentinelCalls != 1 {
		t.Fatalf("LookupSentinelID should be called exactly once across multiple Deletes, got %d", store.sentinelCalls)
	}
}

// --- objectKey ---

func TestObjectKeyExtractsBucketKeyFromCDNURL(t *testing.T) {
	svc := newServiceWithStore(&fakeStore{}, nil, nil, "")
	cases := []struct {
		raw, want string
	}{
		{"", ""},
		{"https://cdn.example.com/uploads/abc.png", "uploads/abc.png"},
		{"https://cdn.example.com/uploads/nested/x.gif", "uploads/nested/x.gif"},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := svc.objectKey(tc.raw); got != tc.want {
			t.Errorf("objectKey(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestObjectKeyHostPrefixFallback(t *testing.T) {
	svc := newServiceWithStore(&fakeStore{}, nil, nil, "cdn.example.com")
	if got := svc.objectKey("cdn.example.com/uploads/abc.png"); got != "uploads/abc.png" {
		t.Errorf("host-prefix fallback failed: got %q", got)
	}
}
