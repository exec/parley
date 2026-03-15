package bin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"

	"parley/internal/db"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

// Service provides bin post management operations.
type Service struct {
	mu   sync.RWMutex
	repo *db.Repository
	hub  *ws.Hub
}

// NewService creates a new bin Service with the given repository.
func NewService(repo *db.Repository) *Service {
	return &Service{repo: repo}
}

// SetHub sets the WebSocket hub for broadcasting events.
func (s *Service) SetHub(hub *ws.Hub) {
	s.mu.Lock()
	s.hub = hub
	s.mu.Unlock()
}

// getServerForChannel returns the server ID and owner ID for a channel.
func (s *Service) getServerForChannel(ctx context.Context, channelIDInt int64) (serverID, ownerID int64, err error) {
	ch, err := s.repo.GetChannelByID(ctx, channelIDInt)
	if err != nil {
		return 0, 0, err
	}
	srv, err := s.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil {
		return 0, 0, err
	}
	return srv.ID, srv.OwnerID, nil
}

func (s *Service) broadcast(channelID string, event string, data interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hub != nil {
		payload, err := json.Marshal(data)
		if err != nil {
			log.Printf("bin.Service.broadcast: marshal error: %v", err)
			return
		}
		s.hub.BroadcastToChannel(channelID, event, payload)
	}
}

// CreatePost creates a new bin post in the given channel.
func (s *Service) CreatePost(ctx context.Context, channelID, userID string, title, description string, tags []string, files []db.BinPostFile) (*db.BinPost, error) {
	if title == "" {
		return nil, errors.New("title is required")
	}

	chID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}
	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	// Validate that the channel is a bin channel.
	ch, err := s.repo.GetChannelByID(ctx, chID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("channel not found")
		}
		return nil, err
	}
	if ch.ChannelType != db.ChannelTypeBin {
		return nil, errors.New("channel is not a bin channel")
	}

	// Permission checks: ViewChannel (404 if denied) and CreatePosts.
	serverID, ownerID, err := s.getServerForChannel(ctx, chID)
	if err != nil {
		return nil, err
	}
	canView, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, chID, permissions.PermViewChannel)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, errors.New("channel not found")
	}
	canCreate, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, chID, permissions.PermCreatePosts)
	if err != nil {
		return nil, err
	}
	if !canCreate {
		return nil, errors.New("forbidden")
	}

	// CreateBinPost also creates the thread channel and initial version 1.
	post, err := s.repo.CreateBinPost(ctx, chID, uID, title, description, tags)
	if err != nil {
		return nil, err
	}

	// Insert the files for the post.
	if len(files) > 0 {
		createdFiles, err := s.repo.CreateBinPostFiles(ctx, post.ID, files)
		if err != nil {
			return nil, err
		}

		// Snapshot files into version 1 (already created by CreateBinPost).
		versions, err := s.repo.GetBinPostVersions(ctx, post.ID)
		if err != nil {
			return nil, err
		}
		if len(versions) > 0 {
			if err := s.repo.CreateBinPostVersionFiles(ctx, versions[0].ID, createdFiles); err != nil {
				return nil, fmt.Errorf("snapshot version files: %w", err)
			}
		}
	}

	// Fetch the full post (with counts).
	fullPost, err := s.repo.GetBinPost(ctx, post.ID)
	if err != nil {
		return nil, err
	}

	// Attach files.
	postFiles, err := s.repo.GetBinPostFiles(ctx, post.ID)
	if err != nil {
		return nil, err
	}
	fullPost.Files = postFiles

	s.broadcast(channelID, ws.EventBinPostCreate, fullPost)

	return fullPost, nil
}

// GetPost retrieves a bin post by ID including its files.
// userID is used for ViewChannel check; pass "" to skip.
func (s *Service) GetPost(ctx context.Context, postID, userID string) (*db.BinPost, error) {
	id, err := strconv.ParseInt(postID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid post ID")
	}

	post, err := s.repo.GetBinPost(ctx, id)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("post not found")
		}
		return nil, err
	}

	// ViewChannel check.
	if userID != "" {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid user ID")
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, post.ChannelID)
		if err == nil {
			canView, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, post.ChannelID, permissions.PermViewChannel)
			if err != nil {
				return nil, err
			}
			if !canView {
				return nil, errors.New("post not found")
			}
		}
	}

	files, err := s.repo.GetBinPostFiles(ctx, id)
	if err != nil {
		return nil, err
	}
	post.Files = files

	return post, nil
}

// ListPosts retrieves bin posts for a channel with optional filters.
// userID is used for ViewChannel check; pass "" to skip.
func (s *Service) ListPosts(ctx context.Context, channelID, userID string, tag, language, authorID, sort string, limit, offset int) ([]db.BinPost, error) {
	chID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	// Validate channel type.
	ch, err := s.repo.GetChannelByID(ctx, chID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("channel not found")
		}
		return nil, err
	}
	if ch.ChannelType != db.ChannelTypeBin {
		return nil, errors.New("channel is not a bin channel")
	}

	// ViewChannel check.
	if userID != "" {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid user ID")
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, chID)
		if err == nil {
			canView, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, chID, permissions.PermViewChannel)
			if err != nil {
				return nil, err
			}
			if !canView {
				return nil, errors.New("channel not found")
			}
		}
	}

	if limit <= 0 {
		limit = 25
	}
	if limit > 50 {
		limit = 50
	}

	posts, err := s.repo.GetBinPostsByChannel(ctx, chID, tag, language, authorID, sort, limit, offset)
	if err != nil {
		return nil, err
	}
	if posts == nil {
		posts = []db.BinPost{}
	}
	return posts, nil
}

