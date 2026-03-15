package db

import (
	"context"
	"fmt"
	"strings"

	pq "github.com/lib/pq"
)

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
