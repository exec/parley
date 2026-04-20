package botcommands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"parley/internal/db"
)

// ErrNotFound is returned when a row is not found.
var ErrNotFound = errors.New("not found")

// Repository owns all DB access for bot_commands and bot_interactions.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a Repository against the shared *sql.DB.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ============ bot_commands ============

// ListServerCommands returns every command registered in a server, ordered by
// bot_id then name, suitable for the autocomplete list presented to users. The
// bot's username / display_name / avatar_url are joined in so the dropdown can
// show which bot owns each command.
func (r *Repository) ListServerCommands(ctx context.Context, serverID int64) ([]BotCommand, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.bot_id, c.server_id, c.name, c.description, c.options,
		       c.created_at, c.updated_at,
		       u.username, u.display_name, u.avatar_url
		FROM bot_commands c
		JOIN users u ON u.id = c.bot_id
		WHERE c.server_id = $1
		ORDER BY c.bot_id, c.name`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list server commands: %w", err)
	}
	defer rows.Close()
	out := []BotCommand{}
	for rows.Next() {
		var c BotCommand
		var optsRaw []byte
		var avatar sql.NullString
		if err := rows.Scan(&c.ID, &c.BotID, &c.ServerID, &c.Name, &c.Description,
			&optsRaw, &c.CreatedAt, &c.UpdatedAt,
			&c.BotUsername, &c.BotDisplayName, &avatar); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		if avatar.Valid {
			c.BotAvatarURL = avatar.String
		}
		if err := unmarshalOptions(optsRaw, &c.Options); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commands: %w", err)
	}
	return out, nil
}

// ListBotCommandsInServer returns every command registered by a specific bot in a server.
func (r *Repository) ListBotCommandsInServer(ctx context.Context, botID, serverID int64) ([]BotCommand, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, bot_id, server_id, name, description, options, created_at, updated_at
		FROM bot_commands
		WHERE bot_id = $1 AND server_id = $2
		ORDER BY name`, botID, serverID)
	if err != nil {
		return nil, fmt.Errorf("list bot commands: %w", err)
	}
	defer rows.Close()
	return scanCommands(rows)
}

// GetCommandByID loads a single command row.
func (r *Repository) GetCommandByID(ctx context.Context, id int64) (*BotCommand, error) {
	var c BotCommand
	var optsRaw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id, bot_id, server_id, name, description, options, created_at, updated_at
		FROM bot_commands WHERE id = $1`, id).
		Scan(&c.ID, &c.BotID, &c.ServerID, &c.Name, &c.Description, &optsRaw, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get command: %w", err)
	}
	if err := unmarshalOptions(optsRaw, &c.Options); err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertCommand inserts a command or updates the existing row with the same
// (bot_id, server_id, name). Returns the stored row.
func (r *Repository) UpsertCommand(ctx context.Context, botID, serverID int64, name, description string, options []BotCommandOption) (*BotCommand, error) {
	optsJSON, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("marshal options: %w", err)
	}
	if len(optsJSON) == 0 || string(optsJSON) == "null" {
		optsJSON = []byte("[]")
	}

	var c BotCommand
	var optsRaw []byte
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO bot_commands (bot_id, server_id, name, description, options, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, NOW(), NOW())
		ON CONFLICT (bot_id, server_id, name) DO UPDATE SET
			description = EXCLUDED.description,
			options     = EXCLUDED.options,
			updated_at  = NOW()
		RETURNING id, bot_id, server_id, name, description, options, created_at, updated_at`,
		botID, serverID, name, description, string(optsJSON),
	).Scan(&c.ID, &c.BotID, &c.ServerID, &c.Name, &c.Description, &optsRaw, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert command: %w", err)
	}
	if err := unmarshalOptions(optsRaw, &c.Options); err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteCommandByName removes a command by (bot_id, server_id, name).