// EditPost updates a bin post's title, description, tags, and files.
// It snapshots the current files as a new version before replacing them.
func (s *Service) EditPost(ctx context.Context, postID, userID string, title, description string, tags []string, files []db.BinPostFile) (*db.BinPost, error) {
	id, err := strconv.ParseInt(postID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid post ID")
	}
	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	// Check author or ManagePosts permission.
	postAuthorID, err := s.repo.GetBinPostAuthorID(ctx, id)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("post not found")
		}
		return nil, err
	}
	if postAuthorID != uID {
		// Not the author — check ManagePosts channel permission.
		post, err := s.repo.GetBinPost(ctx, id)
		if err != nil {
			return nil, err
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, post.ChannelID)
		if err != nil {
			return nil, err
		}
		canManage, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, post.ChannelID, permissions.PermManagePosts)
		if err != nil {
			return nil, err
		}
		if !canManage {
			return nil, errors.New("forbidden")
		}
	}

	// Snapshot current files as a new version.
	latest, err := s.repo.GetLatestVersionNumber(ctx, id)
	if err != nil {
		return nil, err
	}
	newVersionNum := latest + 1
	version, err := s.repo.CreateBinPostVersion(ctx, id, newVersionNum, fmt.Sprintf("Version %d", newVersionNum))
	if err != nil {
		return nil, err
	}
	currentFiles, err := s.repo.GetBinPostFiles(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(currentFiles) > 0 {
		if err := s.repo.CreateBinPostVersionFiles(ctx, version.ID, currentFiles); err != nil {
			return nil, err
		}
	}

	// Replace files.
	if _, err := s.repo.ReplaceBinPostFiles(ctx, id, files); err != nil {
		return nil, err
	}

	// Update post metadata.
	if _, err := s.repo.UpdateBinPost(ctx, id, title, description, tags); err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("post not found")
		}
		return nil, err
	}

	// Fetch full updated post.
	fullPost, err := s.repo.GetBinPost(ctx, id)
	if err != nil {
		return nil, err
	}
	newFiles, err := s.repo.GetBinPostFiles(ctx, id)
	if err != nil {
		return nil, err
	}
	fullPost.Files = newFiles

	channelID := strconv.FormatInt(fullPost.ChannelID, 10)
	s.broadcast(channelID, ws.EventBinPostUpdate, fullPost)

	return fullPost, nil
}

// DeletePost deletes a bin post by ID.
func (s *Service) DeletePost(ctx context.Context, postID, userID string) error {
	id, err := strconv.ParseInt(postID, 10, 64)
	if err != nil {
		return errors.New("invalid post ID")
	}
	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return errors.New("invalid user ID")
	}

	// Check author or ManagePosts permission.
	postAuthorID, err := s.repo.GetBinPostAuthorID(ctx, id)
	if err != nil {
		if err == db.ErrNotFound {
			return errors.New("post not found")
		}
		return err
	}
	if postAuthorID != uID {
		// Not the author — check ManagePosts channel permission.
		post, err := s.repo.GetBinPost(ctx, id)
		if err != nil {
			return err
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, post.ChannelID)
		if err != nil {
			return err
		}
		canManage, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, post.ChannelID, permissions.PermManagePosts)
		if err != nil {
			return err
		}
		if !canManage {
			return errors.New("forbidden")
		}
	}

	// Fetch post to get channelID for broadcast before deleting.
	post, err := s.repo.GetBinPost(ctx, id)
	if err != nil {
		return err
	}
	channelID := strconv.FormatInt(post.ChannelID, 10)

	if err := s.repo.DeleteBinPost(ctx, id); err != nil {
		return err
	}

	s.broadcast(channelID, ws.EventBinPostDelete, map[string]string{
		"id":         postID,
		"channel_id": channelID,
	})

	return nil
}

// GetVersions retrieves all versions for a bin post.
func (s *Service) GetVersions(ctx context.Context, postID string) ([]db.BinPostVersion, error) {
	id, err := strconv.ParseInt(postID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid post ID")
	}
	versions, err := s.repo.GetBinPostVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	if versions == nil {
		versions = []db.BinPostVersion{}
	}
	return versions, nil
}

// GetVersion retrieves a single version by ID including its files.
func (s *Service) GetVersion(ctx context.Context, versionID string) (*db.BinPostVersion, error) {
	id, err := strconv.ParseInt(versionID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid version ID")
	}
	version, err := s.repo.GetBinPostVersion(ctx, id)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, errors.New("version not found")
		}
		return nil, err
	}
	return version, nil
}
