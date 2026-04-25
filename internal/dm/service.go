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

// AddMembers appends users to a group DM. Any existing member can add. The
// 100-member cap is enforced. Idempotent: re-adding existing members is a
// no-op (no system message emitted for those). Per added user a member_added
// system message is broadcast on the dm:{id} virtual channel.
func (s *Service) AddMembers(ctx context.Context, channelID, actorUserID int64, newMemberIDs []int64) error {
	ch, err := s.repo.GetDmChannelByID(ctx, channelID)
	if err != nil {
		return err
	}
	if !ch.IsGroup {
		return errors.New("not a group channel")
	}

	isMember, err := s.repo.IsDmMember(ctx, channelID, actorUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return errors.New("not a member of this channel")
	}

	existing, _ := s.repo.GetDmMembers(ctx, channelID)
	if len(existing)+len(newMemberIDs) > 100 {
		return errors.New("group at capacity (max 100)")
	}

	for _, uid := range newMemberIDs {
		// Skip if already a member (idempotent)
		if already, _ := s.repo.IsDmMember(ctx, channelID, uid); already {
			continue
		}
		if err := s.repo.AddDmMember(ctx, channelID, uid); err != nil {
			return err
		}
		eventJSON, _ := json.Marshal(map[string]any{
			"type":           "member_added",
			"actor_user_id":  strconv.FormatInt(actorUserID, 10),
			"target_user_id": strconv.FormatInt(uid, 10),
		})
		sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
		if err != nil {
			return err
		}
		if s.hub != nil {
			virtualChannel := fmt.Sprintf("dm:%d", channelID)
			sysMsgJSON, _ := json.Marshal(sysMsg)
			s.hub.BroadcastToChannel(virtualChannel, ws.EventDmMessageCreate, sysMsgJSON)
			memberAddPayload, _ := json.Marshal(map[string]any{
				"channel_id": strconv.FormatInt(channelID, 10),
				"user_id":    strconv.FormatInt(uid, 10),
				"added_by":   strconv.FormatInt(actorUserID, 10),
			})
			s.hub.BroadcastToChannel(virtualChannel, ws.EventDmMemberAdd, memberAddPayload)
			// The new member isn't subscribed to dm:{id} yet — send DM_CHANNEL_CREATE on their user-WS so their DM list updates.
			channelPayload, _ := json.Marshal(map[string]any{"channel": ch})
			s.hub.SendToUser(strconv.FormatInt(uid, 10), ws.EventDmChannelCreate, channelPayload)
		}
	}
	return nil
}

// LeaveGroup removes the actor from a group DM. If the actor is the owner:
//   - transferTo non-nil: ownership moves to that user (must be a member),
//     then the actor leaves. Emits owner_transferred + member_left system
//     messages.
//   - transferTo nil: owner_user_id is cleared (NULL). "Power evaporates" —
//     no one can kick after this. Emits member_left only.
//
// Non-owner leave: just removes member, emits member_left.
func (s *Service) LeaveGroup(ctx context.Context, channelID, actorUserID int64, transferTo *int64) error {
	ch, err := s.repo.GetDmChannelByID(ctx, channelID)
	if err != nil {
		return err
	}
	if !ch.IsGroup {
		return errors.New("not a group channel")
	}

	isMember, err := s.repo.IsDmMember(ctx, channelID, actorUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return errors.New("not a member")
	}

	isOwner := ch.OwnerUserID != nil && *ch.OwnerUserID == actorUserID

	if transferTo != nil {
		if !isOwner {
			return errors.New("only owner can transfer ownership")
		}
		if *transferTo == actorUserID {
			return errors.New("cannot transfer to yourself")
		}
		targetIsMember, _ := s.repo.IsDmMember(ctx, channelID, *transferTo)
		if !targetIsMember {
			return errors.New("transfer target is not a member")
		}
		if err := s.repo.TransferDmGroupOwnership(ctx, channelID, transferTo); err != nil {
			return err
		}
		eventJSON, _ := json.Marshal(map[string]any{
			"type":              "owner_transferred",
			"actor_user_id":     strconv.FormatInt(actorUserID, 10),
			"new_owner_user_id": strconv.FormatInt(*transferTo, 10),
		})
		if sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON); err == nil && s.hub != nil {
			sysMsgJSON, _ := json.Marshal(sysMsg)
			s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)
		}
	} else if isOwner {
		// Power evaporates: clear owner.
		if err := s.repo.TransferDmGroupOwnership(ctx, channelID, nil); err != nil {
			return err
		}
	}

	if err := s.repo.RemoveDmMember(ctx, channelID, actorUserID); err != nil {
		return err
	}

	eventJSON, _ := json.Marshal(map[string]any{
		"type":          "member_left",
		"actor_user_id": strconv.FormatInt(actorUserID, 10),
	})
	if sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON); err == nil && s.hub != nil {
		sysMsgJSON, _ := json.Marshal(sysMsg)
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)
	}
	if s.hub != nil {
		memberRemovePayload, _ := json.Marshal(map[string]any{
			"channel_id": strconv.FormatInt(channelID, 10),
			"user_id":    strconv.FormatInt(actorUserID, 10),
		})
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMemberRemove, memberRemovePayload)
	}
	return nil
}

// KickMember removes a target user from a group DM. Owner-only. The owner
// cannot kick themselves (they should use LeaveGroup instead).
func (s *Service) KickMember(ctx context.Context, channelID, actorUserID, targetUserID int64) error {
	ch, err := s.repo.GetDmChannelByID(ctx, channelID)
	if err != nil {
		return err
	}
	if !ch.IsGroup {
		return errors.New("not a group channel")
	}
	if ch.OwnerUserID == nil || *ch.OwnerUserID != actorUserID {
		return errors.New("not the owner of this group")
	}
	if actorUserID == targetUserID {
		return errors.New("cannot kick yourself; use leave instead")
	}
	isMember, _ := s.repo.IsDmMember(ctx, channelID, targetUserID)
	if !isMember {
		return errors.New("target is not a member")
	}

	if err := s.repo.RemoveDmMember(ctx, channelID, targetUserID); err != nil {
		return err
	}
	eventJSON, _ := json.Marshal(map[string]any{
		"type":           "member_kicked",
		"actor_user_id":  strconv.FormatInt(actorUserID, 10),
		"target_user_id": strconv.FormatInt(targetUserID, 10),
	})
	if sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON); err == nil && s.hub != nil {
		sysMsgJSON, _ := json.Marshal(sysMsg)
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)
	}
	if s.hub != nil {
		memberRemovePayload, _ := json.Marshal(map[string]any{
			"channel_id": strconv.FormatInt(channelID, 10),
			"user_id":    strconv.FormatInt(targetUserID, 10),
			"kicked_by":  strconv.FormatInt(actorUserID, 10),
		})
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMemberRemove, memberRemovePayload)
	}
	return nil
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
