package db

import (
	"context"
	"database/sql"
	"errors"
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

// RunMigrations executes all database migrations.
// Each migration is run individually; errors are treated as warnings (already applied).
func (r *Repository) RunMigrations(ctx context.Context) error {
	for _, sql := range Migrations {
		if _, err := r.db.ExecContext(ctx, sql); err != nil {
			log.Printf("migration warning (may already be applied): %v", err)
		}
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
