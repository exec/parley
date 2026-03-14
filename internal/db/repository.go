package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "github.com/lib/pq"
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
		INSERT INTO users (username, email, password_hash, avatar_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
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
		SELECT id, username, email, password_hash, avatar_url, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user User
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by their email
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, username, email, password_hash, avatar_url, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by their username
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, email, password_hash, avatar_url, created_at, updated_at
		FROM users
		WHERE username = $1
	`

	var user User
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.AvatarURL,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// UpdateUser updates an existing user
func (r *Repository) UpdateUser(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET username = $1, email = $2, password_hash = $3, avatar_url = $4, updated_at = $5
		WHERE id = $6
	`

	user.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.AvatarURL,
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

// CreateMessage creates a new message in the database
func (r *Repository) CreateMessage(ctx context.Context, message *Message) error {
	query := `
		INSERT INTO messages (channel_id, author_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	now := time.Now()
	message.CreatedAt = now
	message.UpdatedAt = now

	err := r.db.QueryRowContext(ctx, query,
		message.ChannelID,
		message.AuthorID,
		message.Content,
		message.CreatedAt,
		message.UpdatedAt,
	).Scan(&message.ID)

	if err != nil {
		return err
	}

	return nil
}

// GetMessageByID retrieves a message by its ID
func (r *Repository) GetMessageByID(ctx context.Context, id int64) (*Message, error) {
	query := `
		SELECT id, channel_id, author_id, content, created_at, updated_at
		FROM messages
		WHERE id = $1
	`

	var message Message
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&message.ID,
		&message.ChannelID,
		&message.AuthorID,
		&message.Content,
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
		SELECT m.id, m.channel_id, m.author_id, m.content, m.created_at, m.updated_at, u.username
		FROM messages m
		JOIN users u ON u.id = m.author_id
		WHERE m.channel_id = $1
		ORDER BY m.created_at ASC
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
			&message.CreatedAt,
			&message.UpdatedAt,
			&message.AuthorUsername,
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
func (r *Repository) CreateDmMessage(ctx context.Context, dmChannelID, authorID int64, content string) (*DmMessage, error) {
	query := `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, dm_channel_id, author_id, content, created_at, updated_at
	`

	var msg DmMessage
	err := r.db.QueryRowContext(ctx, query, dmChannelID, authorID, content).Scan(
		&msg.ID,
		&msg.DmChannelID,
		&msg.AuthorID,
		&msg.Content,
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
		SELECT m.id, m.dm_channel_id, m.author_id, m.content, m.created_at, m.updated_at, u.username
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