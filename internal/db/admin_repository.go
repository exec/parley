package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ============ Admin Operations ============

func (r *Repository) BanUser(ctx context.Context, userID int64, reason string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET banned_at = NOW(), ban_reason = $2, updated_at = NOW() WHERE id = $1`, userID, reason)
	return err
}

func (r *Repository) UnbanUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET banned_at = NULL, ban_reason = NULL, updated_at = NOW() WHERE id = $1`, userID)
	return err
}

func (r *Repository) ForceLogoutUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET force_logout_at = NOW(), updated_at = NOW() WHERE id = $1`, userID)
	return err
}

func (r *Repository) GetSystemUser(ctx context.Context) (*User, error) {
	return r.GetUserByUsername(ctx, "Parley")
}

func (r *Repository) AdminCreateUser(ctx context.Context, username, passwordHash string) (*AdminUser, error) {
	u := &AdminUser{}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO admin_users (username, password_hash, active, created_at)
		VALUES ($1, $2, FALSE, NOW())
		RETURNING id, username, password_hash, active, created_at
	`, username, passwordHash).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Active, &u.CreatedAt)
	return u, err
}

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

func (r *Repository) AdminResetPassword(ctx context.Context, username, passwordHash string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE admin_users SET password_hash = $2 WHERE username = $1`, username, passwordHash)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) AdminUpdateLastLogin(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = NOW() WHERE id = $1`, id)
	return err
}

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

func (r *Repository) ResolveReport(ctx context.Context, reportID, adminID int64, status, note string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE reports SET status = $2, resolved_by = $3, resolution_note = $4, updated_at = NOW()
		WHERE id = $1
	`, reportID, status, adminID, note)
	return err
}

func (r *Repository) AdminSearchUsers(ctx context.Context, query string, limit, offset int) ([]User, error) {
	var rows *sql.Rows
	var err error
	if query != "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, username, COALESCE(email,''), password_hash, COALESCE(avatar_url,''), COALESCE(banner_url,''),
			       email_verified, COALESCE(email_verification_token,''),
			       COALESCE(phone_number,''), phone_verified,
			       banned_at, COALESCE(ban_reason,''), force_logout_at, is_system,
			       COALESCE(registration_ip,''), COALESCE(last_seen_ip,''),
			       created_at, updated_at
			FROM users WHERE (username ILIKE $1 OR CAST(id AS TEXT) = $2) AND is_system = FALSE
			ORDER BY created_at DESC LIMIT $3 OFFSET $4
		`, "%"+query+"%", query, limit, offset)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, username, COALESCE(email,''), password_hash, COALESCE(avatar_url,''), COALESCE(banner_url,''),
			       email_verified, COALESCE(email_verification_token,''),
			       COALESCE(phone_number,''), phone_verified,
			       banned_at, COALESCE(ban_reason,''), force_logout_at, is_system,
			       COALESCE(registration_ip,''), COALESCE(last_seen_ip,''),
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
			&u.RegistrationIP, &u.LastSeenIP,
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

// AdminBotRow is a lightweight view of a bot user for the admin panel.
type AdminBotRow struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	OwnerID       *int64     `json:"owner_id,omitempty"`
	OwnerUsername string     `json:"owner_username,omitempty"`
	BannedAt      *time.Time `json:"banned_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (r *Repository) AdminGetBots(ctx context.Context, query string, limit, offset int) ([]AdminBotRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	base := `
		SELECT b.id, b.username, b.bot_owner_id, COALESCE(o.username, ''), b.banned_at, b.created_at
		FROM users b
		LEFT JOIN users o ON o.id = b.bot_owner_id
		WHERE b.is_bot = TRUE AND b.is_system = FALSE`
	if query != "" {
		rows, err = r.db.QueryContext(ctx, base+` AND (b.username ILIKE $1 OR CAST(b.id AS TEXT) = $2)
			ORDER BY b.created_at DESC LIMIT $3 OFFSET $4`,
			"%"+query+"%", query, limit, offset)
	} else {
		rows, err = r.db.QueryContext(ctx, base+` ORDER BY b.created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []AdminBotRow
	for rows.Next() {
		var b AdminBotRow
		var bannedAt sql.NullTime
		if err := rows.Scan(&b.ID, &b.Username, &b.OwnerID, &b.OwnerUsername, &bannedAt, &b.CreatedAt); err != nil {
			return nil, err
		}
		if bannedAt.Valid {
			b.BannedAt = &bannedAt.Time
		}
		bots = append(bots, b)
	}
	return bots, rows.Err()
}

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

func (r *Repository) CreateReportCategory(ctx context.Context, name string) (*ReportCategory, error) {
	c := &ReportCategory{}
	err := r.db.QueryRowContext(ctx, `INSERT INTO report_categories (name) VALUES ($1) RETURNING id, name, created_at`, name).
		Scan(&c.ID, &c.Name, &c.CreatedAt)
	return c, err
}

func (r *Repository) DeleteReportCategory(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM report_categories WHERE id = $1`, id)
	return err
}

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

func (r *Repository) AdminDeleteMessage(ctx context.Context, messageID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE id = $1`, messageID)
	return err
}

func (r *Repository) AdminDeleteUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

func (r *Repository) AdminSetBadges(ctx context.Context, userID int64, badges int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET badges = $1 WHERE id = $2`, badges, userID)
	return err
}

func (r *Repository) AddServerBan(ctx context.Context, serverID, userID, bannedByID int64, reason string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_bans (server_id, user_id, banned_by, reason) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (server_id, user_id) DO UPDATE SET reason = EXCLUDED.reason, banned_by = EXCLUDED.banned_by`,
		serverID, userID, bannedByID, reason,
	)
	return err
}

func (r *Repository) IsServerBanned(ctx context.Context, serverID, userID int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_bans WHERE server_id = $1 AND user_id = $2`,
		serverID, userID,
	).Scan(&count)
	return count > 0, err
}

type ServerBan struct {
	UserID    int64     `json:"user_id,string"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Reason    string    `json:"reason"`
	BannedAt  time.Time `json:"banned_at"`
}

func (r *Repository) ListServerBans(ctx context.Context, serverID int64) ([]ServerBan, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sb.user_id, u.username, COALESCE(u.avatar_url,''), sb.reason, sb.created_at
		 FROM server_bans sb
		 JOIN users u ON u.id = sb.user_id
		 WHERE sb.server_id = $1
		 ORDER BY sb.created_at DESC`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bans := make([]ServerBan, 0)
	for rows.Next() {
		var b ServerBan
		if err := rows.Scan(&b.UserID, &b.Username, &b.AvatarURL, &b.Reason, &b.BannedAt); err != nil {
			return nil, err
		}
		bans = append(bans, b)
	}
	return bans, rows.Err()
}

func (r *Repository) RemoveServerBan(ctx context.Context, serverID, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM server_bans WHERE server_id = $1 AND user_id = $2`,
		serverID, userID,
	)
	return err
}
