package db

import (
	"context"
	"database/sql"
	"time"
)

// Passkey represents a stored WebAuthn credential.
type Passkey struct {
	ID             int64
	UserID         int64
	CredentialID   []byte
	PublicKey      []byte
	SignCount      int64
	AAGUID         []byte
	Name           string
	BackupEligible bool
	BackupState    bool
	CreatedAt      time.Time
	LastUsedAt     *time.Time
}

func (r *Repository) CreatePasskey(ctx context.Context, p *Passkey) error {
	query := `
		INSERT INTO passkeys (user_id, credential_id, public_key, sign_count, aaguid, name, backup_eligible, backup_state, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		RETURNING id, created_at
	`
	return r.db.QueryRowContext(ctx, query,
		p.UserID, p.CredentialID, p.PublicKey, p.SignCount, p.AAGUID, p.Name, p.BackupEligible, p.BackupState,
	).Scan(&p.ID, &p.CreatedAt)
}

func (r *Repository) GetPasskeysByUserID(ctx context.Context, userID int64) ([]Passkey, error) {
	query := `SELECT id, user_id, credential_id, public_key, sign_count, aaguid, name, backup_eligible, backup_state, created_at, last_used_at FROM passkeys WHERE user_id = $1 ORDER BY created_at`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Passkey
	for rows.Next() {
		var p Passkey
		var lastUsed sql.NullTime
		if err := rows.Scan(&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey, &p.SignCount, &p.AAGUID, &p.Name, &p.BackupEligible, &p.BackupState, &p.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			p.LastUsedAt = &lastUsed.Time
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) GetPasskeyByCredentialID(ctx context.Context, credID []byte) (*Passkey, error) {
	query := `SELECT id, user_id, credential_id, public_key, sign_count, aaguid, name, backup_eligible, backup_state, created_at, last_used_at FROM passkeys WHERE credential_id = $1`
	var p Passkey
	var lastUsed sql.NullTime
	err := r.db.QueryRowContext(ctx, query, credID).Scan(&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey, &p.SignCount, &p.AAGUID, &p.Name, &p.BackupEligible, &p.BackupState, &p.CreatedAt, &lastUsed)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		p.LastUsedAt = &lastUsed.Time
	}
	return &p, nil
}

func (r *Repository) UpdatePasskeySignCount(ctx context.Context, id, signCount int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE passkeys SET sign_count = $2, last_used_at = NOW() WHERE id = $1`, id, signCount)
	return err
}

func (r *Repository) DeletePasskey(ctx context.Context, id, userID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM passkeys WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) RenamePasskey(ctx context.Context, id, userID int64, name string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE passkeys SET name = $3 WHERE id = $1 AND user_id = $2`, id, userID, name)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
