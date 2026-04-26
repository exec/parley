package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	pq "github.com/lib/pq"
)

// ============ DM Channel Operations ============

// GetOrCreateDmChannel finds or creates a DM channel between two users.
func (r *Repository) GetOrCreateDmChannel(ctx context.Context, userAID, userBID int64) (*DmChannel, error) {
	// Ensure user1_id < user2_id for the UNIQUE constraint
	user1ID, user2ID := userAID, userBID
	if user1ID > user2ID {
		user1ID, user2ID = user2ID, user1ID
	}

	insertQuery := `
		INSERT INTO dm_channels (user1_id, user2_id, is_group, created_at)
		VALUES ($1, $2, FALSE, NOW())
		ON CONFLICT DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, insertQuery, user1ID, user2ID)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT id, COALESCE(user1_id, 0), COALESCE(user2_id, 0), created_at
		FROM dm_channels
		WHERE user1_id = $1 AND user2_id = $2 AND is_group = FALSE
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

	otherUserID := user2ID
	if userAID == user1ID {
		otherUserID = user2ID
	} else {
		otherUserID = user1ID
	}

	var otherUser User
	var otherDisplayName string
	err = r.db.QueryRowContext(ctx, "SELECT id, username, COALESCE(avatar_url, ''), COALESCE(display_name, '') FROM users WHERE id = $1", otherUserID).Scan(&otherUser.ID, &otherUser.Username, &otherUser.AvatarURL, &otherDisplayName)
	if err == nil {
		channel.OtherUserID = otherUser.ID
		channel.OtherUsername = otherUser.Username
		channel.OtherAvatarURL = otherUser.AvatarURL
		channel.OtherDisplayName = otherDisplayName
	}

	return &channel, nil
}

func (r *Repository) GetUserDmChannels(ctx context.Context, userID int64) ([]DmChannel, error) {
	// Each row returns the channel plus its last-activity timestamp:
	// max(dm_messages.created_at) for the channel, falling back to the
	// channel's own created_at when there are no messages yet. The two
	// queries (1:1 and group) sort independently; we merge + re-sort by
	// last_activity_at after scanning so the final list is uniformly
	// ordered most-recent-first regardless of channel kind.
	type rowWithActivity struct {
		ch     DmChannel
		lastAt time.Time
	}
	var rows []rowWithActivity

	oneToOneQuery := `
		SELECT dc.id, COALESCE(dc.user1_id, 0), COALESCE(dc.user2_id, 0), dc.created_at,
		       dc.is_group, dc.name, dc.avatar_url, dc.created_by_user_id, dc.owner_user_id,
		       u.id as other_user_id, u.username as other_username,
		       COALESCE(u.avatar_url, '') as other_avatar_url,
		       COALESCE(u.display_name, '') as other_display_name,
		       COALESCE(lm.last_at, dc.created_at) AS last_activity_at
		FROM dm_channels dc
		JOIN users u ON u.id = CASE WHEN dc.user1_id = $1 THEN dc.user2_id ELSE dc.user1_id END
		LEFT JOIN (
		    SELECT dm_channel_id, MAX(created_at) AS last_at
		    FROM dm_messages
		    GROUP BY dm_channel_id
		) lm ON lm.dm_channel_id = dc.id
		WHERE dc.is_group = FALSE AND (dc.user1_id = $1 OR dc.user2_id = $1)
	`
	oneToOneRows, err := r.db.QueryContext(ctx, oneToOneQuery, userID)
	if err != nil {
		return nil, err
	}
	defer oneToOneRows.Close()

	for oneToOneRows.Next() {
		var entry rowWithActivity
		err := oneToOneRows.Scan(
			&entry.ch.ID,
			&entry.ch.User1ID,
			&entry.ch.User2ID,
			&entry.ch.CreatedAt,
			&entry.ch.IsGroup,
			&entry.ch.Name,
			&entry.ch.AvatarURL,
			&entry.ch.CreatedByUserID,
			&entry.ch.OwnerUserID,
			&entry.ch.OtherUserID,
			&entry.ch.OtherUsername,
			&entry.ch.OtherAvatarURL,
			&entry.ch.OtherDisplayName,
			&entry.lastAt,
		)
		if err != nil {
			return nil, err
		}
		rows = append(rows, entry)
	}
	if err := oneToOneRows.Err(); err != nil {
		return nil, err
	}

	groupQuery := `
		SELECT dc.id, COALESCE(dc.user1_id, 0), COALESCE(dc.user2_id, 0), dc.created_at,
		       dc.is_group, dc.name, dc.avatar_url, dc.created_by_user_id, dc.owner_user_id,
		       COALESCE(lm.last_at, dc.created_at) AS last_activity_at
		FROM dm_channels dc
		JOIN dm_channel_members m ON m.dm_channel_id = dc.id
		LEFT JOIN (
		    SELECT dm_channel_id, MAX(created_at) AS last_at
		    FROM dm_messages
		    GROUP BY dm_channel_id
		) lm ON lm.dm_channel_id = dc.id
		WHERE dc.is_group = TRUE AND m.user_id = $1
	`
	groupRows, err := r.db.QueryContext(ctx, groupQuery, userID)
	if err != nil {
		return nil, err
	}
	defer groupRows.Close()

	for groupRows.Next() {
		var entry rowWithActivity
		err := groupRows.Scan(
			&entry.ch.ID,
			&entry.ch.User1ID,
			&entry.ch.User2ID,
			&entry.ch.CreatedAt,
			&entry.ch.IsGroup,
			&entry.ch.Name,
			&entry.ch.AvatarURL,
			&entry.ch.CreatedByUserID,
			&entry.ch.OwnerUserID,
			&entry.lastAt,
		)
		if err != nil {
			return nil, err
		}
		members, merr := r.GetDmMembers(ctx, entry.ch.ID)
		if merr == nil {
			entry.ch.Members = members
		}
		rows = append(rows, entry)
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].lastAt.After(rows[j].lastAt)
	})

	channels := make([]DmChannel, len(rows))
	for i, r := range rows {
		channels[i] = r.ch
	}
	return channels, nil
}

// GetDmChannelForUser returns a DmChannel with other_* fields populated from the
// given viewer's perspective — same shape as GetUserDmChannels rows. Used to
// broadcast the DM_CHANNEL_CREATE event with display info for the recipient.
func (r *Repository) GetDmChannelForUser(ctx context.Context, dmChannelID, viewerID int64) (*DmChannel, error) {
	// Only meaningful for 1:1 channels; group channels surface via the
	// CreateChannel fan-out which already includes the full ch object.
	query := `
		SELECT dc.id, COALESCE(dc.user1_id, 0), COALESCE(dc.user2_id, 0), dc.created_at,
		       u.id, u.username, COALESCE(u.avatar_url, ''), COALESCE(u.display_name, '')
		FROM dm_channels dc
		JOIN users u ON u.id = CASE WHEN dc.user1_id = $2 THEN dc.user2_id ELSE dc.user1_id END
		WHERE dc.id = $1 AND dc.is_group = FALSE
	`
	var channel DmChannel
	err := r.db.QueryRowContext(ctx, query, dmChannelID, viewerID).Scan(
		&channel.ID,
		&channel.User1ID,
		&channel.User2ID,
		&channel.CreatedAt,
		&channel.OtherUserID,
		&channel.OtherUsername,
		&channel.OtherAvatarURL,
		&channel.OtherDisplayName,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// CountDmMessages returns the total number of messages in a DM channel.
// Used to detect the "first message" case for DM_CHANNEL_CREATE broadcasts.
func (r *Repository) CountDmMessages(ctx context.Context, dmChannelID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM dm_messages WHERE dm_channel_id = $1", dmChannelID,
	).Scan(&count)
	return count, err
}

func (r *Repository) GetDmChannelByID(ctx context.Context, id int64) (*DmChannel, error) {
	// Select all columns including the group-related fields so the service
	// layer can branch on is_group / owner_user_id without a second roundtrip.
	// COALESCE on user1_id/user2_id maps NULL (group channels post-migration #66)
	// to 0 — call sites that read these branch on IsGroup first, so the
	// sentinel never leaks into business logic.
	query := `
		SELECT id, COALESCE(user1_id, 0), COALESCE(user2_id, 0), created_at,
		       is_group, name, avatar_url, created_by_user_id, owner_user_id
		FROM dm_channels
		WHERE id = $1
	`

	var channel DmChannel
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID,
		&channel.User1ID,
		&channel.User2ID,
		&channel.CreatedAt,
		&channel.IsGroup,
		&channel.Name,
		&channel.AvatarURL,
		&channel.CreatedByUserID,
		&channel.OwnerUserID,
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

func (r *Repository) CreateDmMessage(ctx context.Context, dmChannelID, authorID int64, content, attachmentURL, attachmentName, attachmentType string, parentID *int64) (*DmMessage, error) {
	query := `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, attachment_url, attachment_name, attachment_type, parent_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, dm_channel_id, author_id, content, COALESCE(attachment_url,''), COALESCE(attachment_name,''), COALESCE(attachment_type,''), parent_id, created_at, updated_at
	`

	var msg DmMessage
	err := r.db.QueryRowContext(ctx, query, dmChannelID, authorID, content, attachmentURL, attachmentName, attachmentType, parentID).Scan(
		&msg.ID,
		&msg.DmChannelID,
		&msg.AuthorID,
		&msg.Content,
		&msg.AttachmentURL,
		&msg.AttachmentName,
		&msg.AttachmentType,
		&msg.ParentID,
		&msg.CreatedAt,
		&msg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	var username, avatarURL, displayName string
	r.db.QueryRowContext(ctx, "SELECT username, COALESCE(avatar_url, ''), COALESCE(display_name,'') FROM users WHERE id = $1", authorID).Scan(&username, &avatarURL, &displayName)
	msg.AuthorUsername = username
	msg.AuthorAvatarURL = avatarURL
	msg.AuthorDisplayName = displayName

	if msg.ParentID != nil {
		var parentAuthorID int64
		r.db.QueryRowContext(ctx,
			"SELECT author_id FROM dm_messages WHERE id = $1", *msg.ParentID,
		).Scan(&parentAuthorID)
		if parentAuthorID != 0 {
			var parentUsername, parentDisplayName string
			r.db.QueryRowContext(ctx,
				"SELECT username, COALESCE(display_name,'') FROM users WHERE id = $1", parentAuthorID,
			).Scan(&parentUsername, &parentDisplayName)
			msg.ParentAuthorUsername = parentUsername
			msg.ParentAuthorDisplayName = parentDisplayName
		}
	}

	msg.Reactions = []ReactionGroup{}

	return &msg, nil
}

// GetDmMessages retrieves messages for a DM channel with cursor-based pagination.
// If beforeID > 0, returns messages with id < beforeID (older messages).
// Otherwise returns the latest `limit` messages.
// Results are always returned in ascending order (oldest first).
func (r *Repository) GetDmMessages(ctx context.Context, dmChannelID int64, limit int, beforeID int64) ([]DmMessage, error) {
	var query string
	var rows *sql.Rows
	var err error

	if beforeID > 0 {
		query = `
			SELECT * FROM (
				SELECT m.id, m.dm_channel_id, m.author_id, m.content, m.parent_id,
				       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
				       m.created_at, m.updated_at, u.username, COALESCE(u.avatar_url, ''), COALESCE(u.display_name, ''),
				       m.forwarded_data, m.system_event
				FROM dm_messages m
				JOIN users u ON u.id = m.author_id
				WHERE m.dm_channel_id = $1 AND m.id < $3
				ORDER BY m.id DESC
				LIMIT $2
			) sub
			ORDER BY id ASC
		`
		rows, err = r.db.QueryContext(ctx, query, dmChannelID, limit, beforeID)
	} else {
		query = `
			SELECT * FROM (
				SELECT m.id, m.dm_channel_id, m.author_id, m.content, m.parent_id,
				       COALESCE(m.attachment_url, ''), COALESCE(m.attachment_name, ''), COALESCE(m.attachment_type, ''),
				       m.created_at, m.updated_at, u.username, COALESCE(u.avatar_url, ''), COALESCE(u.display_name, ''),
				       m.forwarded_data, m.system_event
				FROM dm_messages m
				JOIN users u ON u.id = m.author_id
				WHERE m.dm_channel_id = $1
				ORDER BY m.id DESC
				LIMIT $2
			) sub
			ORDER BY id ASC
		`
		rows, err = r.db.QueryContext(ctx, query, dmChannelID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []DmMessage
	for rows.Next() {
		var msg DmMessage
		var fwdJSON []byte
		var sysJSON []byte
		err := rows.Scan(
			&msg.ID,
			&msg.DmChannelID,
			&msg.AuthorID,
			&msg.Content,
			&msg.ParentID,
			&msg.AttachmentURL,
			&msg.AttachmentName,
			&msg.AttachmentType,
			&msg.CreatedAt,
			&msg.UpdatedAt,
			&msg.AuthorUsername,
			&msg.AuthorAvatarURL,
			&msg.AuthorDisplayName,
			&fwdJSON,
			&sysJSON,
		)
		if err != nil {
			return nil, err
		}
		if len(fwdJSON) > 0 {
			var fwd ForwardedMessageData
			if err := json.Unmarshal(fwdJSON, &fwd); err == nil {
				msg.ForwardedMessage = &fwd
			}
		}
		if len(sysJSON) > 0 {
			raw := json.RawMessage(sysJSON)
			msg.SystemEvent = &raw
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch reactions for all messages
	if len(messages) > 0 {
		ids := make([]int64, len(messages))
		for i, m := range messages {
			ids[i] = m.ID
		}
		reactions, err := r.GetDmReactionsForMessages(ctx, ids)
		if err == nil {
			for i, m := range messages {
				if rg, ok := reactions[m.ID]; ok {
					messages[i].Reactions = rg
				} else {
					messages[i].Reactions = []ReactionGroup{}
				}
			}
		}
		// Fetch parent author info for replies
		for i, m := range messages {
			if m.ParentID == nil {
				messages[i].Reactions = messages[i].Reactions // no-op, ensure slice not nil
				continue
			}
			var parentAuthorID int64
			r.db.QueryRowContext(ctx, "SELECT author_id FROM dm_messages WHERE id = $1", *m.ParentID).Scan(&parentAuthorID)
			if parentAuthorID != 0 {
				var username, displayName string
				r.db.QueryRowContext(ctx, "SELECT username, COALESCE(display_name,'') FROM users WHERE id = $1", parentAuthorID).Scan(&username, &displayName)
				messages[i].ParentAuthorUsername = username
				messages[i].ParentAuthorDisplayName = displayName
			}
		}
	}

	return messages, nil
}

// CreateForwardedDmMessage inserts a forwarded-message card into a DM channel.
func (r *Repository) CreateForwardedDmMessage(ctx context.Context, dmChannelID, authorID int64, fwd *ForwardedMessageData) (*DmMessage, error) {
	fwdJSON, err := json.Marshal(fwd)
	if err != nil {
		return nil, err
	}
	query := `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, forwarded_data, created_at, updated_at)
		VALUES ($1, $2, '', $3, NOW(), NOW())
		RETURNING id, dm_channel_id, author_id, content, parent_id, created_at, updated_at
	`
	var msg DmMessage
	err = r.db.QueryRowContext(ctx, query, dmChannelID, authorID, fwdJSON).Scan(
		&msg.ID, &msg.DmChannelID, &msg.AuthorID, &msg.Content, &msg.ParentID,
		&msg.CreatedAt, &msg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	r.db.QueryRowContext(ctx,
		"SELECT username, COALESCE(avatar_url,''), COALESCE(display_name,'') FROM users WHERE id = $1", authorID,
	).Scan(&msg.AuthorUsername, &msg.AuthorAvatarURL, &msg.AuthorDisplayName)
	msg.ForwardedMessage = fwd
	msg.Reactions = []ReactionGroup{}
	return &msg, nil
}

// DeleteDmMessage deletes a DM message. Only the author may delete.
func (r *Repository) DeleteDmMessage(ctx context.Context, messageID, authorID int64) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM dm_messages WHERE id = $1 AND author_id = $2",
		messageID, authorID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetDmMessageChannelID returns the dm_channel_id for a message (for broadcast).
func (r *Repository) GetDmMessageChannelID(ctx context.Context, messageID int64) (int64, error) {
	var channelID int64
	err := r.db.QueryRowContext(ctx,
		"SELECT dm_channel_id FROM dm_messages WHERE id = $1", messageID).Scan(&channelID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return channelID, err
}

// ToggleDmReaction adds or removes a reaction on a DM message.
func (r *Repository) ToggleDmReaction(ctx context.Context, messageID, userID int64, emoji string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM dm_message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3",
		messageID, userID, emoji)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		return false, nil // removed
	}
	_, err = r.db.ExecContext(ctx,
		"INSERT INTO dm_message_reactions(message_id, user_id, emoji) VALUES($1, $2, $3)",
		messageID, userID, emoji)
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetDmReactionsForMessages fetches reaction groups for a set of DM message IDs.
func (r *Repository) GetDmReactionsForMessages(ctx context.Context, messageIDs []int64) (map[int64][]ReactionGroup, error) {
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
        FROM dm_message_reactions
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

// ============ Group DM Operations ============

// CreateGroupChannel creates a new group DM channel with the given creator and member set.
// Creator is also the owner. The legacy user1_id/user2_id columns are set to NULL for
// group channels — readers should branch on is_group rather than relying on those.
// Members are inserted in the same transaction as the channel.
func (r *Repository) CreateGroupChannel(ctx context.Context, creatorUserID int64, name string, memberUserIDs []int64) (*DmChannel, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var ch DmChannel
	err = tx.QueryRowContext(ctx, `
		INSERT INTO dm_channels (user1_id, user2_id, is_group, name, created_by_user_id, owner_user_id, created_at)
		VALUES (NULL, NULL, TRUE, $1, $2, $2, NOW())
		RETURNING id, is_group, name, created_by_user_id, owner_user_id, created_at
	`, name, creatorUserID).Scan(&ch.ID, &ch.IsGroup, &ch.Name, &ch.CreatedByUserID, &ch.OwnerUserID, &ch.CreatedAt)
	if err != nil {
		return nil, err
	}

	for _, uid := range memberUserIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT DO NOTHING
		`, ch.ID, uid); err != nil {
			return nil, err
		}
	}
	return &ch, tx.Commit()
}