func (r *Repository) DeleteCommandByName(ctx context.Context, botID, serverID int64, name string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM bot_commands WHERE bot_id=$1 AND server_id=$2 AND name=$3`,
		botID, serverID, name)
	if err != nil {
		return fmt.Errorf("delete command: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkReplaceCommands replaces the entire set of commands a bot has registered
// in a server. Runs in a transaction so callers see either the old set or the
// new set, never a partial mix.
func (r *Repository) BulkReplaceCommands(ctx context.Context, botID, serverID int64, cmds []RegisterCommandRequest) ([]BotCommand, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bot_commands WHERE bot_id=$1 AND server_id=$2`,
		botID, serverID); err != nil {
		return nil, fmt.Errorf("bulk delete: %w", err)
	}

	out := make([]BotCommand, 0, len(cmds))
	for _, cmd := range cmds {
		optsJSON, err := json.Marshal(cmd.Options)
		if err != nil {
			return nil, fmt.Errorf("marshal options for %q: %w", cmd.Name, err)
		}
		if len(optsJSON) == 0 || string(optsJSON) == "null" {
			optsJSON = []byte("[]")
		}
		var c BotCommand
		var optsRaw []byte
		err = tx.QueryRowContext(ctx, `
			INSERT INTO bot_commands (bot_id, server_id, name, description, options, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, NOW(), NOW())
			RETURNING id, bot_id, server_id, name, description, options, created_at, updated_at`,
			botID, serverID, cmd.Name, cmd.Description, string(optsJSON),
		).Scan(&c.ID, &c.BotID, &c.ServerID, &c.Name, &c.Description, &optsRaw, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			if isPgUniqueViolation(err) {
				return nil, fmt.Errorf("duplicate command name %q", cmd.Name)
			}
			return nil, fmt.Errorf("bulk insert %q: %w", cmd.Name, err)
		}
		if err := unmarshalOptions(optsRaw, &c.Options); err != nil {
			return nil, err
		}
		out = append(out, c)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit bulk replace: %w", err)
	}
	return out, nil
}

// ============ bot_interactions ============

// CreateInteraction inserts a new pending interaction row.
func (r *Repository) CreateInteraction(ctx context.Context, i *BotInteraction) error {
	optsJSON, err := json.Marshal(i.Options)
	if err != nil {
		return fmt.Errorf("marshal interaction options: %w", err)
	}
	if len(optsJSON) == 0 || string(optsJSON) == "null" {
		optsJSON = []byte("{}")
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO bot_interactions
			(token, bot_id, command_id, invoker_user_id, channel_id, server_id, options, state, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, NOW(), NOW())
		RETURNING created_at, updated_at`,
		i.Token, i.BotID, i.CommandID, i.InvokerUserID, i.ChannelID, i.ServerID,
		string(optsJSON), i.State, i.ExpiresAt,
	).Scan(&i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create interaction: %w", err)
	}
	return nil
}

// GetInteractionByToken fetches an interaction by its token.
func (r *Repository) GetInteractionByToken(ctx context.Context, token string) (*BotInteraction, error) {
	var i BotInteraction
	var optsRaw []byte
	var respMsgID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT token, bot_id, command_id, invoker_user_id, channel_id, server_id,
		       options, state, response_message_id, expires_at, created_at, updated_at
		FROM bot_interactions WHERE token = $1`, token,
	).Scan(&i.Token, &i.BotID, &i.CommandID, &i.InvokerUserID, &i.ChannelID, &i.ServerID,
		&optsRaw, &i.State, &respMsgID, &i.ExpiresAt, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get interaction: %w", err)
	}
	if respMsgID.Valid {
		v := respMsgID.Int64
		i.ResponseMessageID = &v
	}
	i.Options = map[string]interface{}{}
	if len(optsRaw) > 0 {
		if err := json.Unmarshal(optsRaw, &i.Options); err != nil {
			return nil, fmt.Errorf("unmarshal interaction options: %w", err)
		}
	}
	return &i, nil
}

