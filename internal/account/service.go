package account

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"parley/internal/db"
	"parley/internal/spaces"
	ws "parley/internal/websocket"
)

// dataStore is the narrow data-access surface the Service needs. The
// production implementation wraps *sql.DB; tests supply a fake.
type dataStore interface {
	// LookupSentinelID returns the id of the seeded `[deleted]` system user.
	// Implementations should be safe to call concurrently.
	LookupSentinelID(ctx context.Context) (int64, error)

	// LookupUsername returns the username for the given user id.
	LookupUsername(ctx context.Context, userID int64) (string, error)

	// FindBlockingServers returns servers owned by userID that still have
	// at least one other member. Membership is read from server_members.
	FindBlockingServers(ctx context.Context, userID int64) ([]BlockerInfo, error)

	// FindBlockingGroupDMs returns group DM channels owned by userID that
	// still have at least one other member.
	FindBlockingGroupDMs(ctx context.Context, userID int64) ([]BlockerInfo, error)

	// LookupAvatarBanner returns the avatar_url and banner_url so the caller
	// can best-effort delete the underlying objects after the user row is
	// gone. Either field may be empty.
	LookupAvatarBanner(ctx context.Context, userID int64) (avatar, banner string, err error)

	// DeleteUser performs the transactional reassign + force-logout + cascade
	// delete in a single Postgres transaction. sentinelID is the resolved id
	// of the `[deleted]` user; userID is the account being deleted.
	DeleteUser(ctx context.Context, userID, sentinelID int64) error
}

// Service implements self-serve account deletion.
type Service struct {
	store        dataStore
	hub          *ws.Hub
	spacesClient *spaces.Client
	cdnHost      string

	sentinelOnce sync.Once
	sentinelID   int64
	sentinelErr  error
}

// NewService wires the production Service against a real *db.Repository, the
// websocket hub (for force-disconnecting any live sessions), and an optional
// *spaces.Client for best-effort avatar/banner cleanup. cdnHost lets the
// service strip the host prefix from stored URLs before issuing object
// deletes; pass "" to disable host-prefix stripping.
func NewService(repo *db.Repository, hub *ws.Hub, spacesClient *spaces.Client, cdnHost string) *Service {
	return &Service{
		store:        &sqlStore{db: repo.DB()},
		hub:          hub,
		spacesClient: spacesClient,
		cdnHost:      cdnHost,
	}
}

// newServiceWithStore is a test seam: lets unit tests inject a fake dataStore
// without standing up a real Postgres pool.
func newServiceWithStore(store dataStore, hub *ws.Hub, spacesClient *spaces.Client, cdnHost string) *Service {
	return &Service{store: store, hub: hub, spacesClient: spacesClient, cdnHost: cdnHost}
}

// resolveSentinel returns the cached sentinel id, looking it up on first call.
// Subsequent calls are O(1). A failure here is fatal for the request — without
// the sentinel we can't reassign authored content safely.
func (s *Service) resolveSentinel(ctx context.Context) (int64, error) {
	s.sentinelOnce.Do(func() {
		s.sentinelID, s.sentinelErr = s.store.LookupSentinelID(ctx)
	})
	return s.sentinelID, s.sentinelErr
}

// VerifyConfirmation checks the supplied username against the authenticated
// user's stored username. Returns ErrInvalidConfirmation on mismatch.
func (s *Service) VerifyConfirmation(ctx context.Context, userID int64, supplied string) error {
	got, err := s.store.LookupUsername(ctx, userID)
	if err != nil {
		return err
	}
	if got == "" || supplied != got {
		return ErrInvalidConfirmation
	}
	return nil
}

// Delete runs the full deletion pipeline: confirm → blocker check → tx delete
// → force-disconnect WS → best-effort spaces cleanup. The blocker check runs
// inside the same call but BEFORE any writes, so a 409 leaves the account
// untouched.
//
// Returns nil on success, ErrInvalidConfirmation, *BlockersError, or any
// underlying DB error.
func (s *Service) Delete(ctx context.Context, userID int64, confirmUsername string) error {
	if err := s.VerifyConfirmation(ctx, userID, confirmUsername); err != nil {
		return err
	}

	sentinelID, err := s.resolveSentinel(ctx)
	if err != nil {
		return err
	}
	if sentinelID == userID {
		// Defensive: never let the sentinel itself be deleted via this path.
		return ErrInvalidConfirmation
	}

	blockingServers, err := s.store.FindBlockingServers(ctx, userID)
	if err != nil {
		return err
	}
	blockingGroupDMs, err := s.store.FindBlockingGroupDMs(ctx, userID)
	if err != nil {
		return err
	}
	if len(blockingServers) > 0 || len(blockingGroupDMs) > 0 {
		return &BlockersError{Servers: blockingServers, GroupDMs: blockingGroupDMs}
	}

	// Snapshot avatar/banner URLs before the delete so we can clean up after
	// the row is gone. A failure here is non-fatal — we still proceed.
	avatar, banner, lookupErr := s.store.LookupAvatarBanner(ctx, userID)
	if lookupErr != nil {
		log.Printf("account.Delete: avatar/banner lookup failed for user %d: %v", userID, lookupErr)
	}

	if err := s.store.DeleteUser(ctx, userID, sentinelID); err != nil {
		return err
	}

	// Force-disconnect any live WS sessions so the client immediately loses
	// state instead of holding stale data until the next reconnect.
	if s.hub != nil {
		s.hub.DisconnectUser(strconv.FormatInt(userID, 10))
	}

	// Best-effort spaces cleanup. Failures here are logged but do not surface
	// to the caller — the account is already gone.
	s.cleanupAssets(ctx, avatar, banner)

	return nil
}

