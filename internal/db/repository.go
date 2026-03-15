package db

import (
	"context"
	"database/sql"
	"errors"
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
func (r *Repository) RunMigrations(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, MigrationSQL())
	return err
}
