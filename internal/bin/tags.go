package bin

import (
	"context"
	"errors"
	"strconv"

	"parley/internal/db"
)

// CreateTag creates a new tag for a bin channel.
func (s *Service) CreateTag(ctx context.Context, channelID, name, color string) (*db.BinChannelTag, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}

	chID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid channel ID")
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
func (s *Service) DeleteTag(ctx context.Context, tagID string) error {
	id, err := strconv.ParseInt(tagID, 10, 64)
	if err != nil {
		return errors.New("invalid tag ID")
	}

	if err := s.repo.DeleteBinChannelTag(ctx, id); err != nil {
		if err == db.ErrNotFound {
			return errors.New("tag not found")
		}
		return err
	}
	return nil
}
