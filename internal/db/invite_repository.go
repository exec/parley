package db

import (
	"context"
	"database/sql"
	"time"
)

// ============ Invite Operations ============

func (r *Repository) CreateInvite(ctx context.Context, invite *Invite) error {
	query := `
		INSERT INTO invites (server_id, code, created_by, created_at, max_uses, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	invite.CreatedAt = time.Now()
	return r.db.QueryRowContext(ctx, query,
		invite.ServerID, invite.Code, invite.CreatedBy, invite.CreatedAt,
		invite.MaxUses, invite.ExpiresAt,
	).Scan(&invite.ID)
}

func (r *Repository) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	query := `
		SELECT id, server_id, code, created_by, created_at, max_uses, expires_at, use_count, revoked_at
		FROM invites WHERE code = $1
	`
	var inv Invite
	var maxUses sql.NullInt64
	var expiresAt sql.NullTime
	var revokedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&inv.ID, &inv.ServerID, &inv.Code, &inv.CreatedBy, &inv.CreatedAt,
		&maxUses, &expiresAt, &inv.UseCount, &revokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if maxUses.Valid {
		v := int(maxUses.Int64)
		inv.MaxUses = &v
	}
	if expiresAt.Valid {
		inv.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		inv.RevokedAt = &revokedAt.Time
	}
	return &inv, nil
}

func (r *Repository) GetServerByInviteCode(ctx context.Context, code string) (*Server, error) {
	// First check the invite is valid
	inv, err := r.GetInviteByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if inv.RevokedAt != nil {
		return nil, ErrNotFound
	}
	if inv.ExpiresAt != nil && time.Now().After(*inv.ExpiresAt) {
		return nil, ErrNotFound
	}
	if inv.MaxUses != nil && inv.UseCount >= *inv.MaxUses {
		return nil, ErrNotFound
	}

	// Increment use_count
	_, err = r.db.ExecContext(ctx, `UPDATE invites SET use_count = use_count + 1 WHERE code = $1`, code)
	if err != nil {
		return nil, err
	}

	var server Server
	err = r.db.QueryRowContext(ctx, `
		SELECT s.id, s.name, s.icon_url, s.owner_id, s.vanity_url, s.created_at, s.updated_at
		FROM servers s
		INNER JOIN invites i ON s.id = i.server_id
		WHERE i.code = $1
	`, code).Scan(&server.ID, &server.Name, &server.IconURL, &server.OwnerID, &server.VanityURL, &server.CreatedAt, &server.UpdatedAt)
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

// InviteWithCreator enriches an Invite with the creator's username
type InviteWithCreator struct {
	Invite
	CreatorUsername string `json:"creator_username"`
}

func (r *Repository) GetServerInvites(ctx context.Context, serverID int64) ([]*InviteWithCreator, error) {
	query := `
		SELECT i.id, i.server_id, i.code, i.created_by, i.created_at,
		       i.max_uses, i.expires_at, i.use_count, i.revoked_at,
		       u.username
		FROM invites i
		JOIN users u ON u.id = i.created_by
		WHERE i.server_id = $1 AND i.revoked_at IS NULL
		ORDER BY i.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*InviteWithCreator
	for rows.Next() {
		var iwc InviteWithCreator
		var maxUses sql.NullInt64
		var expiresAt sql.NullTime
		var revokedAt sql.NullTime
		err := rows.Scan(
			&iwc.ID, &iwc.ServerID, &iwc.Code, &iwc.CreatedBy, &iwc.CreatedAt,
			&maxUses, &expiresAt, &iwc.UseCount, &revokedAt,
			&iwc.CreatorUsername,
		)
		if err != nil {
			return nil, err
		}
		if maxUses.Valid {
			v := int(maxUses.Int64)
			iwc.MaxUses = &v
		}
		if expiresAt.Valid {
			iwc.ExpiresAt = &expiresAt.Time
		}
		if revokedAt.Valid {
			iwc.RevokedAt = &revokedAt.Time
		}
		results = append(results, &iwc)
	}
	return results, rows.Err()
}

func (r *Repository) RevokeInvite(ctx context.Context, code string, serverID int64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE invites SET revoked_at = NOW() WHERE code = $1 AND server_id = $2 AND revoked_at IS NULL`,
		code, serverID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// InviteMember is a member who joined via an invite
type InviteMember struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	JoinedAt    time.Time `json:"joined_at"`
}

func (r *Repository) GetMembersByInviteCode(ctx context.Context, code string, serverID int64) ([]*InviteMember, error) {
	query := `
		SELECT sm.user_id, u.username, COALESCE(u.display_name, '') as display_name,
		       COALESCE(u.avatar_url, '') as avatar_url, sm.joined_at
		FROM server_members sm
		JOIN users u ON u.id = sm.user_id
		WHERE sm.invite_code = $1 AND sm.server_id = $2
		ORDER BY sm.joined_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, code, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []*InviteMember
	for rows.Next() {
		var m InviteMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.AvatarURL, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, rows.Err()
}
