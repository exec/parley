package db

import (
	"context"
	"encoding/json"
)

// permDefaultEveryone mirrors permissions.PermDefaultEveryone.
// = PermViewChannel | PermSendMessages | PermReadMessageHistory | PermAddReactions |
//   PermEmbedLinks | PermAttachFiles | PermConnect | PermSpeak | PermUseVAD |
//   PermChangeNickname | PermCreateInvite | PermCreatePosts
const permDefaultEveryone int64 = (1 << 16) | (1 << 17) | (1 << 23) | (1 << 20) |
	(1 << 18) | (1 << 19) | (1 << 32) | (1 << 33) | (1 << 37) |
	(1 << 7) | (1 << 8) | (1 << 29)

// ============ Role Operations ============

func (r *Repository) GetServerRoles(ctx context.Context, serverID int64) ([]ServerRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, name, color, permissions, hoist, position, is_everyone, created_at
         FROM server_roles WHERE server_id = $1 ORDER BY position ASC, created_at ASC`,
		serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []ServerRole
	for rows.Next() {
		var role ServerRole
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *Repository) CreateServerRole(ctx context.Context, serverID int64, name, color string, permissions int64) (*ServerRole, error) {
	var role ServerRole
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO server_roles (server_id, name, color, permissions, position)
         VALUES ($1, $2, $3, $4, COALESCE((SELECT MAX(position) FROM server_roles WHERE server_id = $1), 0) + 1)
         RETURNING id, server_id, name, color, permissions, hoist, position, is_everyone, created_at`,
		serverID, name, color, permissions,
	).Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// ReorderServerRoles normalizes every role position for a server in one
// transaction. Input is the ordered list of non-@everyone role IDs, top
// (highest hierarchy) first. @everyone is forced to position 0; the other
// roles get positions N, N-1, ..., 1 matching the input order. A two-phase
// update sidesteps the unique (server_id, position) index — every row is
// parked at position = -id (unique because id is) before final assignment.
func (r *Repository) ReorderServerRoles(ctx context.Context, serverID int64, orderedNonEveryoneIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE server_roles SET position = -id WHERE server_id = $1`,
		serverID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE server_roles SET position = 0 WHERE server_id = $1 AND is_everyone = TRUE`,
		serverID); err != nil {
		return err
	}
	n := len(orderedNonEveryoneIDs)
	for i, id := range orderedNonEveryoneIDs {
		pos := n - i
		if _, err := tx.ExecContext(ctx,
			`UPDATE server_roles SET position = $1 WHERE server_id = $2 AND id = $3`,
			pos, serverID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) DeleteServerRole(ctx context.Context, serverID, roleID int64) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM server_roles WHERE id = $1 AND server_id = $2`,
		roleID, serverID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateServerRole(ctx context.Context, serverID, roleID int64, name, color string, permissions int64, hoist bool, position int) (*ServerRole, error) {
	var role ServerRole
	err := r.db.QueryRowContext(ctx,
		`UPDATE server_roles SET name = $1, color = $2, permissions = $3, hoist = $4, position = $5
		 WHERE id = $6 AND server_id = $7
		 RETURNING id, server_id, name, color, permissions, hoist, position, is_everyone, created_at`,
		name, color, permissions, hoist, position, roleID, serverID,
	).Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) GetMemberRoles(ctx context.Context, serverID, userID int64) ([]ServerRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sr.id, sr.server_id, sr.name, sr.color, sr.permissions, sr.hoist, sr.position, sr.is_everyone, sr.created_at
         FROM server_roles sr
         JOIN server_member_roles smr ON smr.role_id = sr.id
         WHERE smr.server_id = $1 AND smr.user_id = $2
         ORDER BY sr.position ASC, sr.created_at ASC`,
		serverID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []ServerRole
	for rows.Next() {
		var role ServerRole
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *Repository) AssignRoleToMember(ctx context.Context, serverID, userID, roleID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_member_roles (server_id, user_id, role_id)
         VALUES ($1, $2, $3)
         ON CONFLICT DO NOTHING`,
		serverID, userID, roleID)
	return err
}

func (r *Repository) RemoveRoleFromMember(ctx context.Context, serverID, userID, roleID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM server_member_roles WHERE server_id = $1 AND user_id = $2 AND role_id = $3`,
		serverID, userID, roleID)
	return err
}

// GetEveryoneRole returns the @everyone role for a server.
func (r *Repository) GetEveryoneRole(ctx context.Context, serverID int64) (*ServerRole, error) {
	var role ServerRole
	err := r.db.QueryRowContext(ctx,
		`SELECT id, server_id, name, color, permissions, hoist, position, is_everyone, created_at
         FROM server_roles WHERE server_id = $1 AND is_everyone = TRUE`, serverID).
		Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.IsEveryone, &role.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// CreateEveryoneRole inserts the @everyone role for a new server with default permissions.
func (r *Repository) CreateEveryoneRole(ctx context.Context, serverID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_roles (server_id, name, color, permissions, hoist, position, is_everyone, created_at)
         VALUES ($1, '@everyone', '#99aab5', $2, FALSE, 0, TRUE, NOW())`,
		serverID, permDefaultEveryone)
	return err
}

// GetHighestRolePosition returns the highest position among the user's assigned roles in a server.
func (r *Repository) GetHighestRolePosition(ctx context.Context, serverID, userID int64) (int, error) {
	var pos int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sr.position), 0) FROM server_member_roles smr
         JOIN server_roles sr ON sr.id = smr.role_id
         WHERE smr.server_id = $1 AND smr.user_id = $2`, serverID, userID).Scan(&pos)
	return pos, err
}

func (r *Repository) GetServerMembersWithRoles(ctx context.Context, serverID int64) ([]*ServerMember, error) {
	query := `
		SELECT sm.id, sm.server_id, sm.user_id, sm.nickname, sm.joined_at,
		       u.username, COALESCE(u.display_name, ''), COALESCE(u.avatar_url, ''),
		       COALESCE(u.banner_url, ''), COALESCE(u.bio, ''), u.badges,
		       u.is_bot, COALESCE(sb.is_degraded, FALSE),
		       COALESCE(u.status_type, 'online'), COALESCE(u.status_text, ''),
		       COALESCE((
		           SELECT jsonb_agg(
		               jsonb_build_object(
		                   'id', sr.id,
		                   'server_id', sr.server_id,
		                   'name', sr.name,
		                   'color', sr.color,
		                   'permissions', sr.permissions,
		                   'hoist', sr.hoist,
		                   'position', sr.position,
		                   'is_everyone', sr.is_everyone,
		                   'created_at', (sr.created_at AT TIME ZONE 'UTC')
		               ) ORDER BY sr.position ASC, sr.created_at ASC
		           )
		           FROM server_member_roles smr
		           JOIN server_roles sr ON sr.id = smr.role_id
		           WHERE smr.server_id = sm.server_id AND smr.user_id = sm.user_id
		       ), '[]'::jsonb)
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
		var rolesJSON []byte
		if err := rows.Scan(
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
			&rolesJSON,
		); err != nil {
			return nil, err
		}
		member.Roles = []ServerRole{}
		if len(rolesJSON) > 0 {
			if err := json.Unmarshal(rolesJSON, &member.Roles); err != nil {
				return nil, err
			}
		}
		members = append(members, &member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}
