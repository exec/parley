package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
)

var (
	ErrNotFound         = errors.New("record not found")
	ErrAlreadyExists    = errors.New("record already exists")
	ErrInvalidOperation = errors.New("invalid operation")
)

// Repository handles database operations for all models.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository with the given database connection.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

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
