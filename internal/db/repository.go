package db

import (
	"context"
	"database/sql"
	"errors"
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
		SELECT id, name, icon_url, owner_id, created_at, updated_at
		FROM servers
		WHERE id = $1
	`

	var server Server
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&server.ID,
		&server.Name,
		&server.IconURL,
		&server.OwnerID,
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
		SELECT s.id, s.name, s.icon_url, s.owner_id, s.created_at, s.updated_at
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
		SET name = $1, icon_url = $2, owner_id = $3, updated_at = $4
		WHERE id = $5
	`

	server.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		server.Name,
		server.IconURL,
		server.OwnerID,
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
		SELECT id, server_id, user_id, nickname, joined_at
		FROM server_members
		WHERE server_id = $1
		ORDER BY joined_at
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
		SELECT id, server_id, name, channel_type, position, parent_id, created_at
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
		SELECT id, server_id, name, channel_type, position, parent_id, created_at
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
		SET name = $1, channel_type = $2, position = $3, parent_id = $4
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
		SELECT id, channel_id, author_id, content, created_at, updated_at
		FROM messages
		WHERE channel_id = $1
		ORDER BY created_at DESC
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