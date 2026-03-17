package theme

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("theme not found")
var ErrAlreadyInstalled = errors.New("theme already installed")

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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
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
		`INSERT INTO user_preferences (user_id, active_theme, active_custom_theme_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE
		   SET active_theme=$2, active_custom_theme_id=$3`,
		userID, theme, customThemeID)
	return err
}

func (r *Repository) CountUserThemes(ctx context.Context, userID int64) (int, error) {
	var n int
	return n, r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_themes WHERE user_id=$1`, userID).Scan(&n)
}

func (r *Repository) CreateTheme(ctx context.Context, userID int64, name, css, baseTheme string, bgURL *string) (*UserTheme, error) {
	t := &UserTheme{}
	return t, r.db.QueryRowContext(ctx,
		`INSERT INTO user_themes (user_id,name,css,base_theme,background_url)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id,user_id,name,css,base_theme,background_url,share_token,source_share_token,is_published,is_featured,created_at`,
		userID, name, css, baseTheme, bgURL,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL, &t.ShareToken, &t.SourceShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt)
}

func (r *Repository) UpdateTheme(ctx context.Context, id, userID int64, name, css, baseTheme string, bgURL *string) (*UserTheme, error) {
	t := &UserTheme{}
	err := r.db.QueryRowContext(ctx,
		`UPDATE user_themes SET name=$3,css=$4,base_theme=$5,background_url=$6
		 WHERE id=$1 AND user_id=$2
		 RETURNING id,user_id,name,css,base_theme,background_url,share_token,source_share_token,is_published,is_featured,created_at`,
		id, userID, name, css, baseTheme, bgURL,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL, &t.ShareToken, &t.SourceShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt)
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
		`SELECT id,user_id,name,css,base_theme,background_url,share_token,source_share_token,is_published,is_featured,created_at
		 FROM user_themes WHERE user_id=$1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserTheme
	for rows.Next() {
		var t UserTheme
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL, &t.ShareToken, &t.SourceShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt); err != nil {
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
		`SELECT t.id, t.user_id, t.name, t.css, t.base_theme, t.background_url, t.share_token,
		        t.source_share_token, t.is_published, t.is_featured, t.created_at,
		        u.username, COALESCE(u.display_name, '')
		 FROM user_themes t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.share_token=$1`, token,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL, &t.ShareToken,
		&t.SourceShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt, &t.AuthorUsername, &t.AuthorDisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (r *Repository) ThemeBelongsToUser(ctx context.Context, id int64, userID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_themes WHERE id=$1 AND user_id=$2`, id, userID).Scan(&count)
	return count > 0, err
}

func (r *Repository) InstallTheme(ctx context.Context, token string, userID int64) (*UserTheme, error) {
	src, err := r.GetThemeByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	// Reject duplicate installs
	var existing int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_themes WHERE user_id=$1 AND source_share_token=$2::uuid`,
		userID, token).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrAlreadyInstalled
	}
	t := &UserTheme{}
	return t, r.db.QueryRowContext(ctx,
		`INSERT INTO user_themes (user_id,name,css,base_theme,background_url,source_share_token)
		 VALUES ($1,$2,$3,$4,$5,$6::uuid)
		 RETURNING id,user_id,name,css,base_theme,background_url,share_token,source_share_token,is_published,is_featured,created_at`,
		userID, src.Name, src.CSS, src.BaseTheme, src.BackgroundURL, token,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL, &t.ShareToken, &t.SourceShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt)
}

// SetPublished publishes or unpublishes a theme. When publishing, auto-generates share_token if not set.
func (r *Repository) SetPublished(ctx context.Context, id, userID int64, publish bool) error {
	var res sql.Result
	var err error
	if publish {
		res, err = r.db.ExecContext(ctx,
			`UPDATE user_themes
			 SET is_published=TRUE, share_token=COALESCE(share_token, gen_random_uuid())
			 WHERE id=$1 AND user_id=$2`, id, userID)
	} else {
		res, err = r.db.ExecContext(ctx,
			`UPDATE user_themes SET is_published=FALSE WHERE id=$1 AND user_id=$2`,
			id, userID)
	}
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFeatured marks or unmarks a theme as featured (admin only — caller must verify).
func (r *Repository) SetFeatured(ctx context.Context, id int64, featured bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE user_themes SET is_featured=$1 WHERE id=$2`, featured, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPublishedThemes returns published themes, featured first then newest first.
func (r *Repository) GetPublishedThemes(ctx context.Context, limit, offset int) ([]UserTheme, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.user_id, t.name, t.css, t.base_theme, t.background_url,
		        t.share_token, t.source_share_token, t.is_published, t.is_featured, t.created_at,
		        u.username, COALESCE(u.display_name, '')
		 FROM user_themes t
		 JOIN users u ON u.id = t.user_id
		 WHERE t.is_published = TRUE
		 ORDER BY t.is_featured DESC, t.created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserTheme
	for rows.Next() {
		var t UserTheme
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL,
			&t.ShareToken, &t.SourceShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt,
			&t.AuthorUsername, &t.AuthorDisplayName,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []UserTheme{}
	}
	return out, rows.Err()
}

// GetPublishedThemeCount returns the total number of published themes.
func (r *Repository) GetPublishedThemeCount(ctx context.Context) (int, error) {
	var n int
	return n, r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_themes WHERE is_published = TRUE`).Scan(&n)
}

// GetThemeByID returns a theme by ID (no user ownership check — for admin operations).
func (r *Repository) GetThemeByID(ctx context.Context, id int64) (*UserTheme, error) {
	t := &UserTheme{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, css, base_theme, background_url, share_token,
		        is_published, is_featured, created_at
		 FROM user_themes WHERE id=$1`, id,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CSS, &t.BaseTheme, &t.BackgroundURL,
		&t.ShareToken, &t.IsPublished, &t.IsFeatured, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// GetUserBadges returns the badges bitmask for a user.
func (r *Repository) GetUserBadges(ctx context.Context, userID int64) (int, error) {
	var badges int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(badges, 0) FROM users WHERE id=$1`, userID).Scan(&badges)
	return badges, err
}
