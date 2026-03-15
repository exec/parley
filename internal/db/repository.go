package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	pq "github.com/lib/pq"
)

var (
	ErrNotFound         = errors.New("record not found")
	ErrAlreadyExists    = errors.New("record already exists")
	ErrInvalidOperation = errors.New("invalid operation")
)

// Repository handles database operations for all models
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository with the given database connection
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// NewRepositoryWithDSN creates a new Repository and establishes a connection using the DSN
func NewRepositoryWithDSN(ctx context.Context, dsn string) (*Repository, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return NewRepository(db), nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	return r.db.Close()
}

// DB returns the underlying database connection
func (r *Repository) DB() *sql.DB {
	return r.db
}

// ============ User Operations ============

// CreateUser creates a new user in the database
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

	if err != nil {
		return err
	}

	return nil
}

// GetUserByID retrieves a user by their ID
func (r *Repository) GetUserByID(ctx context.Context, id int64) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
		       email_verified, COALESCE(email_verification_token, ''),
		       COALESCE(phone_number, ''), phone_verified,
		       banned_at, COALESCE(ban_reason, ''), force_logout_at, is_system,
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

// GetUserByEmail retrieves a user by their email
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
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

// GetUserByUsername retrieves a user by their username
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
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

// UpdateUser updates an existing user
func (r *Repository) UpdateUser(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET username = $1, email = $2, password_hash = $3, avatar_url = $4, banner_url = $5,
		    email_verification_token = NULLIF($6, ''), updated_at = $7
		WHERE id = $8
	`

	user.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
		user.BannerURL,
		user.EmailVerificationToken,
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

// GetUserByVerificationToken retrieves a user by their email verification token.
func (r *Repository) GetUserByVerificationToken(ctx context.Context, token string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
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

// SetEmailVerified marks a user's email as verified and clears the verification token.
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

// CheckAndIncrementEmailResend atomically checks whether the user is under the daily
// resend limit (3 per day) and increments the counter if so. Returns ErrInvalidOperation
// when the limit is exceeded.
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

// CheckAndIncrementEmailChange atomically checks whether the user is under the daily
// email-change limit (3 per day) and increments the counter if so. Returns ErrInvalidOperation
// when the limit is exceeded.
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

// GetUserByPhone retrieves a user by their phone number.
func (r *Repository) GetUserByPhone(ctx context.Context, phone string) (*User, error) {
	query := `
		SELECT id, username, COALESCE(email, ''), password_hash, COALESCE(avatar_url, ''), COALESCE(banner_url, ''),
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
		&user.AvatarURL, &user.BannerURL,
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

// SetPhoneVerified marks a user's phone as verified and clears the verification code.
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

// SetPhoneVerificationCode stores a 6-digit OTP and its expiry for phone verification.
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

// CheckPhoneVerificationCode validates the OTP for a user. Returns ErrNotFound if the
// user has no pending code, ErrInvalidOperation if expired or wrong.
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

// CheckAndIncrementSmsResend atomically checks and increments the daily SMS send counter.
// Max 5 per day. Returns ErrInvalidOperation when the limit is exceeded.
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

// UpdateUserPhone sets a new phone number, clears phone_verified, resets SMS counters.
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

// UpdateUserEmail changes a user's email address, clears email_verified, sets a new
// verification token, and resets the resend counter (new verification = fresh slate).
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

// ============ Server Operations ============

// CreateServer creates a new server in the database
func (r *Repository) CreateServer(ctx context.Context, server *Server) error {
	query := `
		INSERT INTO servers (name, icon_url, owner_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	now := time.Now()
	server.CreatedAt = now
	server.UpdatedAt = now

	err := r.db.QueryRowContext(ctx, query,
		server.Name,
		server.IconURL,
		server.OwnerID,
		server.CreatedAt,
		server.UpdatedAt,
	).Scan(&server.ID)

	if err != nil {
		return err
	}

	return nil
}

// GetServerByID retrieves a server by its ID
func (r *Repository) GetServerByID(ctx context.Context, id int64) (*Server, error) {
	query := `
		SELECT id, name, icon_url, owner_id, vanity_url, created_at, updated_at
		FROM servers
		WHERE id = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&server.ID,
		&server.Name,
		&server.IconURL,
		&server.OwnerID,
		&server.VanityURL,
		&server.CreatedAt,
		&server.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &server, nil
}

// GetServersByUserID retrieves all servers that a user is a member of
func (r *Repository) GetServersByUserID(ctx context.Context, userID int64) ([]*Server, error) {
	query := `
		SELECT s.id, s.name, s.icon_url, s.owner_id, s.vanity_url, s.created_at, s.updated_at
		FROM servers s
		INNER JOIN server_members sm ON s.id = sm.server_id
		WHERE sm.user_id = $1
		ORDER BY s.name
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []*Server
	for rows.Next() {
		var server Server
		err := rows.Scan(
			&server.ID,
			&server.Name,
			&server.IconURL,
			&server.OwnerID,
			&server.VanityURL,
			&server.CreatedAt,
			&server.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		servers = append(servers, &server)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return servers, nil
}

// UpdateServer updates an existing server
func (r *Repository) UpdateServer(ctx context.Context, server *Server) error {
	query := `
		UPDATE servers
		SET name = $1, icon_url = $2, owner_id = $3, vanity_url = $4, updated_at = $5
		WHERE id = $6
	`

	server.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		server.Name,
		server.IconURL,
		server.OwnerID,
		server.VanityURL,
		server.UpdatedAt,
		server.ID,
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

// DeleteServer deletes a server by its ID
func (r *Repository) DeleteServer(ctx context.Context, id int64) error {
	query := `DELETE FROM servers WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
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

// ============ ServerMember Operations ============

// AddMember adds a user to a server
func (r *Repository) AddMember(ctx context.Context, member *ServerMember) error {
	query := `
		INSERT INTO server_members (server_id, user_id, nickname, joined_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	member.JoinedAt = time.Now()

	err := r.db.QueryRowContext(ctx, query,
		member.ServerID,
		member.UserID,
		member.Nickname,
		member.JoinedAt,
	).Scan(&member.ID)

	if err != nil {
		return err
	}

	return nil
}

// RemoveMember removes a user from a server
func (r *Repository) RemoveMember(ctx context.Context, serverID, userID int64) error {
	query := `DELETE FROM server_members WHERE server_id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, serverID, userID)
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

// GetMember retrieves a specific server member
func (r *Repository) GetMember(ctx context.Context, serverID, userID int64) (*ServerMember, error) {
	query := `
		SELECT id, server_id, user_id, nickname, joined_at
		FROM server_members
		WHERE server_id = $1 AND user_id = $2
	`

	var member ServerMember
	err := r.db.QueryRowContext(ctx, query, serverID, userID).Scan(
		&member.ID,
		&member.ServerID,
		&member.UserID,
		&member.Nickname,
		&member.JoinedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &member, nil
}

// GetServerMembers retrieves all members of a server
func (r *Repository) GetServerMembers(ctx context.Context, serverID int64) ([]*ServerMember, error) {
	query := `
		SELECT sm.id, sm.server_id, sm.user_id, sm.nickname, sm.joined_at, u.username
		FROM server_members sm
		JOIN users u ON u.id = sm.user_id
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
		err := rows.Scan(
			&member.ID,
			&member.ServerID,
			&member.UserID,
			&member.Nickname,
			&member.JoinedAt,
			&member.Username,
		)
		if err != nil {
			return nil, err
		}
		members = append(members, &member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return members, nil
}

// ============ Channel Operations ============

// CreateChannel creates a new channel in the database
func (r *Repository) CreateChannel(ctx context.Context, channel *Channel) error {
	query := `
		INSERT INTO channels (server_id, name, channel_type, position, parent_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	channel.CreatedAt = time.Now()

	err := r.db.QueryRowContext(ctx, query,
		channel.ServerID,
		channel.Name,
		channel.ChannelType,
		channel.Position,
		channel.ParentID,
		channel.CreatedAt,
	).Scan(&channel.ID)

	if err != nil {
		return err
	}

	return nil
}

// GetChannelByID retrieves a channel by its ID
func (r *Repository) GetChannelByID(ctx context.Context, id int64) (*Channel, error) {
	query := `
		SELECT id, server_id, name, channel_type, position, parent_id, created_at, updated_at
		FROM channels
		WHERE id = $1
	`

	var channel Channel
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID,
		&channel.ServerID,
		&channel.Name,
		&channel.ChannelType,
		&channel.Position,
		&channel.ParentID,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &channel, nil
}

// GetChannelsByServerID retrieves all channels for a server
func (r *Repository) GetChannelsByServerID(ctx context.Context, serverID int64) ([]*Channel, error) {
	query := `
		SELECT id, server_id, name, channel_type, position, parent_id, created_at, updated_at
		FROM channels
		WHERE server_id = $1
		ORDER BY position, name
	`

	rows, err := r.db.QueryContext(ctx, query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		var channel Channel
		err := rows.Scan(
			&channel.ID,
			&channel.ServerID,
			&channel.Name,
			&channel.ChannelType,
			&channel.Position,
			&channel.ParentID,
			&channel.CreatedAt,
			&channel.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, &channel)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return channels, nil
}

// UpdateChannel updates an existing channel
func (r *Repository) UpdateChannel(ctx context.Context, channel *Channel) error {
	query := `
		UPDATE channels
		SET name = $1, channel_type = $2, position = $3, parent_id = $4, updated_at = NOW()
		WHERE id = $5
	`

	result, err := r.db.ExecContext(ctx, query,
		channel.Name,
		channel.ChannelType,
		channel.Position,
		channel.ParentID,
		channel.ID,
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

// DeleteChannel deletes a channel by its ID
func (r *Repository) DeleteChannel(ctx context.Context, id int64) error {
	query := `DELETE FROM channels WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
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

// ============ Message Operations ============

// CreateMessage creates a new message in the database.
// If a nonce is set, duplicate submissions (retries) return the existing message instead of inserting.
func (r *Repository) CreateMessage(ctx context.Context, channelID, authorID int64, content, nonce, attachmentURL, attachmentName, attachmentType string) (*Message, error) {
	now := time.Now()

	query := `
		INSERT INTO messages (channel_id, author_id, content, nonce, attachment_url, attachment_name, attachment_type, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9)
		ON CONFLICT (nonce) WHERE nonce IS NOT NULL AND nonce != ''
		DO UPDATE SET updated_at = messages.updated_at
		RETURNING id, created_at, updated_at, COALESCE(nonce, ''), attachment_url, attachment_name, attachment_type
	`

	var message Message
	message.ChannelID = channelID
	message.AuthorID = authorID
	message.Content = content
	message.CreatedAt = now
	message.UpdatedAt = now

	err := r.db.QueryRowContext(ctx, query,
		channelID,
		authorID,
		content,
		nonce,
		attachmentURL,
		attachmentName,
		attachmentType,
		now,
		now,
	).Scan(
		&message.ID,
		&message.CreatedAt,
		&message.UpdatedAt,
		&message.Nonce,
		&message.AttachmentURL,
		&message.AttachmentName,
		&message.AttachmentType,
	)
	if err != nil {
		return nil, err
	}

	return &message, nil
}

// GetMessageByID retrieves a message by its ID
func (r *Repository) GetMessageByID(ctx context.Context, id int64) (*Message, error) {
	query := `
		SELECT id, channel_id, author_id, content, COALESCE(nonce, ''), created_at, updated_at
		FROM messages
		WHERE id = $1
	`

	var message Message
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&message.ID,
		&message.ChannelID,
		&message.AuthorID,
		&message.Content,
		&message.Nonce,
		&message.CreatedAt,
		&message.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &message, nil
}

// GetChannelMessages retrieves messages for a channel with pagination
func (r *Repository) GetChannelMessages(ctx context.Context, channelID int64, limit, offset int) ([]*Message, error) {
	query := `
		SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce, ''), m.created_at, m.updated_at,
		       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
		       u.username, COALESCE(u.avatar_url, '')
		FROM messages m
		JOIN users u ON u.id = m.author_id
		WHERE m.channel_id = $1
		ORDER BY m.id ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, channelID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		var message Message
		err := rows.Scan(
			&message.ID,
			&message.ChannelID,
			&message.AuthorID,
			&message.Content,
			&message.Nonce,
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.AttachmentURL,
			&message.AttachmentName,
			&message.AttachmentType,
			&message.AuthorUsername,
			&message.AuthorAvatarURL,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, &message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

// DeleteMessage deletes a message by its ID
func (r *Repository) DeleteMessage(ctx context.Context, id int64) error {
	query := `DELETE FROM messages WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
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

// RunMigrations executes all database migrations
func (r *Repository) RunMigrations(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, MigrationSQL())
	return err
}

// ============ DM Channel Operations ============

// GetOrCreateDmChannel finds or creates a DM channel between two users
func (r *Repository) GetOrCreateDmChannel(ctx context.Context, userAID, userBID int64) (*DmChannel, error) {
	// Ensure user1_id < user2_id for the UNIQUE constraint
	user1ID, user2ID := userAID, userBID
	if user1ID > user2ID {
		user1ID, user2ID = user2ID, user1ID
	}

	// Try to insert, ignore if exists
	insertQuery := `
		INSERT INTO dm_channels (user1_id, user2_id, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, insertQuery, user1ID, user2ID)
	if err != nil {
		return nil, err
	}

	// Get the channel
	query := `
		SELECT id, user1_id, user2_id, created_at
		FROM dm_channels
		WHERE user1_id = $1 AND user2_id = $2
	`
	var channel DmChannel
	err = r.db.QueryRowContext(ctx, query, user1ID, user2ID).Scan(
		&channel.ID,
		&channel.User1ID,
		&channel.User2ID,
		&channel.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Populate the other user's info
	otherUserID := user2ID
	if userAID == user1ID {
		otherUserID = user2ID
	} else {
		otherUserID = user1ID
	}

	var otherUser User
	err = r.db.QueryRowContext(ctx, "SELECT id, username FROM users WHERE id = $1", otherUserID).Scan(&otherUser.ID, &otherUser.Username)
	if err == nil {
		channel.OtherUserID = otherUser.ID
		channel.OtherUsername = otherUser.Username
	}

	return &channel, nil
}

// GetUserDmChannels returns all DM channels for a user
func (r *Repository) GetUserDmChannels(ctx context.Context, userID int64) ([]DmChannel, error) {
	query := `
		SELECT dc.id, dc.user1_id, dc.user2_id, dc.created_at,
			   u.id as other_user_id, u.username as other_username
		FROM dm_channels dc
		JOIN users u ON u.id = CASE WHEN dc.user1_id = $1 THEN dc.user2_id ELSE dc.user1_id END
		WHERE dc.user1_id = $1 OR dc.user2_id = $1
		ORDER BY dc.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []DmChannel
	for rows.Next() {
		var channel DmChannel
		err := rows.Scan(
			&channel.ID,
			&channel.User1ID,
			&channel.User2ID,
			&channel.CreatedAt,
			&channel.OtherUserID,
			&channel.OtherUsername,
		)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return channels, nil
}

// GetDmChannelByID retrieves a DM channel by its ID
func (r *Repository) GetDmChannelByID(ctx context.Context, id int64) (*DmChannel, error) {
	query := `
		SELECT id, user1_id, user2_id, created_at
		FROM dm_channels
		WHERE id = $1
	`

	var channel DmChannel
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID,
		&channel.User1ID,
		&channel.User2ID,
		&channel.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// ============ DM Message Operations ============

// CreateDmMessage creates a new DM message
func (r *Repository) CreateDmMessage(ctx context.Context, dmChannelID, authorID int64, content, attachmentURL, attachmentName, attachmentType string) (*DmMessage, error) {
	query := `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, attachment_url, attachment_name, attachment_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, dm_channel_id, author_id, content, attachment_url, attachment_name, attachment_type, created_at, updated_at
	`

	var msg DmMessage
	err := r.db.QueryRowContext(ctx, query, dmChannelID, authorID, content, attachmentURL, attachmentName, attachmentType).Scan(
		&msg.ID,
		&msg.DmChannelID,
		&msg.AuthorID,
		&msg.Content,
		&msg.AttachmentURL,
		&msg.AttachmentName,
		&msg.AttachmentType,
		&msg.CreatedAt,
		&msg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Get author username
	var username string
	r.db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = $1", authorID).Scan(&username)
	msg.AuthorUsername = username

	return &msg, nil
}

// GetDmMessages retrieves messages for a DM channel
func (r *Repository) GetDmMessages(ctx context.Context, dmChannelID int64, limit, offset int) ([]DmMessage, error) {
	query := `
		SELECT m.id, m.dm_channel_id, m.author_id, m.content,
		       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
		       m.created_at, m.updated_at, u.username
		FROM dm_messages m
		JOIN users u ON u.id = m.author_id
		WHERE m.dm_channel_id = $1
		ORDER BY m.created_at ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, dmChannelID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []DmMessage
	for rows.Next() {
		var msg DmMessage
		err := rows.Scan(
			&msg.ID,
			&msg.DmChannelID,
			&msg.AuthorID,
			&msg.Content,
			&msg.AttachmentURL,
			&msg.AttachmentName,
			&msg.AttachmentType,
			&msg.CreatedAt,
			&msg.UpdatedAt,
			&msg.AuthorUsername,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

// ============ User Search & Profile Operations ============

// GetPublicUser returns public profile info for a user
func (r *Repository) GetPublicUser(ctx context.Context, userID int64) (*PublicUser, error) {
	query := `
		SELECT id, username, COALESCE(avatar_url, ''), created_at
		FROM users
		WHERE id = $1
	`

	var user PublicUser
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.AvatarURL,
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

// SearchUsers searches users by username prefix
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

// ============ Invite Operations ============

// CreateInvite creates a new invite for a server
func (r *Repository) CreateInvite(ctx context.Context, invite *Invite) error {
	query := `
		INSERT INTO invites (server_id, code, created_by, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	invite.CreatedAt = time.Now()

	err := r.db.QueryRowContext(ctx, query,
		invite.ServerID,
		invite.Code,
		invite.CreatedBy,
		invite.CreatedAt,
	).Scan(&invite.ID)

	if err != nil {
		return err
	}

	return nil
}

// GetInviteByCode retrieves an invite by its code
func (r *Repository) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	query := `
		SELECT id, server_id, code, created_by, created_at
		FROM invites
		WHERE code = $1
	`

	var invite Invite
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&invite.ID,
		&invite.ServerID,
		&invite.Code,
		&invite.CreatedBy,
		&invite.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &invite, nil
}

// GetServerByInviteCode retrieves a server by invite code
func (r *Repository) GetServerByInviteCode(ctx context.Context, code string) (*Server, error) {
	query := `
		SELECT s.id, s.name, s.icon_url, s.owner_id, s.vanity_url, s.created_at, s.updated_at
		FROM servers s
		INNER JOIN invites i ON s.id = i.server_id
		WHERE i.code = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&server.ID,
		&server.Name,
		&server.IconURL,
		&server.OwnerID,
		&server.VanityURL,
		&server.CreatedAt,
		&server.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &server, nil
}

// InviteCodeExists returns true if the given code is already used as an invite code or a server vanity URL
// (excluding the given serverID when checking vanity URLs, so a server can keep its own slug).
func (r *Repository) InviteCodeExists(ctx context.Context, code string, excludeServerID ...int64) (bool, error) {
	exclude := int64(0)
	if len(excludeServerID) > 0 {
		exclude = excludeServerID[0]
	}
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM invites WHERE code = $1
			UNION ALL
			SELECT 1 FROM servers WHERE vanity_url = $1 AND id != $2
		)`, code, exclude).Scan(&exists)
	return exists, err
}

// GetServerByVanityURL retrieves a server by its vanity URL slug
func (r *Repository) GetServerByVanityURL(ctx context.Context, vanityURL string) (*Server, error) {
	query := `
		SELECT id, name, icon_url, owner_id, vanity_url, created_at, updated_at
		FROM servers
		WHERE vanity_url = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, vanityURL).Scan(
		&server.ID,
		&server.Name,
		&server.IconURL,
		&server.OwnerID,
		&server.VanityURL,
		&server.CreatedAt,
		&server.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &server, nil
}

// SetVanityURL sets or clears the vanity URL for a server
func (r *Repository) SetVanityURL(ctx context.Context, serverID int64, vanityURL sql.NullString) error {
	query := `UPDATE servers SET vanity_url = $1, updated_at = NOW() WHERE id = $2`
	result, err := r.db.ExecContext(ctx, query, vanityURL, serverID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteInvite deletes an invite by its code
func (r *Repository) DeleteInvite(ctx context.Context, code string) error {
	query := `DELETE FROM invites WHERE code = $1`

	result, err := r.db.ExecContext(ctx, query, code)
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

// ============ Reaction Operations ============

// ToggleReaction adds or removes a user's reaction to a message.
// Returns true if the reaction was added, false if it was removed.
func (r *Repository) ToggleReaction(ctx context.Context, messageID, userID int64, emoji string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3",
		messageID, userID, emoji)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		return false, nil // removed
	}
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO message_reactions(message_id, user_id, emoji) VALUES($1, $2, $3)",
		messageID, userID, emoji)
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetReactionsForMessages fetches reaction groups for a set of message IDs.
// Returns a map of message ID → slice of ReactionGroup (one per emoji).
func (r *Repository) GetReactionsForMessages(ctx context.Context, messageIDs []int64) (map[int64][]ReactionGroup, error) {
	if len(messageIDs) == 0 {
		return map[int64][]ReactionGroup{}, nil
	}

	placeholders := make([]string, len(messageIDs))
	args := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT message_id, emoji, COUNT(*) as count,
		       ARRAY_AGG(user_id::text ORDER BY user_id) as user_ids
		FROM message_reactions
		WHERE message_id IN (%s)
		GROUP BY message_id, emoji
		ORDER BY message_id, MIN(created_at) ASC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64][]ReactionGroup)
	for rows.Next() {
		var messageID int64
		var rg ReactionGroup
		var userIDs pq.StringArray
		if err := rows.Scan(&messageID, &rg.Emoji, &rg.Count, &userIDs); err != nil {
			return nil, err
		}
		rg.UserIDs = []string(userIDs)
		result[messageID] = append(result[messageID], rg)
	}
	return result, rows.Err()
}

// GetServerRoles returns all roles for a server
func (r *Repository) GetServerRoles(ctx context.Context, serverID int64) ([]ServerRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, server_id, name, color, permissions, created_at
         FROM server_roles WHERE server_id = $1 ORDER BY created_at ASC`,
		serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []ServerRole
	for rows.Next() {
		var role ServerRole
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// CreateServerRole creates a new role in a server
func (r *Repository) CreateServerRole(ctx context.Context, serverID int64, name, color string, permissions int64) (*ServerRole, error) {
	var role ServerRole
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO server_roles (server_id, name, color, permissions)
         VALUES ($1, $2, $3, $4)
         RETURNING id, server_id, name, color, permissions, created_at`,
		serverID, name, color, permissions,
	).Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// DeleteServerRole deletes a role from a server
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

// GetMemberRoles returns all roles assigned to a member in a server
func (r *Repository) GetMemberRoles(ctx context.Context, serverID, userID int64) ([]ServerRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sr.id, sr.server_id, sr.name, sr.color, sr.permissions, sr.created_at
         FROM server_roles sr
         JOIN server_member_roles smr ON smr.role_id = sr.id
         WHERE smr.server_id = $1 AND smr.user_id = $2
         ORDER BY sr.created_at ASC`,
		serverID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []ServerRole
	for rows.Next() {
		var role ServerRole
		if err := rows.Scan(&role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// AssignRoleToMember assigns a role to a server member
func (r *Repository) AssignRoleToMember(ctx context.Context, serverID, userID, roleID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_member_roles (server_id, user_id, role_id)
         VALUES ($1, $2, $3)
         ON CONFLICT DO NOTHING`,
		serverID, userID, roleID)
	return err
}

// RemoveRoleFromMember removes a role from a server member
func (r *Repository) RemoveRoleFromMember(ctx context.Context, serverID, userID, roleID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM server_member_roles WHERE server_id = $1 AND user_id = $2 AND role_id = $3`,
		serverID, userID, roleID)
	return err
}

// GetServerMembersWithRoles returns all members of a server with their roles
func (r *Repository) GetServerMembersWithRoles(ctx context.Context, serverID int64) ([]*ServerMember, error) {
	// Get all members
	members, err := r.GetServerMembers(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return members, nil
	}

	// Get all member roles in one query
	rows, err := r.db.QueryContext(ctx,
		`SELECT smr.user_id, sr.id, sr.server_id, sr.name, sr.color, sr.permissions, sr.created_at
         FROM server_member_roles smr
         JOIN server_roles sr ON sr.id = smr.role_id
         WHERE smr.server_id = $1
         ORDER BY sr.created_at ASC`,
		serverID)
	if err != nil {
		return members, nil // non-fatal: return members without roles
	}
	defer rows.Close()

	rolesByUser := make(map[int64][]ServerRole)
	for rows.Next() {
		var userID int64
		var role ServerRole
		if err := rows.Scan(&userID, &role.ID, &role.ServerID, &role.Name, &role.Color, &role.Permissions, &role.CreatedAt); err != nil {
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

// ============ Admin Operations ============

// BanUser sets banned_at and ban_reason for a user.
func (r *Repository) BanUser(ctx context.Context, userID int64, reason string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET banned_at = NOW(), ban_reason = $2, updated_at = NOW() WHERE id = $1`, userID, reason)
	return err
}

// UnbanUser clears banned_at and ban_reason.
func (r *Repository) UnbanUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET banned_at = NULL, ban_reason = NULL, updated_at = NOW() WHERE id = $1`, userID)
	return err
}

// ForceLogoutUser sets force_logout_at to now, invalidating all existing tokens.
func (r *Repository) ForceLogoutUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET force_logout_at = NOW(), updated_at = NOW() WHERE id = $1`, userID)
	return err
}

// GetSystemUser returns the system bot account.
func (r *Repository) GetSystemUser(ctx context.Context) (*User, error) {
	return r.GetUserByUsername(ctx, "Parley")
}

// AdminCreateUser creates an admin user (inactive by default).
func (r *Repository) AdminCreateUser(ctx context.Context, username, passwordHash string) (*AdminUser, error) {
	u := &AdminUser{}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO admin_users (username, password_hash, active, created_at)
		VALUES ($1, $2, FALSE, NOW())
		RETURNING id, username, password_hash, active, created_at
	`, username, passwordHash).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Active, &u.CreatedAt)
	return u, err
}

// AdminGetUser fetches an admin user by username.
func (r *Repository) AdminGetUser(ctx context.Context, username string) (*AdminUser, error) {
	u := &AdminUser{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, active, created_at, last_login_at
		FROM admin_users WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Active, &u.CreatedAt, &u.LastLoginAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return u, err
}

// AdminListUsers lists all admin users.
func (r *Repository) AdminListUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, password_hash, active, created_at, last_login_at FROM admin_users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []AdminUser
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Active, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// AdminSetActive sets the active flag for an admin user.
func (r *Repository) AdminSetActive(ctx context.Context, username string, active bool) error {
	result, err := r.db.ExecContext(ctx, `UPDATE admin_users SET active = $2 WHERE username = $1`, username, active)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminUpdateLastLogin updates the last_login_at for an admin user.
func (r *Repository) AdminUpdateLastLogin(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = NOW() WHERE id = $1`, id)
	return err
}

// GetReports returns reports filtered by status (empty = all), paginated.
func (r *Repository) GetReports(ctx context.Context, status string, limit, offset int) ([]Report, error) {
	query := `
		SELECT rp.id, rp.reporter_id, rp.reported_user_id, rp.reported_message_id,
		       rp.category_id, rc.name, rp.description, rp.status,
		       rp.resolved_by, COALESCE(rp.resolution_note, ''),
		       COALESCE(u_reporter.username, ''), COALESCE(u_reported.username, ''),
		       rp.created_at, rp.updated_at
		FROM reports rp
		JOIN report_categories rc ON rc.id = rp.category_id
		LEFT JOIN users u_reporter ON u_reporter.id = rp.reporter_id
		LEFT JOIN users u_reported ON u_reported.id = rp.reported_user_id
	`
	args := []interface{}{}
	if status != "" {
		query += ` WHERE rp.status = $1`
		args = append(args, status)
		query += fmt.Sprintf(` ORDER BY rp.created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	} else {
		query += ` ORDER BY rp.created_at DESC LIMIT $1 OFFSET $2`
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []Report
	for rows.Next() {
		var rp Report
		if err := rows.Scan(&rp.ID, &rp.ReporterID, &rp.ReportedUserID, &rp.ReportedMessageID,
			&rp.CategoryID, &rp.CategoryName, &rp.Description, &rp.Status,
			&rp.ResolvedBy, &rp.ResolutionNote,
			&rp.ReporterUsername, &rp.ReportedUsername,
			&rp.CreatedAt, &rp.UpdatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, rp)
	}
	return reports, rows.Err()
}

// GetReport returns a single report by ID.
func (r *Repository) GetReport(ctx context.Context, id int64) (*Report, error) {
	var rp Report
	err := r.db.QueryRowContext(ctx, `
		SELECT rp.id, rp.reporter_id, rp.reported_user_id, rp.reported_message_id,
		       rp.category_id, rc.name, rp.description, rp.status,
		       rp.resolved_by, COALESCE(rp.resolution_note, ''),
		       COALESCE(u_reporter.username, ''), COALESCE(u_reported.username, ''),
		       rp.created_at, rp.updated_at
		FROM reports rp
		JOIN report_categories rc ON rc.id = rp.category_id
		LEFT JOIN users u_reporter ON u_reporter.id = rp.reporter_id
		LEFT JOIN users u_reported ON u_reported.id = rp.reported_user_id
		WHERE rp.id = $1
	`, id).Scan(&rp.ID, &rp.ReporterID, &rp.ReportedUserID, &rp.ReportedMessageID,
		&rp.CategoryID, &rp.CategoryName, &rp.Description, &rp.Status,
		&rp.ResolvedBy, &rp.ResolutionNote,
		&rp.ReporterUsername, &rp.ReportedUsername,
		&rp.CreatedAt, &rp.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &rp, err
}

// ResolveReport updates a report's status and resolution note.
func (r *Repository) ResolveReport(ctx context.Context, reportID, adminID int64, status, note string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE reports SET status = $2, resolved_by = $3, resolution_note = $4, updated_at = NOW()
		WHERE id = $1
	`, reportID, status, adminID, note)
	return err
}

// GetMessageContext returns N messages before and after a given message ID in the same channel.
func (r *Repository) GetMessageContext(ctx context.Context, messageID int64, before, after int) ([]Message, error) {
	var channelID int64
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, `SELECT channel_id, created_at FROM messages WHERE id = $1`, messageID).Scan(&channelID, &createdAt)
	if err != nil {
		return nil, err
	}

	query := `
		(SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce,''),
		        COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
		        m.created_at, m.updated_at, u.username
		 FROM messages m JOIN users u ON u.id = m.author_id
		 WHERE m.channel_id = $1 AND m.created_at < $2
		 ORDER BY m.created_at DESC LIMIT $3)
		UNION ALL
		(SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce,''),
		        COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
		        m.created_at, m.updated_at, u.username
		 FROM messages m JOIN users u ON u.id = m.author_id
		 WHERE m.channel_id = $1 AND m.created_at >= $2
		 ORDER BY m.created_at ASC LIMIT $4)
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, channelID, createdAt, before+1, after+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce,
			&m.AttachmentURL, &m.AttachmentName, &m.AttachmentType,
			&m.CreatedAt, &m.UpdatedAt, &m.AuthorUsername); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// SearchMessages searches messages by content and/or author username, paginated.
func (r *Repository) SearchMessages(ctx context.Context, query string, userID int64, limit, offset int) ([]Message, error) {
	args := []interface{}{}
	where := []string{}
	if query != "" {
		args = append(args, "%"+query+"%")
		where = append(where, fmt.Sprintf("m.content ILIKE $%d", len(args)))
	}
	if userID > 0 {
		args = append(args, userID)
		where = append(where, fmt.Sprintf("m.author_id = $%d", len(args)))
	}
	sqlStr := `SELECT m.id, m.channel_id, m.author_id, m.content, COALESCE(m.nonce,''),
	                  COALESCE(m.attachment_url,''), COALESCE(m.attachment_name,''), COALESCE(m.attachment_type,''),
	                  m.created_at, m.updated_at, u.username
	           FROM messages m JOIN users u ON u.id = m.author_id`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, limit, offset)
	sqlStr += fmt.Sprintf(` ORDER BY m.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Nonce,
			&m.AttachmentURL, &m.AttachmentName, &m.AttachmentType,
			&m.CreatedAt, &m.UpdatedAt, &m.AuthorUsername); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// AdminSearchUsers searches users by username for the admin panel.
func (r *Repository) AdminSearchUsers(ctx context.Context, query string, limit, offset int) ([]User, error) {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, username, COALESCE(email,''), password_hash, COALESCE(avatar_url,''), COALESCE(banner_url,''),
			       email_verified, COALESCE(email_verification_token,''),
			       COALESCE(phone_number,''), phone_verified,
			       banned_at, COALESCE(ban_reason,''), force_logout_at, is_system,
			       created_at, updated_at
			FROM users WHERE username ILIKE $1 AND is_system = FALSE
			ORDER BY created_at DESC LIMIT $2 OFFSET $3
		`, "%"+query+"%", limit, offset)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, username, COALESCE(email,''), password_hash, COALESCE(avatar_url,''), COALESCE(banner_url,''),
			       email_verified, COALESCE(email_verification_token,''),
			       COALESCE(phone_number,''), phone_verified,
			       banned_at, COALESCE(ban_reason,''), force_logout_at, is_system,
			       created_at, updated_at
			FROM users WHERE is_system = FALSE
			ORDER BY created_at DESC LIMIT $1 OFFSET $2
		`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var bannedAt, forceLogoutAt sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.AvatarURL, &u.BannerURL,
			&u.EmailVerified, &u.EmailVerificationToken, &u.PhoneNumber, &u.PhoneVerified,
			&bannedAt, &u.BanReason, &forceLogoutAt, &u.IsSystem,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		if bannedAt.Valid {
			u.BannedAt = &bannedAt.Time
		}
		if forceLogoutAt.Valid {
			u.ForceLogoutAt = &forceLogoutAt.Time
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetAdminStats returns dashboard statistics.
func (r *Repository) GetAdminStats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{}
	queries := map[string]string{
		"total_users":     `SELECT COUNT(*) FROM users WHERE is_system = FALSE`,
		"total_messages":  `SELECT COUNT(*) FROM messages`,
		"total_servers":   `SELECT COUNT(*) FROM servers`,
		"open_reports":    `SELECT COUNT(*) FROM reports WHERE status = 'open'`,
		"banned_users":    `SELECT COUNT(*) FROM users WHERE banned_at IS NOT NULL`,
		"new_users_today": `SELECT COUNT(*) FROM users WHERE created_at >= CURRENT_DATE AND is_system = FALSE`,
	}
	for key, q := range queries {
		var n int64
		if err := r.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return nil, err
		}
		stats[key] = n
	}
	return stats, nil
}

// GetReportCategories returns all report categories.
func (r *Repository) GetReportCategories(ctx context.Context) ([]ReportCategory, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, created_at FROM report_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []ReportCategory
	for rows.Next() {
		var c ReportCategory
		rows.Scan(&c.ID, &c.Name, &c.CreatedAt)
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// CreateReportCategory adds a new report category.
func (r *Repository) CreateReportCategory(ctx context.Context, name string) (*ReportCategory, error) {
	c := &ReportCategory{}
	err := r.db.QueryRowContext(ctx, `INSERT INTO report_categories (name) VALUES ($1) RETURNING id, name, created_at`, name).
		Scan(&c.ID, &c.Name, &c.CreatedAt)
	return c, err
}

// DeleteReportCategory removes a category.
func (r *Repository) DeleteReportCategory(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM report_categories WHERE id = $1`, id)
	return err
}

// AdminGetServers returns servers for the admin panel, optionally filtered by name.
func (r *Repository) AdminGetServers(ctx context.Context, query string, limit, offset int) ([]Server, error) {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = r.db.QueryContext(ctx, `SELECT id, name, icon_url, owner_id, vanity_url, created_at, updated_at FROM servers WHERE name ILIKE $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, "%"+query+"%", limit, offset)
	} else {
		rows, err = r.db.QueryContext(ctx, `SELECT id, name, icon_url, owner_id, vanity_url, created_at, updated_at FROM servers ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		rows.Scan(&s.ID, &s.Name, &s.IconURL, &s.OwnerID, &s.VanityURL, &s.CreatedAt, &s.UpdatedAt)
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

// GetServerMemberUserIDs returns user IDs of all members in a server.
func (r *Repository) GetServerMemberUserIDs(ctx context.Context, serverID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id FROM server_members WHERE server_id = $1`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SendSystemDM sends a DM from the system user to a recipient. Creates DM channel if needed.
func (r *Repository) SendSystemDM(ctx context.Context, systemUserID, recipientID int64, content string) error {
	var dmChannelID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM dm_channels
		WHERE (user1_id = $1 AND user2_id = $2) OR (user1_id = $2 AND user2_id = $1)
	`, systemUserID, recipientID).Scan(&dmChannelID)
	if err == sql.ErrNoRows {
		err = r.db.QueryRowContext(ctx, `
			INSERT INTO dm_channels (user1_id, user2_id, created_at)
			VALUES ($1, $2, NOW()) RETURNING id
		`, systemUserID, recipientID).Scan(&dmChannelID)
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, dmChannelID, systemUserID, content)
	return err
}

// AdminDeleteMessage hard-deletes a message.
func (r *Repository) AdminDeleteMessage(ctx context.Context, messageID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE id = $1`, messageID)
	return err
}

// AdminDeleteUser hard-deletes a user account and all their data.
func (r *Repository) AdminDeleteUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

// AddServerBan adds a server-level ban for a user.
func (r *Repository) AddServerBan(ctx context.Context, serverID, userID, bannedByID int64, reason string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_bans (server_id, user_id, banned_by, reason) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (server_id, user_id) DO UPDATE SET reason = EXCLUDED.reason, banned_by = EXCLUDED.banned_by`,
		serverID, userID, bannedByID, reason,
	)
	return err
}

// IsServerBanned returns true if the user is banned from the server.
func (r *Repository) IsServerBanned(ctx context.Context, serverID, userID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_bans WHERE server_id = $1 AND user_id = $2`,
		serverID, userID,
	).Scan(&count)
	return count > 0, err
}