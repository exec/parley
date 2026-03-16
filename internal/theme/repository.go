package theme

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrNotFound = errors.New("theme not found")

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetPreferences(ctx context.Context, userID int64) (*UserPreferences, error) {
	p := &UserPreferences{}
	err := r.db.QueryRowContext(ctx,
		`SELECT active_theme, active_custom_theme_id FROM user_preferences WHERE user_id=$1`,
		userID).Scan(&p.ActiveTheme, &p.ActiveCustomThemeID)
	if err != nil {
		return nil, err
	}
	themes, err := r.GetUserThemes(ctx, userID)
	if err != nil {
		return nil, err
	}
	p.CustomThemes = themes
	return p, nil
}

func (r *Repository) SetActiveTheme(ctx context.Context, userID int64, theme string, customThemeID *int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE user_preferences SET active_theme=$2, active_custom_theme_id=$3 WHERE user_id=$1`,
		userID, theme, customThemeID)
	return err
}

func (r *Repository) CountUserThemes(ctx context.Context, userID int64) (int, error) {
	var n int
	return n, r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_themes WHERE user_id=$1`, userID).Scan(&n)
}

func (r *Repository) CreateTheme(ctx context.Context, userID int64, name, css string, bgURL *string) (*UserTheme, error) {
	t := &UserTheme{}
	return t, r.db.QueryRowContext(ctx,
		`INSERT INTO user_themes (user_id,name,css,background_url)
		 VALUES ($1,$2,$3,$4)
		 RETURNING id,user_id,name,css,background_url,share_token,created_at`,
		userID, name, css, bgURL,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BackgroundURL, &t.ShareToken, &t.CreatedAt)
}

func (r *Repository) UpdateTheme(ctx context.Context, id, userID int64, name, css string, bgURL *string) (*UserTheme, error) {
	t := &UserTheme{}
	err := r.db.QueryRowContext(ctx,
		`UPDATE user_themes SET name=$3,css=$4,background_url=$5
		 WHERE id=$1 AND user_id=$2
		 RETURNING id,user_id,name,css,background_url,share_token,created_at`,
		id, userID, name, css, bgURL,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BackgroundURL, &t.ShareToken, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (r *Repository) DeleteTheme(ctx context.Context, id, userID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Atomically reset preference if this was the active theme
	if _, err = tx.ExecContext(ctx,
		`UPDATE user_preferences SET active_theme='rory', active_custom_theme_id=NULL
		 WHERE user_id=$1 AND active_custom_theme_id=$2`, userID, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`DELETE FROM user_themes WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (r *Repository) GetUserThemes(ctx context.Context, userID int64) ([]UserTheme, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,user_id,name,css,background_url,share_token,created_at
		 FROM user_themes WHERE user_id=$1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserTheme
	for rows.Next() {
		var t UserTheme
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BackgroundURL, &t.ShareToken, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []UserTheme{}
	}
	return out, rows.Err()
}

func (r *Repository) GenerateShareToken(ctx context.Context, id, userID int64) (string, error) {
	var token string
	err := r.db.QueryRowContext(ctx,
		`UPDATE user_themes
		 SET share_token=COALESCE(share_token, gen_random_uuid())
		 WHERE id=$1 AND user_id=$2
		 RETURNING share_token`, id, userID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return token, err
}

func (r *Repository) GetThemeByToken(ctx context.Context, token string) (*UserTheme, error) {
	t := &UserTheme{}
	err := r.db.QueryRowContext(ctx,
		`SELECT t.id, t.user_id, t.name, t.css, t.background_url, t.share_token,
		        t.created_at, u.username
		 FROM user_themes t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.share_token=$1`, token,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BackgroundURL, &t.ShareToken,
		&t.CreatedAt, &t.AuthorUsername)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (r *Repository) InstallTheme(ctx context.Context, token string, userID int64) (*UserTheme, error) {
	src, err := r.GetThemeByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	name := src.Name // verbatim copy, no "(copy)" suffix
	t := &UserTheme{}
	return t, r.db.QueryRowContext(ctx,
		`INSERT INTO user_themes (user_id,name,css,background_url,created_at)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id,user_id,name,css,background_url,share_token,created_at`,
		userID, name, src.CSS, src.BackgroundURL, time.Now(),
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BackgroundURL, &t.ShareToken, &t.CreatedAt)
}
