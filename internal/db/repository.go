package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	"parley/internal/audit"
)

// IsPgErrorCode returns true when err (possibly wrapped) carries the given
// Postgres SQLSTATE across either the pgx driver (pgconn.PgError) or the
// legacy lib/pq one. Keeps error-code checks driver-agnostic so callers
// don't care which driver the pool was opened with.
func IsPgErrorCode(err error, code string) bool {
	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		return pgxErr.Code == code
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == code
	}
	return false
}

// IsUniqueViolation is a convenience for the common 23505 check.
func IsUniqueViolation(err error) bool { return IsPgErrorCode(err, "23505") }

var (
	ErrNotFound         = errors.New("record not found")
	ErrAlreadyExists    = errors.New("record already exists")
	ErrInvalidOperation = errors.New("invalid operation")
)

// metadataCacheTTL controls how long read-heavy metadata (channels, servers)
// stays warm in memory before the next lookup hits Postgres. Keep it short
// enough that rename/icon/topic changes feel near-instant, long enough that
// a chatty channel with dozens of messages per second doesn't re-query the
// same row on every message.
const metadataCacheTTL = 5 * time.Second

type cachedChannel struct {
	ch        *Channel
	expiresAt time.Time
}

type cachedServer struct {
	srv       *Server
	expiresAt time.Time
}

// UserMessageInfo is the narrow user row the message path needs to decorate
// a newly-sent message (author username/display_name/avatar + bot flag).
// It's cached to avoid re-fetching the same row on every message.
type UserMessageInfo struct {
	Username    string
	DisplayName string
	AvatarURL   string
	IsBot       bool
}

type cachedUserMessageInfo struct {
	u         *UserMessageInfo
	expiresAt time.Time
}

// Repository handles database operations for all models.
type Repository struct {
	db               *sql.DB
	channelCache     sync.Map // int64 id -> *cachedChannel
	serverCache      sync.Map // int64 id -> *cachedServer
	userMsgInfoCache sync.Map // int64 id -> *cachedUserMessageInfo
}

// NewRepository creates a new Repository with the given database connection.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// invalidateChannelCache drops a channel entry so the next read refreshes.
func (r *Repository) invalidateChannelCache(id int64) { r.channelCache.Delete(id) }

// invalidateServerCache drops a server entry so the next read refreshes.
func (r *Repository) invalidateServerCache(id int64) { r.serverCache.Delete(id) }

// invalidateUserMessageInfo drops a user's message-path cache entry.
func (r *Repository) invalidateUserMessageInfo(id int64) { r.userMsgInfoCache.Delete(id) }

// GetUserMessageInfo returns the narrow user fields the message path needs
// (username/display_name/avatar_url/is_bot), with a short-TTL cache so a
// chatty user in a message-storm doesn't re-hit the DB for every send.
func (r *Repository) GetUserMessageInfo(ctx context.Context, userID int64) (*UserMessageInfo, error) {
	if v, ok := r.userMsgInfoCache.Load(userID); ok {
		e := v.(*cachedUserMessageInfo)
		if time.Now().Before(e.expiresAt) {
			return e.u, nil
		}
	}
	var u UserMessageInfo
	err := r.db.QueryRowContext(ctx,
		"SELECT username, COALESCE(display_name, ''), COALESCE(avatar_url, ''), is_bot FROM users WHERE id = $1",
		userID,
	).Scan(&u.Username, &u.DisplayName, &u.AvatarURL, &u.IsBot)
	if err != nil {
		return nil, err
	}
	r.userMsgInfoCache.Store(userID, &cachedUserMessageInfo{u: &u, expiresAt: time.Now().Add(metadataCacheTTL)})
	return &u, nil
}

// NewRepositoryWithDSN creates a new Repository and establishes a connection using the DSN.
func NewRepositoryWithDSN(ctx context.Context, dsn string) (*Repository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return NewRepository(db), nil
}

// Close closes the database connection.
func (r *Repository) Close() error {
	return r.db.Close()
}

// DB returns the underlying database connection.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// RunMigrations executes pending database migrations using a schema_migrations
// tracking table so each migration runs exactly once, preventing destructive
// migrations (e.g. permission bit remaps) from re-running on every restart.
func (r *Repository) RunMigrations(ctx context.Context) error {
	// Ensure tracking table exists.
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id         INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Bootstrap: if the tracker is empty but the DB is already initialised
	// (existing install), mark every current migration as applied so they are
	// not re-run and do not corrupt data.
	var tracked int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&tracked); err != nil {
		return fmt.Errorf("count schema_migrations: %w", err)
	}
	if tracked == 0 {
		var initialized bool
		if err := r.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = 'users'
			)
		`).Scan(&initialized); err != nil {
			return fmt.Errorf("check db initialized state: %w", err)
		}
		if initialized {
			// DB already has tables — seed the tracker with all migrations that
			// have been applied (the entire current list for existing installs).
			for i := range Migrations {
				r.db.ExecContext(ctx, `INSERT INTO schema_migrations (id) VALUES ($1) ON CONFLICT DO NOTHING`, i)
			}
			log.Printf("schema_migrations bootstrapped with %d entries for existing install", len(Migrations))
			return nil
		}
	}

	// Run each migration that has not been recorded yet.
	for i, sql := range Migrations {
		var applied bool
		r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE id = $1)`, i).Scan(&applied)
		if applied {
			continue
		}
		if _, err := r.db.ExecContext(ctx, sql); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
		if _, err := r.db.ExecContext(ctx, `INSERT INTO schema_migrations (id) VALUES ($1) ON CONFLICT DO NOTHING`, i); err != nil {
			log.Printf("migration %d: failed to record: %v", i, err)
		}
		log.Printf("migration %d applied", i)
	}
	return nil
}

