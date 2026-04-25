package db

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// ============ User Operations ============

func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	return r.insertUser(ctx, r.db, user)
}

// CreateUserTx inserts a user inside an open transaction so callers can tie
// user creation to other writes (e.g. consuming a registration invite) with
// rollback-on-failure semantics.
func (r *Repository) CreateUserTx(ctx context.Context, tx *sql.Tx, user *User) error {
	return r.insertUser(ctx, tx, user)
}

type sqlQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func (r *Repository) insertUser(ctx context.Context, q sqlQuerier, user *User) error {
	query := `
		INSERT INTO users (username, email, password_hash, avatar_url, banner_url, email_verification_token, email_verification_token_expires_at, phone_number, registration_ip, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, NULLIF($6, ''), CASE WHEN NULLIF($6, '') IS NOT NULL THEN NOW() + INTERVAL '72 hours' ELSE NULL END, NULLIF($7, ''), NULLIF($8, ''), $9, $10)
		RETURNING id
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	return q.QueryRowContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
		user.BannerURL,
		user.EmailVerificationToken,
		user.PhoneNumber,
		user.RegistrationIP,
		user.CreatedAt,
		user.UpdatedAt,
	).Scan(&user.ID)
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''), COALESCE(display_name, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system, badges,
		       COALESCE(registration_ip, ''), COALESCE(last_seen_ip, ''),
		       status_type, status_text, invite_count,
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
		&user.RegistrationIP,
		&user.LastSeenIP,
		&user.StatusType,
		&user.StatusText,
		&user.InviteCount,
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
		       status_type, status_text,
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
		&user.StatusType,
		&user.StatusText,
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
		       status_type, status_text,
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
		&user.StatusType,
		&user.StatusText,
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
		    email_verification_token = NULLIF($6, ''),
		    email_verification_token_expires_at = CASE WHEN NULLIF($6, '') IS NOT NULL THEN NOW() + INTERVAL '72 hours' ELSE NULL END,
		    bio = $7, display_name = $8, updated_at = $9
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

func (r *Repository) UpdateLastSeenIP(ctx context.Context, userID int64, ip string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET last_seen_ip = NULLIF($2, '') WHERE id = $1`,
		userID, ip,
	)
	return err
}

func (r *Repository) GetUserByVerificationToken(ctx context.Context, token string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       COALESCE(bio, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system,
		       status_type, status_text,
		       created_at, updated_at
		FROM users
		WHERE email_verification_token = $1
		  AND (email_verification_token_expires_at IS NULL OR email_verification_token_expires_at > NOW())
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
		&user.StatusType,
		&user.StatusText,
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
		       status_type, status_text,
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
		&user.StatusType, &user.StatusText,
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
		    email_verification_token_expires_at = CASE WHEN NULLIF($3, '') IS NOT NULL THEN NOW() + INTERVAL '72 hours' ELSE NULL END,
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

func (r *Repository) SetPasswordResetToken(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	query := `UPDATE users SET password_reset_token = $2, password_reset_expires_at = $3, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, userID, token, expiresAt)
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

func (r *Repository) ConsumePasswordResetToken(ctx context.Context, token, newHash string) error {
	query := `
		UPDATE users
		SET password_hash = $2, password_reset_token = NULL, password_reset_expires_at = NULL, updated_at = NOW()
		WHERE password_reset_token = $1 AND password_reset_expires_at > NOW()
	`
	result, err := r.db.ExecContext(ctx, query, token, newHash)
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

func (r *Repository) UpdatePasswordHash(ctx context.Context, userID int64, hash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, userID, hash)
	return err
}

// ClearPasswordIfPasskeyExists atomically clears the user's password — writing
// the unusable sentinel hash — only when at least one passkey row is still
// associated with the user. Returns true when the UPDATE actually hit a row.
// Collapses the list-check + update pair into one statement so a concurrent
// passkey-delete cannot race us into a both-cleared state (F-auth-removepw-race).
func (r *Repository) ClearPasswordIfPasskeyExists(ctx context.Context, userID int64, sentinel string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $2, updated_at = NOW()
		 WHERE id = $1 AND EXISTS (SELECT 1 FROM passkeys WHERE user_id = $1)`,
		userID, sentinel,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) GetPublicUser(ctx context.Context, userID int64) (*PublicUser, error) {
	query := `
		SELECT id, username, COALESCE(display_name, ''), COALESCE(avatar_url, ''), COALESCE(banner_url, ''), COALESCE(bio, ''), badges, created_at
		FROM users
		WHERE id = $1
	`

	var user PublicUser
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
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

func (r *Repository) UpdateUserStatus(ctx context.Context, userID int64, statusType, statusText string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET status_type = $2, status_text = $3, updated_at = NOW() WHERE id = $1`,
		userID, statusType, statusText,
	)
	return err
}

func (r *Repository) GetUserStatusType(ctx context.Context, userID int64) (string, error) {
	var statusType string
	err := r.db.QueryRowContext(ctx, `SELECT status_type FROM users WHERE id = $1`, userID).Scan(&statusType)
	if err != nil {
		return "online", err
	}
	return statusType, nil
}

// SetUserStatusType sets status_type unconditionally for the given user.
// Used by the hub on last WS disconnect.
func (r *Repository) SetUserStatusType(ctx context.Context, userID int64, statusType string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET status_type = $2, updated_at = NOW() WHERE id = $1`,
		userID, statusType)
	return err
}

// SetUserStatusTypeIfNotInvisible sets status_type only when the current value
// is not 'invisible'. Used by the hub on first WS connection.
func (r *Repository) SetUserStatusTypeIfNotInvisible(ctx context.Context, userID int64, statusType string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET status_type = $2, updated_at = NOW()
		 WHERE id = $1 AND status_type != 'invisible'`,
		userID, statusType)
	return err
}

// UpdateUserFields updates username, display_name, and avatar_url for the given user.
// Used by PATCH /api/users/me.
func (r *Repository) UpdateUserFields(ctx context.Context, userID int64, username, displayName, avatarURL string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = $2, display_name = $3, avatar_url = $4, updated_at = NOW()
		 WHERE id = $1`,
		userID, username, displayName, avatarURL)
	if err == nil {
		r.invalidateUserMessageInfo(userID)
	}
	return err
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

// GetUserDisplayName returns the user's display_name falling back to username.
func (r *Repository) GetUserDisplayName(ctx context.Context, userID int64) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(NULLIF(display_name, ''), username) FROM users WHERE id = $1",
		userID,
	).Scan(&name)
	return name, err
}
