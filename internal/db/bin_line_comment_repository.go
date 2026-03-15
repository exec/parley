package db

import (
	"context"
	"database/sql"
)

// CreateBinLineComment inserts a new line comment and returns it with author info.
func (r *Repository) CreateBinLineComment(ctx context.Context, postID, versionID, fileID int64, lineNumber int, authorID int64, content string, parentID *int64) (*BinLineComment, error) {
	c := &BinLineComment{
		PostID:     postID,
		VersionID:  versionID,
		FileID:     fileID,
		LineNumber: lineNumber,
		AuthorID:   authorID,
		Content:    content,
		ParentID:   parentID,
	}

	err := r.db.QueryRowContext(ctx,
		`INSERT INTO bin_line_comments (post_id, version_id, file_id, line_number, author_id, content, parent_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at, updated_at`,
		postID, versionID, fileID, lineNumber, authorID, content, parentID,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Fetch author info.
	err = r.db.QueryRowContext(ctx,
		`SELECT username, COALESCE(avatar_url, '') FROM users WHERE id = $1`, authorID,
	).Scan(&c.AuthorUsername, &c.AuthorAvatarURL)
	if err != nil {
		return nil, err
	}

	return c, nil
}

// GetBinLineComments retrieves line comments for a post, optionally filtered by version and file.
func (r *Repository) GetBinLineComments(ctx context.Context, postID int64, versionID, fileID *int64) ([]BinLineComment, error) {
	query := `
		SELECT
			lc.id, lc.post_id, lc.version_id, lc.file_id, lc.line_number,
			lc.author_id, lc.content, lc.parent_id, lc.created_at, lc.updated_at,
			u.username, COALESCE(u.avatar_url, '')
		FROM bin_line_comments lc
		JOIN users u ON u.id = lc.author_id
		WHERE lc.post_id = $1`

	args := []interface{}{postID}

	if versionID != nil {
		args = append(args, *versionID)
		query += ` AND lc.version_id = $2`
	}
	if fileID != nil {
		args = append(args, *fileID)
		if versionID != nil {
			query += ` AND lc.file_id = $3`
		} else {
			query += ` AND lc.file_id = $2`
		}
	}

	query += ` ORDER BY lc.created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []BinLineComment
	for rows.Next() {
		var c BinLineComment
		if err := rows.Scan(
			&c.ID, &c.PostID, &c.VersionID, &c.FileID, &c.LineNumber,
			&c.AuthorID, &c.Content, &c.ParentID, &c.CreatedAt, &c.UpdatedAt,
			&c.AuthorUsername, &c.AuthorAvatarURL,
		); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// UpdateBinLineComment updates the content of a line comment and returns it with author info.
func (r *Repository) UpdateBinLineComment(ctx context.Context, commentID int64, content string) (*BinLineComment, error) {
	var c BinLineComment
	err := r.db.QueryRowContext(ctx,
		`UPDATE bin_line_comments
		 SET content = $1, updated_at = NOW()
		 WHERE id = $2
		 RETURNING id, post_id, version_id, file_id, line_number, author_id, content, parent_id, created_at, updated_at`,
		content, commentID,
	).Scan(
		&c.ID, &c.PostID, &c.VersionID, &c.FileID, &c.LineNumber,
		&c.AuthorID, &c.Content, &c.ParentID, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT username, COALESCE(avatar_url, '') FROM users WHERE id = $1`, c.AuthorID,
	).Scan(&c.AuthorUsername, &c.AuthorAvatarURL)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

// DeleteBinLineComment deletes a line comment by ID.
func (r *Repository) DeleteBinLineComment(ctx context.Context, commentID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM bin_line_comments WHERE id = $1`, commentID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetBinLineCommentAuthorID returns the author_id of a line comment.
func (r *Repository) GetBinLineCommentAuthorID(ctx context.Context, commentID int64) (int64, error) {
	var authorID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT author_id FROM bin_line_comments WHERE id = $1`, commentID,
	).Scan(&authorID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return authorID, err
}