// CreateUserPreferences inserts a default preferences row for a new user.
func (r *Repository) CreateUserPreferences(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_preferences (user_id, active_theme)
		 VALUES ($1, 'rory') ON CONFLICT DO NOTHING`, userID)
	return err
}

// nullableString converts an empty string to a NULL sql.NullString.
func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// GetUsernameByID returns the username for a user ID. Used by audit call sites.
func (r *Repository) GetUsernameByID(ctx context.Context, userID int64) (string, error) {
	var username string
	err := r.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
	if err != nil {
		return "", err
	}
	return username, nil
}

// GetUserBadges returns the badges bitmask for a user.
func (r *Repository) GetUserBadges(ctx context.Context, userID int64) (int, error) {
	var badges int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(badges, 0) FROM users WHERE id=$1`, userID).Scan(&badges)
	return badges, err
}

// GetServerRoleByID fetches a single role by ID (used for role.update before-snapshot).
func (r *Repository) GetServerRoleByID(ctx context.Context, roleID int64) (*ServerRole, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, server_id, name, color, permissions, hoist, position, is_everyone, created_at
		 FROM server_roles WHERE id = $1`, roleID)
	var role ServerRole
	if err := row.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color,
		&role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt); err != nil {
		return nil, err
	}
	return &role, nil
}

// AuditLog is the DB model for server_audit_logs.
type AuditLog struct {
	ID              int64
	ServerID        int64
	ActorID         *int64
	ActorUsername   string
	ActorAvatarURL  string // joined from users.avatar_url (empty if actor missing)
	Action          string
	TargetID        string
	TargetType      string
	TargetName      string
	TargetAvatarURL string // joined from users.avatar_url (only when target_type='user')
	Changes         []byte // raw JSONB
	Reason          string
	CreatedAt       time.Time
}

// CreateAuditLog inserts one audit log row.
func (r *Repository) CreateAuditLog(ctx context.Context, e audit.Entry) error {
	// changesArg is nil (→ SQL NULL) when there are no changes, or a JSON string
	// when there are. lib/pq sends []byte as bytea (binary), which Postgres rejects
	// for JSONB, so we pass the JSON as a string instead.
	var changesArg interface{}
	if e.Changes != nil {
		b, err := json.Marshal(e.Changes)
		if err != nil {
			return err
		}
		changesArg = string(b)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_audit_logs
		 (server_id, actor_id, actor_username, action, target_id, target_type, target_name, changes, reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.ServerID, e.ActorID, e.ActorUsername, e.Action,
		nullableString(e.TargetID), nullableString(e.TargetType), nullableString(e.TargetName),
		changesArg, nullableString(e.Reason),
	)
	return err
}

// ListAuditLogs returns audit log entries for a server, newest first.
// actorID, action, and target are optional filters (nil / "" = no filter).
// target performs a substring (ILIKE) match on target_name.
// Returns (entries, totalCount, error).
func (r *Repository) ListAuditLogs(ctx context.Context, serverID int64, actorID *int64, action, target string, limit, offset int) ([]AuditLog, int, error) {
	args := []any{serverID}
	where := `WHERE sal.server_id = $1`
	idx := 2
	if actorID != nil {
		where += fmt.Sprintf(` AND sal.actor_id = $%d`, idx)
		args = append(args, *actorID)
		idx++
	}
	if action != "" {
		where += fmt.Sprintf(` AND sal.action = $%d`, idx)
		args = append(args, action)
		idx++
	}
	if target != "" {
		where += fmt.Sprintf(` AND sal.target_name ILIKE $%d ESCAPE '\'`, idx)
		args = append(args, "%"+escapeLike(target)+"%")
		idx++
	}

	// total count
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_audit_logs sal `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// paginated rows. Two LEFT JOINs: actor (always-on by user id), target
	// (only when target_type='user'; cast users.id → text since target_id is TEXT).
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT sal.id, sal.server_id, sal.actor_id, sal.actor_username,
		        COALESCE(actor.avatar_url, ''),
		        sal.action,
		        COALESCE(sal.target_id, ''), COALESCE(sal.target_type, ''),
		        COALESCE(sal.target_name, ''),
		        COALESCE(target.avatar_url, ''),
		        sal.changes, COALESCE(sal.reason, ''), sal.created_at
		 FROM server_audit_logs sal
		 LEFT JOIN users actor ON actor.id = sal.actor_id
		 LEFT JOIN users target ON sal.target_type = 'user'
		                       AND target.id::text = sal.target_id
		 `+where+
			fmt.Sprintf(` ORDER BY sal.created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.ServerID, &l.ActorID, &l.ActorUsername,
			&l.ActorAvatarURL,
			&l.Action,
			&l.TargetID, &l.TargetType, &l.TargetName,
			&l.TargetAvatarURL,
			&l.Changes, &l.Reason, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
