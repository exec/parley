package db

import (
	"context"
	"fmt"

	"github.com/lib/pq"
)

// GetServerCategories returns all server categories ordered by name.
func (r *Repository) GetServerCategories(ctx context.Context) ([]ServerCategory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM server_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []ServerCategory
	for rows.Next() {
		var c ServerCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// CreateServerCategory inserts a new server category and returns it.
func (r *Repository) CreateServerCategory(ctx context.Context, name string) (*ServerCategory, error) {
	var c ServerCategory
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO server_categories (name) VALUES ($1) RETURNING id, name, created_at`,
		name,
	).Scan(&c.ID, &c.Name, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteServerCategory removes a server category by ID.
func (r *Repository) DeleteServerCategory(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM server_categories WHERE id = $1`, id)
	return err
}

// GetServerCategoryAssignments returns the categories assigned to a server.
func (r *Repository) GetServerCategoryAssignments(ctx context.Context, serverID int64) ([]ServerCategory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sc.id, sc.name, sc.created_at
		FROM server_category_assignments sca
		JOIN server_categories sc ON sc.id = sca.category_id
		WHERE sca.server_id = $1
		ORDER BY sc.name`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []ServerCategory
	for rows.Next() {
		var c ServerCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// GetBulkServerCategoryAssignments returns the categories assigned to each of the given server IDs.
// The result is keyed by server ID.
func (r *Repository) GetBulkServerCategoryAssignments(ctx context.Context, serverIDs []int64) (map[int64][]ServerCategory, error) {
	if len(serverIDs) == 0 {
		return map[int64][]ServerCategory{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT sca.server_id, sc.id, sc.name, sc.created_at
		FROM server_category_assignments sca
		JOIN server_categories sc ON sc.id = sca.category_id
		WHERE sca.server_id = ANY($1)
		ORDER BY sc.name`,
		pq.Array(serverIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64][]ServerCategory, len(serverIDs))
	for rows.Next() {
		var serverID int64
		var c ServerCategory
		if err := rows.Scan(&serverID, &c.ID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		result[serverID] = append(result[serverID], c)
	}
	return result, rows.Err()
}

// SetServerCategories replaces all category assignments for a server in a transaction.
func (r *Repository) SetServerCategories(ctx context.Context, serverID int64, categoryIDs []int64) error {
	if len(categoryIDs) > 3 {
		return fmt.Errorf("maximum 3 categories allowed")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM server_category_assignments WHERE server_id = $1`, serverID,
	); err != nil {
		return err
	}

	for _, catID := range categoryIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO server_category_assignments (server_id, category_id) VALUES ($1, $2)`,
			serverID, catID,
		); err != nil {
			return fmt.Errorf("invalid category")
		}
	}
	return tx.Commit()
}

// GetPublicServers returns paginated public servers with optional name search and category filter.
func (r *Repository) GetPublicServers(ctx context.Context, categoryID *int64, q string, limit, offset int) ([]PublicServerRow, int, error) {
	const baseWhere = `
		WHERE s.is_public = TRUE
		  AND ($1::BIGINT IS NULL OR EXISTS (
		      SELECT 1 FROM server_category_assignments sca
		      WHERE sca.server_id = s.id AND sca.category_id = $1
		  ))
		  AND ($2 = '' OR s.name ILIKE '%' || $2 || '%')`

	countQuery := `SELECT COUNT(*) FROM servers s` + baseWhere
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, categoryID, q).Scan(&total); err != nil {
		return nil, 0, err
	}

	rowQuery := `
		SELECT s.id, s.name, s.icon_url, s.vanity_url, s.description,
		       (SELECT COUNT(*) FROM server_members sm WHERE sm.server_id = s.id) AS member_count
		FROM servers s` + baseWhere + `
		ORDER BY s.name
		LIMIT $3 OFFSET $4`

	rows, err := r.db.QueryContext(ctx, rowQuery, categoryID, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []PublicServerRow
	for rows.Next() {
		var row PublicServerRow
		if err := rows.Scan(
			&row.ID, &row.Name, &row.IconURL, &row.VanityURL,
			&row.Description, &row.MemberCount,
		); err != nil {
			return nil, 0, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}