// MarkInteractionResponded transitions pending -> responded with the given
// response message ID. Returns ErrNotFound if no row matches (already responded,
// expired, or missing token).
func (r *Repository) MarkInteractionResponded(ctx context.Context, token string, responseMsgID int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE bot_interactions
		SET state='responded', response_message_id=$2, updated_at=NOW()
		WHERE token=$1 AND state='pending'`, token, responseMsgID)
	if err != nil {
		return fmt.Errorf("mark responded: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ExpirePastInteractions marks every still-pending interaction whose expires_at
// has passed as 'expired'. Called by a background sweeper (wired in main.go).
// Safe to run concurrently with CreateInteraction since it only touches rows in
// the terminal 'pending' state with expires_at < NOW().
func (r *Repository) ExpirePastInteractions(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE bot_interactions
		SET state='expired', updated_at=NOW()
		WHERE state='pending' AND expires_at < $1`, now)
	if err != nil {
		return 0, fmt.Errorf("expire interactions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ============ helpers ============

// IsBotInServer returns true if botID is recorded in server_bots for serverID.
// Kept here (rather than importing the bots package) so the botcommands package
// has no reverse dependency on bots.
func (r *Repository) IsBotInServer(ctx context.Context, serverID, botID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_bots WHERE server_id=$1 AND bot_user_id=$2`,
		serverID, botID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("is bot in server: %w", err)
	}
	return n > 0, nil
}

// GetInvoker fetches the identifying fields for the user who invoked a command,
// used to populate the INTERACTION_CREATE payload.
func (r *Repository) GetInvoker(ctx context.Context, userID int64) (InteractionInvoker, error) {
	var inv InteractionInvoker
	err := r.db.QueryRowContext(ctx, `
		SELECT id, username, COALESCE(display_name,''), COALESCE(avatar_url,'')
		FROM users WHERE id = $1`, userID,
	).Scan(&inv.ID, &inv.Username, &inv.DisplayName, &inv.AvatarURL)
	if errors.Is(err, sql.ErrNoRows) {
		return inv, ErrNotFound
	}
	if err != nil {
		return inv, fmt.Errorf("get invoker: %w", err)
	}
	return inv, nil
}

// CreateInteractionResponseMessage inserts a message with kind='interaction_response'
// authored by the bot, bypassing the normal SendMessage permission path. The
// caller has already validated via the interaction token that the bot is the
// rightful author.
func (r *Repository) CreateInteractionResponseMessage(ctx context.Context, channelID, authorID int64, content string) (int64, time.Time, error) {
	var id int64
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO messages (channel_id, author_id, content, via_api, kind, created_at, updated_at)
		VALUES ($1, $2, $3, TRUE, 'interaction_response', NOW(), NOW())
		RETURNING id, created_at`,
		channelID, authorID, content,
	).Scan(&id, &createdAt)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create interaction response message: %w", err)
	}
	return id, createdAt, nil
}

// ============ internal helpers ============

func scanCommands(rows *sql.Rows) ([]BotCommand, error) {
	out := []BotCommand{}
	for rows.Next() {
		var c BotCommand
		var optsRaw []byte
		if err := rows.Scan(&c.ID, &c.BotID, &c.ServerID, &c.Name, &c.Description, &optsRaw, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		if err := unmarshalOptions(optsRaw, &c.Options); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commands: %w", err)
	}
	return out, nil
}

func unmarshalOptions(raw []byte, dst *[]BotCommandOption) error {
	*dst = []BotCommandOption{}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("unmarshal options: %w", err)
	}
	if *dst == nil {
		*dst = []BotCommandOption{}
	}
	return nil
}

// isPgUniqueViolation detects Postgres unique-constraint violations (SQLSTATE 23505).
// Driver-agnostic via db.IsUniqueViolation so the check works under both pgx
// and lib/pq.
func isPgUniqueViolation(err error) bool {
	return db.IsUniqueViolation(err)
}