func (r *Repository) AddDmMember(ctx context.Context, channelID, userID int64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dm_channel_members (dm_channel_id, user_id, joined_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT DO NOTHING
	`, channelID, userID)
	return err
}

func (r *Repository) RemoveDmMember(ctx context.Context, channelID, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dm_channel_members WHERE dm_channel_id = $1 AND user_id = $2`, channelID, userID)
	return err
}

// IsDmMember returns true when userID is a member of the DM. Group DMs store
// membership in dm_channel_members; 1:1 DMs store it in user1_id/user2_id, and
// newer 1:1 channels do not populate the join table at all — so this lookup
// must consult both shapes.
func (r *Repository) IsDmMember(ctx context.Context, channelID, userID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		  FROM dm_channels c
		 WHERE c.id = $1
		   AND (
		         c.user1_id = $2
		      OR c.user2_id = $2
		      OR EXISTS (
		           SELECT 1 FROM dm_channel_members m
		            WHERE m.dm_channel_id = c.id AND m.user_id = $2
		         )
		       )
	`, channelID, userID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return n == 1, err
}

func (r *Repository) GetDmMembers(ctx context.Context, channelID int64) ([]DmChannelMember, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.dm_channel_id, m.user_id, m.joined_at, u.username,
		       COALESCE(u.display_name, u.username), COALESCE(u.avatar_url, ''),
		       COALESCE(u.banner_url, ''), COALESCE(u.bio, ''), u.badges
		  FROM dm_channel_members m
		  JOIN users u ON u.id = m.user_id
		 WHERE m.dm_channel_id = $1
		 ORDER BY m.joined_at
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DmChannelMember
	for rows.Next() {
		var m DmChannelMember
		if err := rows.Scan(&m.DmChannelID, &m.UserID, &m.JoinedAt, &m.Username, &m.DisplayName, &m.AvatarURL, &m.BannerURL, &m.Bio, &m.Badges); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateDmGroupName(ctx context.Context, channelID int64, name string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dm_channels SET name = $1 WHERE id = $2 AND is_group = TRUE`, name, channelID)
	return err
}