// cleanupAssets best-effort deletes the avatar and banner objects from spaces.
// The stored URLs are full CDN URLs; we extract the bucket-relative key by
// taking everything after the host. Empty inputs and a nil spaces client are
// no-ops.
func (s *Service) cleanupAssets(ctx context.Context, avatar, banner string) {
	if s.spacesClient == nil {
		return
	}
	for _, raw := range []string{avatar, banner} {
		key := s.objectKey(raw)
		if key == "" {
			continue
		}
		if err := s.spacesClient.Delete(ctx, key); err != nil {
			log.Printf("account.Delete: spaces cleanup of %q failed: %v", key, err)
		}
	}
}

// objectKey extracts the bucket-relative key from a stored CDN URL. Returns
// "" if the input is empty or unparseable. Tolerates URLs without a scheme by
// falling back to a host-prefix strip.
func (s *Service) objectKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" && (u.Host != "" || u.Scheme != "") {
		return strings.TrimPrefix(u.Path, "/")
	}
	if s.cdnHost != "" && strings.HasPrefix(raw, s.cdnHost) {
		return strings.TrimPrefix(strings.TrimPrefix(raw, s.cdnHost), "/")
	}
	return ""
}

// sqlStore is the production dataStore backed by *sql.DB. Each method maps
// 1:1 onto a small SQL query — the heavy lifting is in DeleteUser, which runs
// the entire reassign-and-cascade pipeline in a single transaction.
type sqlStore struct {
	db *sql.DB
}

func (s *sqlStore) LookupSentinelID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE username = 'deleted' AND is_system = TRUE`,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("account: [deleted] sentinel user missing — migration #69 not applied")
		}
		return 0, err
	}
	return id, nil
}

func (s *sqlStore) LookupUsername(ctx context.Context, userID int64) (string, error) {
	var u string
	err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&u)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return u, nil
}

func (s *sqlStore) FindBlockingServers(ctx context.Context, userID int64) ([]BlockerInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id::text, s.name
		FROM servers s
		WHERE s.owner_id = $1
		  AND EXISTS (
		      SELECT 1 FROM server_members sm
		      WHERE sm.server_id = s.id AND sm.user_id <> $1
		  )
		ORDER BY s.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlockerInfo
	for rows.Next() {
		var b BlockerInfo
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *sqlStore) FindBlockingGroupDMs(ctx context.Context, userID int64) ([]BlockerInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id::text, COALESCE(d.name, '')
		FROM dm_channels d
		WHERE d.is_group = TRUE
		  AND d.owner_user_id = $1
		  AND EXISTS (
		      SELECT 1 FROM dm_channel_members m
		      WHERE m.dm_channel_id = d.id AND m.user_id <> $1
		  )
		ORDER BY d.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlockerInfo
	for rows.Next() {
		var b BlockerInfo
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *sqlStore) LookupAvatarBanner(ctx context.Context, userID int64) (string, string, error) {
	var avatar, banner sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT avatar_url, banner_url FROM users WHERE id = $1`, userID,
	).Scan(&avatar, &banner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil
		}
		return "", "", err
	}
	return avatar.String, banner.String, nil
}

// DeleteUser runs the reassign-and-delete pipeline in a single transaction.
// Order matters: every authored-content table must be reassigned BEFORE the
// user row is deleted, otherwise the existing ON DELETE CASCADE FKs would
// scrub the rows we want to preserve. Bot users owned by this user are
// hard-deleted (cascades through their own messages/sessions/keys); only
// human authored content survives via reassignment.
func (s *sqlStore) DeleteUser(ctx context.Context, userID, sentinelID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Reassign authored content. Each UPDATE is keyed by author_id and a
	// no-op when the user has nothing in that table.
	reassigns := []string{
		`UPDATE messages          SET author_id = $1 WHERE author_id = $2`,
		`UPDATE dm_messages       SET author_id = $1 WHERE author_id = $2`,
		`UPDATE bin_posts         SET author_id = $1 WHERE author_id = $2`,
		`UPDATE bin_line_comments SET author_id = $1 WHERE author_id = $2`,
	}
	for _, q := range reassigns {
		if _, err := tx.ExecContext(ctx, q, sentinelID, userID); err != nil {
			return err
		}
	}

	// Force-logout: any in-flight JWT issued before this timestamp is
	// rejected by the auth middleware. Set this BEFORE the delete so the
	// flag lands even if the cascade trips a constraint somewhere we missed.
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET force_logout_at = NOW() WHERE id = $1`, userID,
	); err != nil {
		return err
	}

	// Hard-delete bots owned by this user. The bot user rows cascade their
	// own messages/passkeys/sessions/etc. on the way out. We don't reassign
	// bot-authored content to the sentinel — bots are application identities
	// and disappear with their owner.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM users WHERE bot_owner_id = $1`, userID,
	); err != nil {
		return err
	}

	// Finally, delete the user. Existing ON DELETE CASCADE FKs scrub
	// friend_requests, user_blocks, server_members, dm_channel_members,
	// notifications, sessions, passkeys, themes, voice_presences, etc.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM users WHERE id = $1`, userID,
	); err != nil {
		return err
	}

	return tx.Commit()
}
