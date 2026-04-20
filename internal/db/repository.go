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

	"parley/internal/audit"
)

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

// Repository handles database operations for all models.
type Repository struct {
	db            *sql.DB
	channelCache  sync.Map // int64 id -> *cachedChannel
	serverCache   sync.Map // int64 id -> *cachedServer
}

// NewRepository creates a new Repository with the given database connection.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// invalidateChannelCache drops a channel entry so the next read refreshes.
func (r *Repository) invalidateChannelCache(id int64) { r.channelCache.Delete(id) }

// invalidateServerCache drops a server entry so the next read refreshes.
func (r *Repository) invalidateServerCache(id int64) { r.serverCache.Delete(id) }

// NewRepositoryWithDSN creates a new Repository and establishes a connection using the DSN.
func NewRepositoryWithDSN(ctx context.Context, dsn string) (*Repository, error) {
	db, err := sql.Open("postgres", dsn)
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
	ID            int64
	ServerID      int64
	ActorID       *int64
	ActorUsername string
	Action        string
	TargetID      string
	TargetType    string
	TargetName    string
	Changes       []byte // raw JSONB
	Reason        string
	CreatedAt     time.Time
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
// actorID and action are optional filters (nil / "" = no filter).
// Returns (entries, totalCount, error).
func (r *Repository) ListAuditLogs(ctx context.Context, serverID int64, actorID *int64, action string, limit, offset int) ([]AuditLog, int, error) {
	args := []any{serverID}
	where := `WHERE server_id = $1`
	idx := 2
	if actorID != nil {
		where += fmt.Sprintf(` AND actor_id = $%d`, idx)
		args = append(args, *actorID)
		idx++
	}
	if action != "" {
		where += fmt.Sprintf(` AND action = $%d`, idx)
		args = append(args, action)
		idx++
	}

	// total count
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_audit_logs `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// paginated rows
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, actor_id, actor_username, action,
		        COALESCE(target_id,''), COALESCE(target_type,''), COALESCE(target_name,''),
		        changes, COALESCE(reason,''), created_at
		 FROM server_audit_logs `+where+
			fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.ServerID, &l.ActorID, &l.ActorUsername,
			&l.Action, &l.TargetID, &l.TargetType, &l.TargetName,
			&l.Changes, &l.Reason, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