func (r *Repository) UpdateDmGroupAvatar(ctx context.Context, channelID int64, avatarURL *string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dm_channels SET avatar_url = $1 WHERE id = $2 AND is_group = TRUE`, avatarURL, channelID)
	return err
}

func (r *Repository) TransferDmGroupOwnership(ctx context.Context, channelID int64, newOwnerID *int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE dm_channels SET owner_user_id = $1 WHERE id = $2 AND is_group = TRUE`, newOwnerID, channelID)
	return err
}

// InsertSystemMessage adds a synthetic dm_messages row carrying the given system event.
// content is empty; system_event is the JSONB payload.
func (r *Repository) InsertSystemMessage(ctx context.Context, channelID int64, actorUserID int64, eventJSON []byte) (*DmMessage, error) {
	var m DmMessage
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO dm_messages (dm_channel_id, author_id, content, system_event, created_at, updated_at)
		VALUES ($1, $2, '', $3::jsonb, NOW(), NOW())
		RETURNING id, dm_channel_id, author_id, content, system_event, created_at, updated_at
	`, channelID, actorUserID, eventJSON).Scan(&m.ID, &m.DmChannelID, &m.AuthorID, &m.Content, &m.SystemEvent, &m.CreatedAt, &m.UpdatedAt)
	return &m, err
}
