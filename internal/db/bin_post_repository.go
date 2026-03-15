package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/lib/pq"
)

// slugify returns a lowercase slug of up to maxLen characters from s.
func slugify(s string, maxLen int) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(strings.ToLower(s), "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > maxLen {
		slug = slug[:maxLen]
	}
	if slug == "" {
		slug = "thread"
	}
	return slug
}

// CreateBinPost inserts a new bin post, creates its thread channel and initial version.
func (r *Repository) CreateBinPost(ctx context.Context, channelID, authorID int64, title, description string, tags []string) (*BinPost, error) {
	post := &BinPost{
		ChannelID:   channelID,
		AuthorID:    authorID,
		Title:       title,
		Description: description,
		Tags:        pq.StringArray(tags),
	}

	err := r.db.QueryRowContext(ctx,
		`INSERT INTO bin_posts (channel_id, author_id, title, description, tags)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at, updated_at`,
		channelID, authorID, title, description, pq.Array(tags),
	).Scan(&post.ID, &post.CreatedAt, &post.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Look up the server_id for the parent bin channel so we can create the thread channel.
	var serverID int64
	err = r.db.QueryRowContext(ctx,
		`SELECT server_id FROM channels WHERE id = $1`, channelID,
	).Scan(&serverID)
	if err != nil {
		return nil, err
	}

	// Create thread channel (type=text/0), parented to the bin channel.
	threadName := slugify(title, 30)
	var threadChannelID int64
	err = r.db.QueryRowContext(ctx,
		`INSERT INTO channels (server_id, name, channel_type, position, parent_id, topic, created_at)
		 VALUES ($1, $2, 0, 0, $3, '', NOW())
		 RETURNING id`,
		serverID, threadName, channelID,
	).Scan(&threadChannelID)
	if err != nil {
		return nil, err
	}

	// Update the post with the thread_channel_id.
	_, err = r.db.ExecContext(ctx,
		`UPDATE bin_posts SET thread_channel_id = $1 WHERE id = $2`,
		threadChannelID, post.ID,
	)
	if err != nil {
		return nil, err
	}
	post.ThreadChannelID = threadChannelID

	// Create initial version (version 1).
	_, err = r.CreateBinPostVersion(ctx, post.ID, 1, "Initial version")
	if err != nil {
		return nil, err
	}

	return post, nil
}

// GetBinPost retrieves a bin post by ID including author info and computed counts.
func (r *Repository) GetBinPost(ctx context.Context, postID int64) (*BinPost, error) {
	query := `
		SELECT
			bp.id, bp.channel_id, bp.thread_channel_id, bp.author_id, bp.title, bp.description,
			bp.tags, bp.created_at, bp.updated_at,
			u.username, COALESCE(u.avatar_url, ''),
			(SELECT COUNT(*) FROM bin_post_versions WHERE post_id = bp.id) AS version_count,
			(SELECT COUNT(*) FROM messages WHERE channel_id = bp.thread_channel_id) AS comment_count,
			(SELECT COUNT(*) FROM bin_line_comments WHERE post_id = bp.id) AS line_comment_count
		FROM bin_posts bp
		JOIN users u ON u.id = bp.author_id
		WHERE bp.id = $1
	`

	var post BinPost
	err := r.db.QueryRowContext(ctx, query, postID).Scan(
		&post.ID, &post.ChannelID, &post.ThreadChannelID, &post.AuthorID,
		&post.Title, &post.Description, pq.Array(&post.Tags),
		&post.CreatedAt, &post.UpdatedAt,
		&post.AuthorUsername, &post.AuthorAvatarURL,
		&post.VersionCount, &post.CommentCount, &post.LineCommentCount,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

// GetBinPostsByChannel retrieves bin posts for a channel with optional filtering and sorting.
func (r *Repository) GetBinPostsByChannel(ctx context.Context, channelID int64, tag, language, authorID string, sort string, limit, offset int) ([]BinPost, error) {
	args := []interface{}{channelID}
	where := []string{"bp.channel_id = $1"}

	if tag != "" {
		args = append(args, tag)
		where = append(where, fmt.Sprintf("$%d = ANY(bp.tags)", len(args)))
	}
	if language != "" {
		args = append(args, language)
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM bin_post_files WHERE post_id = bp.id AND language = $%d)",
			len(args),
		))
	}
	if authorID != "" {
		args = append(args, authorID)
		where = append(where, fmt.Sprintf("bp.author_id = $%d", len(args)))
	}

	orderBy := "ORDER BY bp.created_at DESC"
	switch sort {
	case "oldest":
		orderBy = "ORDER BY bp.created_at ASC"
	case "recently_active":
		orderBy = "ORDER BY bp.updated_at DESC"
	}

	args = append(args, limit, offset)
	sqlStr := fmt.Sprintf(`
		SELECT
			bp.id, bp.channel_id, bp.thread_channel_id, bp.author_id, bp.title, bp.description,
			bp.tags, bp.created_at, bp.updated_at,
			u.username, COALESCE(u.avatar_url, ''),
			(SELECT COUNT(*) FROM bin_post_versions WHERE post_id = bp.id) AS version_count,
			(SELECT COUNT(*) FROM messages WHERE channel_id = bp.thread_channel_id) AS comment_count,
			(SELECT COUNT(*) FROM bin_line_comments WHERE post_id = bp.id) AS line_comment_count
		FROM bin_posts bp
		JOIN users u ON u.id = bp.author_id
		WHERE %s
		%s
		LIMIT $%d OFFSET $%d`,
		strings.Join(where, " AND "),
		orderBy,
		len(args)-1,
		len(args),
	)

	rows, err := r.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []BinPost
	for rows.Next() {
		var post BinPost
		err := rows.Scan(
			&post.ID, &post.ChannelID, &post.ThreadChannelID, &post.AuthorID,
			&post.Title, &post.Description, pq.Array(&post.Tags),
			&post.CreatedAt, &post.UpdatedAt,
			&post.AuthorUsername, &post.AuthorAvatarURL,
			&post.VersionCount, &post.CommentCount, &post.LineCommentCount,
		)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

// UpdateBinPost updates the title, description, and tags of a bin post.
func (r *Repository) UpdateBinPost(ctx context.Context, postID int64, title, description string, tags []string) (*BinPost, error) {
	var post BinPost
	err := r.db.QueryRowContext(ctx,
		`UPDATE bin_posts
		 SET title = $1, description = $2, tags = $3, updated_at = NOW()
		 WHERE id = $4
		 RETURNING id, channel_id, thread_channel_id, author_id, title, description, tags, created_at, updated_at`,
		title, description, pq.Array(tags), postID,
	).Scan(
		&post.ID, &post.ChannelID, &post.ThreadChannelID, &post.AuthorID,
		&post.Title, &post.Description, pq.Array(&post.Tags),
		&post.CreatedAt, &post.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

// DeleteBinPost deletes a bin post by ID. Cascades handle cleanup.
func (r *Repository) DeleteBinPost(ctx context.Context, postID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM bin_posts WHERE id = $1`, postID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetBinPostAuthorID returns the author_id of a bin post.
func (r *Repository) GetBinPostAuthorID(ctx context.Context, postID int64) (int64, error) {
	var authorID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT author_id FROM bin_posts WHERE id = $1`, postID,
	).Scan(&authorID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return authorID, err
}
