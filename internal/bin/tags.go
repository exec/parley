package bin

import (
	"context"
	"errors"
	"strconv"

	"parley/internal/db"
	"parley/internal/permissions"
)

// CreateTag creates a new tag for a bin channel.
// userID is used for ManageTags permission check.
func (s *Service) CreateTag(ctx context.Context, channelID, userID, name, color string) (*db.BinChannelTag, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}

	chID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	if userID != "" {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return nil, errors.New("invalid user ID")
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, chID)
		if err != nil {
			return nil, err
		}
		canManage, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, chID, permissions.PermManageTags)
		if err != nil {
			return nil, err
		}
		if !canManage {
			return nil, ErrForbidden
		}
	}

	tag, err := s.repo.CreateBinChannelTag(ctx, chID, name, color)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

// GetTags retrieves all tags for a bin channel.
func (s *Service) GetTags(ctx context.Context, channelID string) ([]db.BinChannelTag, error) {
	chID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
	}

	tags, err := s.repo.GetBinChannelTags(ctx, chID)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []db.BinChannelTag{}
	}
	return tags, nil
}

// DeleteTag deletes a bin channel tag by ID.
// userID is used for ManageTags permission check; pass "" to skip.
func (s *Service) DeleteTag(ctx context.Context, tagID, userID string) error {
	id, err := strconv.ParseInt(tagID, 10, 64)
	if err != nil {
		return errors.New("invalid tag ID")
	}

	if userID != "" {
		uID, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			return errors.New("invalid user ID")
		}
		tag, err := s.repo.GetBinChannelTagByID(ctx, id)
		if err != nil {
			if err == db.ErrNotFound {
				return ErrTagNotFound
			}
			return err
		}
		serverID, ownerID, err := s.getServerForChannel(ctx, tag.ChannelID)
		if err != nil {
			return err
		}
		canManage, err := permissions.HasChannelPermission(ctx, s.repo, serverID, uID, ownerID, tag.ChannelID, permissions.PermManageTags)
		if err != nil {
			return err
		}
		if !canManage {
			return ErrForbidden
		}
	}

	if err := s.repo.DeleteBinChannelTag(ctx, id); err != nil {
		if err == db.ErrNotFound {
			return ErrTagNotFound
		}
		return err
	}
	return nil
}
