package dm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

// Service is the DM business-logic layer. Methods enforce membership rules,
// emit system messages, and fan out WS events. Handlers should call into
// Service rather than touching the repository directly for any state-mutating
// operation that has cross-cutting concerns.
type Service struct {
	repo *db.Repository
	hub  *ws.Hub
}

func NewService(repo *db.Repository, hub *ws.Hub) *Service {
	return &Service{repo: repo, hub: hub}
}

// CreateChannel creates a 1:1 DM if otherUserIDs has exactly one entry, or a
// group DM otherwise. For 1:1, reuses an existing channel (via the repo's
// GetOrCreateDmChannel). For group, the actor becomes creator + initial
// owner, a default themed name is picked unless customName is non-empty, a
// `group_created` system message is inserted, and DM_CHANNEL_CREATE is
// broadcast to every initial member's user-WS.
func (s *Service) CreateChannel(ctx context.Context, actorUserID int64, otherUserIDs []int64, customName string) (*db.DmChannel, error) {
	others := dedupeAndExcludeSelf(otherUserIDs, actorUserID)
	if len(others) == 0 {
		return nil, errors.New("must include at least one other user")
	}

	if len(others) == 1 {
		return s.repo.GetOrCreateDmChannel(ctx, actorUserID, others[0])
	}

	// Group path
	name := customName
	if name == "" {
		name = PickGroupName(len(others) + 1) // +1 for actor
	}

	members := append([]int64{actorUserID}, others...)
	ch, err := s.repo.CreateGroupChannel(ctx, actorUserID, name, members)
	if err != nil {
		return nil, err
	}

	eventJSON, _ := json.Marshal(map[string]any{
		"type":          "group_created",
		"actor_user_id": strconv.FormatInt(actorUserID, 10),
	})
	if _, err := s.repo.InsertSystemMessage(ctx, ch.ID, actorUserID, eventJSON); err != nil {
		return nil, fmt.Errorf("insert group_created event: %w", err)
	}

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]any{"channel": ch})
		for _, uid := range members {
			s.hub.SendToUser(strconv.FormatInt(uid, 10), ws.EventDmChannelCreate, payload)
		}
	}

	return ch, nil
}

// IsMember answers whether userID belongs to channelID. Migration #65 backfilled
// dm_channel_members for all existing 1:1 channels, so a single dm_channel_members
// lookup serves both 1:1 and group channels uniformly.
func (s *Service) IsMember(ctx context.Context, channelID, userID int64) (bool, error) {
	return s.repo.IsDmMember(ctx, channelID, userID)
}

func dedupeAndExcludeSelf(ids []int64, self int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id == self || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
