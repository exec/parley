package db

import (
	"context"
	"database/sql"
	"time"
)

// ============ Invite Operations ============

func (r *Repository) CreateInvite(ctx context.Context, invite *Invite) error {
	query := `
		INSERT INTO invites (server_id, code, created_by, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	invite.CreatedAt = time.Now()

	err := r.db.QueryRowContext(ctx, query,
		invite.ServerID,
		invite.Code,
		invite.CreatedBy,
		invite.CreatedAt,
	).Scan(&invite.ID)

	return err
}

func (r *Repository) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	query := `
		SELECT id, server_id, code, created_by, created_at
		FROM invites
		WHERE code = $1
	`

	var invite Invite
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&invite.ID,
		&invite.ServerID,
		&invite.Code,
		&invite.CreatedBy,
		&invite.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &invite, nil
}

func (r *Repository) GetServerByInviteCode(ctx context.Context, code string) (*Server, error) {
	query := `
		SELECT s.id, s.name, s.icon_url, s.owner_id, s.vanity_url, s.created_at, s.updated_at
		FROM servers s
		INNER JOIN invites i ON s.id = i.server_id
		WHERE i.code = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&server.ID,
		&server.Name,
		&server.IconURL,
		&server.OwnerID,
		&server.VanityURL,
		&server.CreatedAt,
		&server.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &server, nil
}

// InviteCodeExists returns true if the given code is already used as an invite code or a server vanity URL
// (excluding the given serverID when checking vanity URLs, so a server can keep its own slug).
func (r *Repository) InviteCodeExists(ctx context.Context, code string, excludeServerID ...int64) (bool, error) {
	exclude := int64(0)
	if len(excludeServerID) > 0 {
		exclude = excludeServerID[0]
	}
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM invites WHERE code = $1
			UNION ALL
			SELECT 1 FROM servers WHERE vanity_url = $1 AND id != $2
		)`, code, exclude).Scan(&exists)
	return exists, err
}

func (r *Repository) GetServerByVanityURL(ctx context.Context, vanityURL string) (*Server, error) {
	query := `
		SELECT id, name, icon_url, owner_id, vanity_url, created_at, updated_at
		FROM servers
		WHERE vanity_url = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, vanityURL).Scan(
		&server.ID,
		&server.Name,
		&server.IconURL,
		&server.OwnerID,
		&server.VanityURL,
		&server.CreatedAt,
		&server.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &server, nil
}

func (r *Repository) SetVanityURL(ctx context.Context, serverID int64, vanityURL sql.NullString) error {
	query := `UPDATE servers SET vanity_url = $1, updated_at = NOW() WHERE id = $2`
	result, err := r.db.ExecContext(ctx, query, vanityURL, serverID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteInvite(ctx context.Context, code string) error {
	query := `DELETE FROM invites WHERE code = $1`

	result, err := r.db.ExecContext(ctx, query, code)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
