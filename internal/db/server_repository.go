package db

import (
	"context"
	"database/sql"
	"time"
)

// ============ Server Operations ============

func (r *Repository) CreateServer(ctx context.Context, server *Server) error {
	query := `
		INSERT INTO servers (name, icon_url, owner_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	now := time.Now()
	server.CreatedAt = now
	server.UpdatedAt = now

	err := r.db.QueryRowContext(ctx, query,
		server.Name,
		server.IconURL,
		server.OwnerID,
		server.CreatedAt,
		server.UpdatedAt,
	).Scan(&server.ID)

	return err
}

func (r *Repository) GetServerByID(ctx context.Context, id int64) (*Server, error) {
	query := `
		SELECT id, name, icon_url, owner_id, vanity_url, created_at, updated_at
		FROM servers
		WHERE id = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, id).Scan(
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

func (r *Repository) GetServersByUserID(ctx context.Context, userID int64) ([]*Server, error) {
	query := `
		SELECT s.id, s.name, s.icon_url, s.owner_id, s.vanity_url, s.created_at, s.updated_at
		FROM servers s
		INNER JOIN server_members sm ON s.id = sm.server_id
		WHERE sm.user_id = $1
		ORDER BY s.name
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []*Server
	for rows.Next() {
		var server Server
		err := rows.Scan(
			&server.ID,
			&server.Name,
			&server.IconURL,
			&server.OwnerID,
			&server.VanityURL,
			&server.CreatedAt,
			&server.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		servers = append(servers, &server)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return servers, nil
}

func (r *Repository) UpdateServer(ctx context.Context, server *Server) error {
	query := `
		UPDATE servers
		SET name = $1, icon_url = $2, owner_id = $3, vanity_url = $4,
		    description = $5, is_public = $6, updated_at = NOW()
		WHERE id = $7
	`

	result, err := r.db.ExecContext(ctx, query,
		server.Name,
		server.IconURL,
		server.OwnerID,
		server.VanityURL,
		server.Description,
		server.IsPublic,
		server.ID,
	)
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

func (r *Repository) DeleteServer(ctx context.Context, id int64) error {
	query := `DELETE FROM servers WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
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

// ============ ServerMember Operations ============

func (r *Repository) AddMember(ctx context.Context, member *ServerMember) error {
	query := `
		INSERT INTO server_members (server_id, user_id, nickname, joined_at, invite_code)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	member.JoinedAt = time.Now()

	var inviteCode sql.NullString
	if member.InviteCode != "" {
		inviteCode = sql.NullString{String: member.InviteCode, Valid: true}
	}

	err := r.db.QueryRowContext(ctx, query,
		member.ServerID,
		member.UserID,
		member.Nickname,
		member.JoinedAt,
		inviteCode,
	).Scan(&member.ID)

	return err
}

func (r *Repository) RemoveMember(ctx context.Context, serverID, userID int64) error {
	query := `DELETE FROM server_members WHERE server_id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, serverID, userID)
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

func (r *Repository) GetMember(ctx context.Context, serverID, userID int64) (*ServerMember, error) {
	query := `
		SELECT id, server_id, user_id, nickname, joined_at
		FROM server_members
		WHERE server_id = $1 AND user_id = $2
	`

	var member ServerMember
	err := r.db.QueryRowContext(ctx, query, serverID, userID).Scan(
		&member.ID,
		&member.ServerID,
		&member.UserID,
		&member.Nickname,
		&member.JoinedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &member, nil
}

func (r *Repository) GetServerMembers(ctx context.Context, serverID int64) ([]*ServerMember, error) {
	query := `
		SELECT sm.id, sm.server_id, sm.user_id, sm.nickname, sm.joined_at,
		       u.username, COALESCE(u.display_name, ''), COALESCE(u.avatar_url, ''),
		       COALESCE(u.banner_url, ''), COALESCE(u.bio, ''), u.badges,
		       u.is_bot, COALESCE(sb.is_degraded, FALSE),
		       COALESCE(u.status_type, 'online'), COALESCE(u.status_text, '')
		FROM server_members sm
		JOIN users u ON u.id = sm.user_id
		LEFT JOIN server_bots sb ON sb.server_id = sm.server_id AND sb.bot_user_id = sm.user_id
		WHERE sm.server_id = $1
		ORDER BY sm.joined_at
	`

	rows, err := r.db.QueryContext(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*ServerMember
	for rows.Next() {
		var member ServerMember
		err := rows.Scan(
			&member.ID,
			&member.ServerID,
			&member.UserID,
			&member.Nickname,
			&member.JoinedAt,
			&member.Username,
			&member.DisplayName,
			&member.AvatarURL,
			&member.BannerURL,
			&member.Bio,
			&member.Badges,
			&member.IsBot,
			&member.BotDegraded,
			&member.StatusType,
			&member.StatusText,
		)
		if err != nil {
			return nil, err
		}
		members = append(members, &member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

func (r *Repository) GetMembersByRole(ctx context.Context, serverID, roleID int64) ([]*ServerMember, error) {
	query := `
		SELECT sm.id, sm.server_id, sm.user_id, sm.nickname, sm.joined_at
		FROM server_members sm
		JOIN server_member_roles smr ON smr.server_id = sm.server_id AND smr.user_id = sm.user_id
		WHERE smr.server_id = $1 AND smr.role_id = $2
	`

	rows, err := r.db.QueryContext(ctx, query, serverID, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*ServerMember
	for rows.Next() {
		var member ServerMember
		err := rows.Scan(
			&member.ID,
			&member.ServerID,
			&member.UserID,
			&member.Nickname,
			&member.JoinedAt,
		)
		if err != nil {
			return nil, err
		}
		members = append(members, &member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}
