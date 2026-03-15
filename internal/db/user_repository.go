package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ============ User Operations ============

func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (username, email, password_hash, avatar_url, banner_url, email_verification_token, phone_number, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9)
		RETURNING id
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	err := r.db.QueryRowContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
		user.BannerURL,
		user.EmailVerificationToken,
		user.PhoneNumber,
		user.CreatedAt,
		user.UpdatedAt,
	).Scan(&user.ID)

	return err
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''), COALESCE(display_name, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system, badges,
		       created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user User
	var bannedAt, forceLogoutAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.BannerURL,
		&user.Bio,
		&user.DisplayName,
		&user.EmailVerified,
		&user.EmailVerificationToken,
		&user.PhoneNumber,
		&user.PhoneVerified,
		&bannedAt,
		&user.BanReason,
		&forceLogoutAt,
		&user.IsSystem,
		&user.Badges,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bannedAt.Valid {
		user.BannedAt = &bannedAt.Time
	}
	if forceLogoutAt.Valid {
		user.ForceLogoutAt = &forceLogoutAt.Time
	}

	return &user, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system,
		       created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user User
	var bannedAt, forceLogoutAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.BannerURL,
		&user.Bio,
		&user.EmailVerified,
		&user.EmailVerificationToken,
		&user.PhoneNumber,
		&user.PhoneVerified,
		&bannedAt,
		&user.BanReason,
		&forceLogoutAt,
		&user.IsSystem,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bannedAt.Valid {
		user.BannedAt = &bannedAt.Time
	}
	if forceLogoutAt.Valid {
		user.ForceLogoutAt = &forceLogoutAt.Time
	}

	return &user, nil
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system,
		       created_at, updated_at
		FROM users
		WHERE username = $1
	`

	var user User
	var bannedAt, forceLogoutAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.BannerURL,
		&user.Bio,
		&user.EmailVerified,
		&user.EmailVerificationToken,
		&user.PhoneNumber,
		&user.PhoneVerified,
		&bannedAt,
		&user.BanReason,
		&forceLogoutAt,
		&user.IsSystem,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bannedAt.Valid {
		user.BannedAt = &bannedAt.Time
	}
	if forceLogoutAt.Valid {
		user.ForceLogoutAt = &forceLogoutAt.Time
	}

	return &user, nil
}

func (r *Repository) UpdateUser(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET username = $1, email = $2, password_hash = $3, avatar_url = $4, banner_url = $5,
		    email_verification_token = NULLIF($6, ''), bio = $7, display_name = $8, updated_at = $9
		WHERE id = $10
	`

	user.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
		user.BannerURL,
		user.EmailVerificationToken,
		user.Bio,
		user.DisplayName,
		user.UpdatedAt,
		user.ID,
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

func (r *Repository) GetUserByVerificationToken(ctx context.Context, token string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system,
		       created_at, updated_at
		FROM users
		WHERE email_verification_token = $1
	`

	var user User
	var bannedAt, forceLogoutAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.BannerURL,
		&user.Bio,
		&user.EmailVerified,
		&user.EmailVerificationToken,
		&user.PhoneNumber,
		&user.PhoneVerified,
		&bannedAt,
		&user.BanReason,
		&forceLogoutAt,
		&user.IsSystem,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bannedAt.Valid {
		user.BannedAt = &bannedAt.Time
	}
	if forceLogoutAt.Valid {
		user.ForceLogoutAt = &forceLogoutAt.Time
	}

	return &user, nil
}

func (r *Repository) SetEmailVerified(ctx context.Context, userID int64) error {
	query := `UPDATE users SET email_verified = TRUE, email_verification_token = NULL, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CheckAndIncrementEmailResend(ctx context.Context, userID int64) error {
	query := `
		UPDATE users
		SET
			email_resend_count = CASE
				WHEN email_resend_date IS NULL OR email_resend_date < CURRENT_DATE THEN 1
				ELSE email_resend_count + 1
			END,
			email_resend_date = CURRENT_DATE
		WHERE id = $1
		  AND (email_resend_date IS NULL OR email_resend_date < CURRENT_DATE OR email_resend_count < 3)
	`
	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrInvalidOperation
	}
	return nil
}

func (r *Repository) CheckAndIncrementEmailChange(ctx context.Context, userID int64) error {
	query := `
		UPDATE users
		SET
			email_change_count = CASE
				WHEN email_change_date IS NULL OR email_change_date < CURRENT_DATE THEN 1
				ELSE email_change_count + 1
			END,
			email_change_date = CURRENT_DATE
		WHERE id = $1
		  AND (email_change_date IS NULL OR email_change_date < CURRENT_DATE OR email_change_count < 3)
	`
	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrInvalidOperation
	}
	return nil
}

