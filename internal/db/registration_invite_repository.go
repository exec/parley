package db

import (
	"context"
	"database/sql"
)

// ============ Registration Invite Operations ============

// GetUserInviteCount returns how many more invite codes the user is allowed
// to generate.
func (r *Repository) GetUserInviteCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT invite_count FROM users WHERE id = $1`, userID,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return count, err
}

// CreateRegistrationInvite atomically decrements the user's invite_count and
// inserts a new code. Returns ErrInvalidOperation if the user has no invites
// left.
func (r *Repository) CreateRegistrationInvite(ctx context.Context, inviterID int64, code string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET invite_count = invite_count - 1
		 WHERE id = $1 AND invite_count > 0`, inviterID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidOperation
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO registration_invites (code, inviter_id) VALUES ($1, $2)`,
		code, inviterID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// ListUserRegistrationInvites returns all invite codes the user has generated,
// used and unused, newest first, with the invitee's username when available.
func (r *Repository) ListUserRegistrationInvites(ctx context.Context, userID int64) ([]RegistrationInvite, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT ri.code, ri.inviter_id, ri.invitee_id, ri.created_at, ri.used_at,
		       COALESCE(u.username, '')
		FROM registration_invites ri
		LEFT JOIN users u ON u.id = ri.invitee_id
		WHERE ri.inviter_id = $1
		ORDER BY ri.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RegistrationInvite
	for rows.Next() {
		var ri RegistrationInvite
		var invitee sql.NullInt64
		var usedAt sql.NullTime
		if err := rows.Scan(&ri.Code, &ri.InviterID, &invitee, &ri.CreatedAt, &usedAt, &ri.InviteeUsername); err != nil {
			return nil, err
		}
		if invitee.Valid {
			v := invitee.Int64
			ri.InviteeID = &v
		}
		if usedAt.Valid {
			t := usedAt.Time
			ri.UsedAt = &t
		}
		out = append(out, ri)
	}
	return out, rows.Err()
}

// ConsumeRegistrationInvite marks an invite as used by the given invitee.
// Must be called from inside an open transaction because the caller creates
// the user and this row together. Returns ErrInvalidOperation if the code
// doesn't exist or has already been used.
func (r *Repository) ConsumeRegistrationInvite(ctx context.Context, tx *sql.Tx, code string, inviteeID int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE registration_invites
		    SET invitee_id = $2, used_at = NOW()
		  WHERE code = $1 AND used_at IS NULL`,
		code, inviteeID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidOperation
	}
	return nil
}

// RegistrationInviteExists returns true if the code is present in
// registration_invites (used or unused). Lightweight probe for UI
// validation before the actual registration call.
func (r *Repository) RegistrationInviteUnused(ctx context.Context, code string) (bool, error) {
	var unused bool
	err := r.db.QueryRowContext(ctx,
		`SELECT used_at IS NULL FROM registration_invites WHERE code = $1`, code,
	).Scan(&unused)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return unused, nil
}

// AdminAddUserInvites adds N invites to a single user. Caller is responsible
// for range-checking N (the admin handlers cap at 10).
func (r *Repository) AdminAddUserInvites(ctx context.Context, userID int64, count int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET invite_count = invite_count + $2 WHERE id = $1`,
		userID, count,
	)
	return err
}

// AdminBulkAddInvites adds N invites to every non-system, non-bot user.
// Returns the number of users updated.
func (r *Repository) AdminBulkAddInvites(ctx context.Context, count int) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET invite_count = invite_count + $1
		 WHERE is_system = FALSE AND is_bot = FALSE AND banned_at IS NULL`,
		count,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
