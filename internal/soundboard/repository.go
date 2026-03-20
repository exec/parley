package soundboard

import (
	"context"
	"database/sql"
)

// Repository handles all soundboard DB queries.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CountByServer returns the number of sounds for a server.
func (r *Repository) CountByServer(ctx context.Context, serverID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM soundboard_sounds WHERE server_id = $1`, serverID,
	).Scan(&n)
	return n, err
}

// Create inserts a new sound and returns it with its generated ID and created_at.
func (r *Repository) Create(ctx context.Context, s *Sound) (*Sound, error) {
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO soundboard_sounds (server_id, uploader_id, name, emoji, file_url, file_key)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		s.ServerID, s.UploaderID, s.Name, s.Emoji, s.FileURL, s.FileKey,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ListByServer returns all sounds for a server ordered by name.
func (r *Repository) ListByServer(ctx context.Context, serverID int64) ([]Sound, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, uploader_id, name, emoji, file_url, file_key, created_at
		 FROM soundboard_sounds WHERE server_id = $1 ORDER BY name`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Sound, 0)
	for rows.Next() {
		var s Sound
		var emoji sql.NullString
		if err := rows.Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &emoji,
			&s.FileURL, &s.FileKey, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Emoji = emoji.String
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetByID returns a single sound by ID.
func (r *Repository) GetByID(ctx context.Context, id int64) (*Sound, error) {
	var s Sound
	var emoji sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, server_id, uploader_id, name, emoji, file_url, file_key, created_at
		 FROM soundboard_sounds WHERE id = $1`, id,
	).Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &emoji,
		&s.FileURL, &s.FileKey, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.Emoji = emoji.String
	return &s, nil
}

// Update updates a sound's name and/or emoji.
func (r *Repository) Update(ctx context.Context, id int64, name, emoji string) (*Sound, error) {
	var s Sound
	var emojiNull sql.NullString
	err := r.db.QueryRowContext(ctx,
		`UPDATE soundboard_sounds SET name=$1, emoji=$2
		 WHERE id=$3
		 RETURNING id, server_id, uploader_id, name, emoji, file_url, file_key, created_at`,
		name, emoji, id,
	).Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &emojiNull,
		&s.FileURL, &s.FileKey, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	s.Emoji = emojiNull.String
	return &s, nil
}

// Delete removes a sound by ID and returns the file_key for Spaces cleanup.
func (r *Repository) Delete(ctx context.Context, id int64) (fileKey string, err error) {
	err = r.db.QueryRowContext(ctx,
		`DELETE FROM soundboard_sounds WHERE id=$1 RETURNING file_key`, id,
	).Scan(&fileKey)
	return fileKey, err
}

// ListForUser returns all sounds from servers the user is a member of,
// joined with server name, ordered by server name then sound name.
func (r *Repository) ListForUser(ctx context.Context, userID int64) ([]SoundWithServer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ss.id, ss.server_id, ss.uploader_id, ss.name, ss.emoji,
		        ss.file_url, ss.file_key, ss.created_at, s.name AS server_name
		 FROM soundboard_sounds ss
		 JOIN server_members sm ON sm.server_id = ss.server_id AND sm.user_id = $1
		 JOIN servers s ON s.id = ss.server_id
		 ORDER BY s.name, ss.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SoundWithServer, 0)
	for rows.Next() {
		var s SoundWithServer
		var emoji sql.NullString
		if err := rows.Scan(&s.ID, &s.ServerID, &s.UploaderID, &s.Name, &emoji,
			&s.FileURL, &s.FileKey, &s.CreatedAt, &s.ServerName); err != nil {
			return nil, err
		}
		s.Emoji = emoji.String
		out = append(out, s)
	}
	return out, rows.Err()
}