func (r *Repository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system,
		       created_at, updated_at
		FROM users
		WHERE phone_number = $1
	`
	var user User
	var bannedAt, forceLogoutAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, phone).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.AvatarURL, &user.BannerURL, &user.Bio,
		&user.EmailVerified, &user.EmailVerificationToken,
		&user.PhoneNumber, &user.PhoneVerified,
		&bannedAt, &user.BanReason, &forceLogoutAt, &user.IsSystem,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if bannedAt.Valid {
		user.BannedAt = &bannedAt.Time
	}
	if forceLogoutAt.Valid {
		user.ForceLogoutAt = &forceLogoutAt.Time
	}
	return &user, nil
}

func (r *Repository) SetPhoneVerified(ctx context.Context, userID int64) error {
	query := `UPDATE users SET phone_verified = TRUE, phone_verification_code = NULL, phone_code_expires_at = NULL, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) SetPhoneVerificationCode(ctx context.Context, userID int64, code string, expiresAt time.Time) error {
	query := `UPDATE users SET phone_verification_code = $2, phone_code_expires_at = $3, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, userID, code, expiresAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CheckPhoneVerificationCode(ctx context.Context, userID int64, code string) error {
	var storedCode sql.NullString
	var expiresAt sql.NullTime
	query := `SELECT phone_verification_code, phone_code_expires_at FROM users WHERE id = $1`
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&storedCode, &expiresAt); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if !storedCode.Valid || storedCode.String == "" {
		return ErrInvalidOperation
	}
	if !expiresAt.Valid || time.Now().After(expiresAt.Time) {
		return ErrInvalidOperation
	}
	if storedCode.String != code {
		return ErrInvalidOperation
	}
	return nil
}

func (r *Repository) CheckAndIncrementSmsResend(ctx context.Context, userID int64) error {
	query := `
		UPDATE users
		SET
			sms_resend_count = CASE
				WHEN sms_resend_date IS NULL OR sms_resend_date < CURRENT_DATE THEN 1
				ELSE sms_resend_count + 1
			END,
			sms_resend_date = CURRENT_DATE
		WHERE id = $1
		  AND (sms_resend_date IS NULL OR sms_resend_date < CURRENT_DATE OR sms_resend_count < 5)
	`
	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrInvalidOperation
	}
	return nil
}

func (r *Repository) UpdateUserPhone(ctx context.Context, userID int64, phone string) error {
	query := `
		UPDATE users
		SET phone_number = NULLIF($2, ''),
		    phone_verified = FALSE,
		    phone_verification_code = NULL,
		    phone_code_expires_at = NULL,
		    sms_resend_count = 0,
		    sms_resend_date = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`
	result, err := r.db.ExecContext(ctx, query, userID, phone)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateUserEmail(ctx context.Context, userID int64, newEmail, token string) error {
	query := `
		UPDATE users
		SET email = $2,
		    email_verified = FALSE,
		    email_verification_token = NULLIF($3, ''),
		    email_resend_count = 0,
		    email_resend_date = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`
	result, err := r.db.ExecContext(ctx, query, userID, newEmail, token)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetPublicUser(ctx context.Context, userID int64) (*PublicUser, error) {
	query := `
		SELECT id, username, COALESCE(avatar_url, ''), COALESCE(banner_url, ''), COALESCE(bio, ''), badges, created_at
		FROM users
		WHERE id = $1
	`

	var user PublicUser
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.AvatarURL,
		&user.BannerURL,
		&user.Bio,
		&user.Badges,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// escapeLike escapes LIKE/ILIKE pattern metacharacters so that user-supplied
// strings are treated as literals. The ESCAPE clause in the query must match.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func (r *Repository) SearchUsers(ctx context.Context, query string, excludeUserID int64) ([]PublicUser, error) {
	sqlQuery := `
		SELECT id, username, COALESCE(avatar_url, ''), created_at
		FROM users
		WHERE username ILIKE $1 ESCAPE '\' AND id != $2
		ORDER BY username
		LIMIT 20
	`

	rows, err := r.db.QueryContext(ctx, sqlQuery, escapeLike(query)+"%", excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []PublicUser
	for rows.Next() {
		var user PublicUser
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.AvatarURL,
			&user.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
