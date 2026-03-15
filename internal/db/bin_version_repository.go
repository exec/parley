package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ============ File Operations ============

// CreateBinPostFiles batch-inserts files for a bin post and returns them with IDs.
func (r *Repository) CreateBinPostFiles(ctx context.Context, postID int64, files []BinPostFile) ([]BinPostFile, error) {
	if len(files) == 0 {
		return nil, nil
	}

	valueStrings := make([]string, 0, len(files))
	args := []interface{}{}
	for i, f := range files {
		base := i * 5
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4, base+5))
		args = append(args, postID, f.Filename, f.Language, f.Content, f.Position)
	}

	query := fmt.Sprintf(
		`INSERT INTO bin_post_files (post_id, filename, language, content, position) VALUES %s RETURNING id`,
		strings.Join(valueStrings, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]BinPostFile, len(files))
	copy(result, files)
	i := 0
	for rows.Next() {
		if err := rows.Scan(&result[i].ID); err != nil {
			return nil, err
		}
		result[i].PostID = postID
		i++
	}
	return result, rows.Err()
}

// GetBinPostFiles retrieves all files for a bin post ordered by position.
func (r *Repository) GetBinPostFiles(ctx context.Context, postID int64) ([]BinPostFile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, post_id, filename, language, content, position
		 FROM bin_post_files
		 WHERE post_id = $1
		 ORDER BY position`,
		postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []BinPostFile
	for rows.Next() {
		var f BinPostFile
		if err := rows.Scan(&f.ID, &f.PostID, &f.Filename, &f.Language, &f.Content, &f.Position); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// ReplaceBinPostFiles deletes all existing files for a post and inserts new ones.
func (r *Repository) ReplaceBinPostFiles(ctx context.Context, postID int64, files []BinPostFile) ([]BinPostFile, error) {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bin_post_files WHERE post_id = $1`, postID)
	if err != nil {
		return nil, err
	}
	return r.CreateBinPostFiles(ctx, postID, files)
}

// ============ Version Operations ============

// CreateBinPostVersion inserts a new version record for a bin post.
func (r *Repository) CreateBinPostVersion(ctx context.Context, postID int64, version int, description string) (*BinPostVersion, error) {
	v := &BinPostVersion{
		PostID:      postID,
		Version:     version,
		Description: description,
	}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO bin_post_versions (post_id, version, description)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		postID, version, description,
	).Scan(&v.ID, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// CreateBinPostVersionFiles inserts file snapshots for a version.
func (r *Repository) CreateBinPostVersionFiles(ctx context.Context, versionID int64, files []BinPostFile) error {
	if len(files) == 0 {
		return nil
	}

	valueStrings := make([]string, 0, len(files))
	args := []interface{}{}
	for i, f := range files {
		base := i * 5
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4, base+5))
		args = append(args, versionID, f.Filename, f.Language, f.Content, f.Position)
	}

	query := fmt.Sprintf(
		`INSERT INTO bin_post_version_files (version_id, filename, language, content, position) VALUES %s`,
		strings.Join(valueStrings, ", "),
	)

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetBinPostVersions retrieves all versions for a bin post ordered by version descending (without files).
func (r *Repository) GetBinPostVersions(ctx context.Context, postID int64) ([]BinPostVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, post_id, version, description, created_at
		 FROM bin_post_versions
		 WHERE post_id = $1
		 ORDER BY version DESC`,
		postID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []BinPostVersion
	for rows.Next() {
		var v BinPostVersion
		if err := rows.Scan(&v.ID, &v.PostID, &v.Version, &v.Description, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// GetBinPostVersion retrieves a single version by ID including its files.
func (r *Repository) GetBinPostVersion(ctx context.Context, versionID int64) (*BinPostVersion, error) {
	var v BinPostVersion
	err := r.db.QueryRowContext(ctx,
		`SELECT id, post_id, version, description, created_at
		 FROM bin_post_versions
		 WHERE id = $1`,
		versionID,
	).Scan(&v.ID, &v.PostID, &v.Version, &v.Description, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, version_id, filename, language, content, position
		 FROM bin_post_version_files
		 WHERE version_id = $1
		 ORDER BY position`,
		versionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var f BinPostVersionFile
		if err := rows.Scan(&f.ID, &f.VersionID, &f.Filename, &f.Language, &f.Content, &f.Position); err != nil {
			return nil, err
		}
		v.Files = append(v.Files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &v, nil
}

// GetLatestVersionNumber returns the highest version number for a post, or 0 if none.
func (r *Repository) GetLatestVersionNumber(ctx context.Context, postID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM bin_post_versions WHERE post_id = $1`,
		postID,
	).Scan(&n)
	return n, err
}

// PurgeOldBinPostVersions deletes bin post versions older than 90 days.
func (r *Repository) PurgeOldBinPostVersions(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM bin_post_versions WHERE created_at < NOW() - INTERVAL '90 days'`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
