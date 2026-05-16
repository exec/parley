package bin

import (
	"context"
	"errors"
	"strconv"

	"parley/internal/db"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

// CreateLineComment creates a new line comment on a bin post.
func (s *Service) CreateLineComment(ctx context.Context, postID, userID string, versionID, fileID string, lineNumber int, content string, parentID string) (*db.BinLineComment, error) {
	if content == "" {
		return nil, errors.New("content is required")
	}

	pID, err := strconv.ParseInt(postID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid post ID")
	}
	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}
	vID, err := strconv.ParseInt(versionID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid version ID")
	}
	fID, err := strconv.ParseInt(fileID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid file ID")
	}

	var parentIDPtr *int64
	if parentID != "" {
		pid, err := strconv.ParseInt(parentID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid parent ID")
		}
		parentIDPtr = &pid
	}

	// Check SendMessages permission on the post's bin channel.
	post, err := s.repo.GetBinPost(ctx, pID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrPostNotFound
		}
		return nil, err
	}
	serverID, ownerID, err := s.getServerForChannel(ctx, post.ChannelID)
	if err != nil {
		return nil, err
	}
	// Fetch the channel mask once; check ViewChannel (404) and SendMessages
	// (403) in-process.
	mask, err := permissions.GetEffectiveChannelPermissions(ctx, s.repo, s.memberCache, serverID, uID, ownerID, post.ChannelID)
	if err != nil {
		return nil, err
	}
	if !permissions.HasPerm(mask, permissions.PermViewChannel) {
		return nil, ErrPostNotFound
	}
	if !permissions.HasPerm(mask, permissions.PermSendMessages) {
		return nil, ErrForbidden
	}

	comment, err := s.repo.CreateBinLineComment(ctx, pID, vID, fID, lineNumber, uID, content, parentIDPtr)
	if err != nil {
		return nil, err
	}

	// Broadcast to the post's thread channel.
	threadChannelID := strconv.FormatInt(post.ThreadChannelID, 10)
	s.broadcast(threadChannelID, ws.EventBinLineCommentCreate, comment)

	return comment, nil
}

// GetLineComments retrieves line comments for a post with optional version/file filters.
// userID is used for ViewChannel check; pass "" to skip.
func (s *Service) GetLineComments(ctx context.Context, postID, userID string, versionID, fileID *string) ([]db.BinLineComment, error) {
	pID, err := strconv.ParseInt(postID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid post ID")
	}

	// ViewChannel check.
	if userID != "" {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid user ID")
		}
		post, err := s.repo.GetBinPost(ctx, pID)
		if err != nil {
			if err == db.ErrNotFound {
				return nil, ErrPostNotFound
			}
			return nil, err
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, post.ChannelID)
		if err == nil {
			canView, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, post.ChannelID, permissions.PermViewChannel)
			if err != nil {
				return nil, err
			}
			if !canView {
				return nil, ErrPostNotFound
			}
		}
	}

	var vIDPtr, fIDPtr *int64
	if versionID != nil && *versionID != "" {
		v, err := strconv.ParseInt(*versionID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid version ID")
		}
		vIDPtr = &v
	}
	if fileID != nil && *fileID != "" {
		f, err := strconv.ParseInt(*fileID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid file ID")
		}
		fIDPtr = &f
	}

	comments, err := s.repo.GetBinLineComments(ctx, pID, vIDPtr, fIDPtr)
	if err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []db.BinLineComment{}
	}
	return comments, nil
}

// UpdateLineComment updates the content of a line comment.
func (s *Service) UpdateLineComment(ctx context.Context, commentID, userID, content string) (*db.BinLineComment, error) {
	if content == "" {
		return nil, errors.New("content is required")
	}

	cID, err := strconv.ParseInt(commentID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid comment ID")
	}
	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	// Check author.
	authorID, err := s.repo.GetBinLineCommentAuthorID(ctx, cID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}
	if authorID != uID {
		return nil, ErrForbidden
	}

	comment, err := s.repo.UpdateBinLineComment(ctx, cID, content)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}

	// Broadcast to post's thread channel.
	post, err := s.repo.GetBinPost(ctx, comment.PostID)
	if err == nil {
		threadChannelID := strconv.FormatInt(post.ThreadChannelID, 10)
		s.broadcast(threadChannelID, ws.EventBinLineCommentUpdate, comment)
	}

	return comment, nil
}

// DeleteLineComment deletes a line comment by ID.
func (s *Service) DeleteLineComment(ctx context.Context, commentID, userID string) error {
	cID, err := strconv.ParseInt(commentID, 10, 64)
	if err != nil {
		return errors.New("invalid comment ID")
	}
	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return errors.New("invalid user ID")
	}

	// Check author.
	authorID, err := s.repo.GetBinLineCommentAuthorID(ctx, cID)
	if err != nil {
		if err == db.ErrNotFound {
			return ErrCommentNotFound
		}
		return err
	}
	if authorID != uID {
		return ErrForbidden
	}

	// Fetch comment for broadcast before deleting.
	// We need the post ID to find the thread channel.
	// GetBinLineCommentAuthorID only returns the author, so we look up the comment via UpdateBinLineComment trick.
	// Instead, just delete and broadcast with the comment ID only.
	if err := s.repo.DeleteBinLineComment(ctx, cID); err != nil {
		return err
	}

	s.broadcast("", ws.EventBinLineCommentDelete, map[string]string{
		"id": commentID,
	})

	return nil
}
