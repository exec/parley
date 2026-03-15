package db

import (
	"context"
)

// ============ Role Operations ============

func (r *Repository) GetServerRoles(ctx context.Context, serverID int64) ([]ServerRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, name, color, permissions, hoist, position, created_at
         FROM server_roles WHERE server_id = $1 ORDER BY position ASC, created_at ASC`,
		serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []ServerRole
	for rows.Next() {
		var role ServerRole
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *Repository) CreateServerRole(ctx context.Context, serverID int64, name, color string, permissions int64) (*ServerRole, error) {
	var role ServerRole
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO server_roles (server_id, name, color, permissions)
         VALUES ($1, $2, $3, $4)
         RETURNING id, server_id, name, color, permissions, hoist, position, created_at`,
		serverID, name, color, permissions,
	).Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
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
		 RETURNING id, server_id, name, color, permissions, hoist, position, created_at`,
		name, color, permissions, hoist, position, roleID, serverID,
	).Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) GetMemberRoles(ctx context.Context, serverID, userID int64) ([]ServerRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sr.id, sr.server_id, sr.name, sr.color, sr.permissions, sr.hoist, sr.position, sr.created_at
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
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.CreatedAt); err != nil {
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

func (r *Repository) GetServerMembersWithRoles(ctx context.Context, serverID int64) ([]*ServerMember, error) {
	members, err := r.GetServerMembers(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return members, nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT smr.user_id, sr.id, sr.server_id, sr.name, sr.color, sr.permissions, sr.hoist, sr.position, sr.created_at
         FROM server_member_roles smr
         JOIN server_roles sr ON sr.id = smr.role_id
         WHERE smr.server_id = $1
         ORDER BY sr.position ASC, sr.created_at ASC`,
		serverID)
	if err != nil {
		return members, nil // non-fatal: return members without roles
	}
	defer rows.Close()

	rolesByUser := make(map[int64][]ServerRole)
	for rows.Next() {
		var userID int64
		var role ServerRole
		if err := rows.Scan(&userID, &role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.Hoist, &role.Position, &role.CreatedAt); err != nil {
			continue
		}
		rolesByUser[userID] = append(rolesByUser[userID], role)
	}

	for i := range members {
		if roles, ok := rolesByUser[members[i].UserID]; ok {
			members[i].Roles = roles
		} else {
			members[i].Roles = []ServerRole{}
		}
	}

	return members, nil
}
