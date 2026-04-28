package dm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"parley/internal/db"
	voice "parley/internal/voice"
	ws "parley/internal/websocket"
)

// Sentinel errors returned by handlers / injected callbacks. Kept in the dm
// package so handler 4xx mapping doesn't need to import packages that would
// pull a dependency cycle (e.g. internal/message).
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrNotFriend = errors.New("not_friend")
	ErrBlocked   = errors.New("blocked")
)

// FriendChecker is the slice of friend.Service this package needs. Wired via
// Service.SetFriendChecker from cmd/api/routes.go to avoid an import cycle
// (internal/friend already imports internal/db; internal/dm already imports
// internal/db; both need to coordinate without importing each other).
type FriendChecker interface {
	IsFriend(ctx context.Context, a, b int64) (bool, error)
	IsEitherBlocked(ctx context.Context, a, b int64) (bool, error)
}

// dmRepo is the repository surface the Service uses. Defined as an interface
// so tests can inject an in-memory fake without needing a real database.
type dmRepo interface {
	IsDmMember(ctx context.Context, channelID, userID int64) (bool, error)
	InsertSystemMessage(ctx context.Context, channelID, actorUserID int64, eventJSON []byte) (*db.DmMessage, error)
	GetDmChannelByID(ctx context.Context, id int64) (*db.DmChannel, error)
	GetDmMembers(ctx context.Context, channelID int64) ([]db.DmChannelMember, error)
	GetOrCreateDmChannel(ctx context.Context, user1ID, user2ID int64) (*db.DmChannel, error)
	CreateGroupChannel(ctx context.Context, ownerID int64, name string, memberIDs []int64) (*db.DmChannel, error)
	AddDmMember(ctx context.Context, channelID, userID int64) error
	RemoveDmMember(ctx context.Context, channelID, userID int64) error
	TransferDmGroupOwnership(ctx context.Context, channelID int64, newOwnerID *int64) error
	UpdateDmGroupName(ctx context.Context, channelID int64, name string) error
	UpdateDmGroupAvatar(ctx context.Context, channelID int64, avatarURL *string) error
	GetUserDisplayName(ctx context.Context, userID int64) (string, error)
}

// dmHub is the websocket hub surface the Service uses.
type dmHub interface {
	BroadcastToChannel(channelID string, messageType string, payload []byte)
	SendToUser(userID string, messageType string, payload []byte) error
	SendBytesToUsers(userIDs []int64, messageType string, payload []byte)
}

// Service is the DM business-logic layer. Methods enforce membership rules,
// emit system messages, and fan out WS events. Handlers should call into
// Service rather than touching the repository directly for any state-mutating
// operation that has cross-cutting concerns.
type Service struct {
	repo    dmRepo
	hub     dmHub
	friends FriendChecker // optional; nil-safe — gating skipped when unset (tests)
}

func NewService(repo *db.Repository, hub *ws.Hub) *Service {
	return &Service{repo: repo, hub: hub}
}

// SetFriendChecker wires the friend service so DM gating can consult friendship
// and block state. Called from cmd/api wiring after both services are built.
func (s *Service) SetFriendChecker(fc FriendChecker) { s.friends = fc }

// gateAddTarget rejects the operation if actor and target are blocked in
// either direction, or — when requireFriend — are not friends. Nil-safe:
// returns nil immediately when no FriendChecker is wired (test path).
func (s *Service) gateAddTarget(ctx context.Context, actor, target int64, requireFriend bool) error {
	if s.friends == nil {
		return nil
	}
	blocked, err := s.friends.IsEitherBlocked(ctx, actor, target)
	if err != nil {
		return err
	}
	if blocked {
		return ErrBlocked
	}
	if !requireFriend {
		return nil
	}
	friend, err := s.friends.IsFriend(ctx, actor, target)
	if err != nil {
		return err
	}
	if !friend {
		return ErrNotFriend
	}
	return nil
}

// resolveDisplayName fetches the user's display name (falling back to username)
// at event-emit time. We snapshot the name into the system_event payload so
// rendering survives membership changes — kicked users disappear from the
// member list, but their kick message still says "alice removed bob".
func (s *Service) resolveDisplayName(ctx context.Context, userID int64) string {
	name, err := s.repo.GetUserDisplayName(ctx, userID)
	if err != nil || name == "" {
		return "someone"
	}
	return name
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
		// 1:1 DMs are a discovery surface — no friend requirement, but
		// blocks in either direction kill the request.
		if err := s.gateAddTarget(ctx, actorUserID, others[0], false); err != nil {
			return nil, err
		}
		return s.repo.GetOrCreateDmChannel(ctx, actorUserID, others[0])
	}

	// Group path: every invitee must be a friend of the actor (and no
	// blocks in either direction). GCs force PII (avatar/status/presence)
	// onto every member; friends-only is the cheapest defensible boundary.
	for _, uid := range others {
		if err := s.gateAddTarget(ctx, actorUserID, uid, true); err != nil {
			return nil, err
		}
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
		"type":               "group_created",
		"actor_user_id":      strconv.FormatInt(actorUserID, 10),
		"actor_display_name": s.resolveDisplayName(ctx, actorUserID),
	})
	if _, err := s.repo.InsertSystemMessage(ctx, ch.ID, actorUserID, eventJSON); err != nil {
		return nil, fmt.Errorf("insert group_created event: %w", err)
	}

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]any{"channel": ch})
		s.hub.SendBytesToUsers(members, ws.EventDmChannelCreate, payload)
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
		// Friends-only invite (mirrors the GC-create rule). Blocks in
		// either direction also reject. Done per-target so the partial
		// add of friends still succeeds when one invitee is a non-friend
		// would short-circuit — current call sites pass single users at a
		// time from the UI, so this is fine.
		if err := s.gateAddTarget(ctx, actorUserID, uid, true); err != nil {
			return err
		}
		if err := s.repo.AddDmMember(ctx, channelID, uid); err != nil {
			return err
		}
		eventJSON, _ := json.Marshal(map[string]any{
			"type":                "member_added",
			"actor_user_id":       strconv.FormatInt(actorUserID, 10),
			"actor_display_name":  s.resolveDisplayName(ctx, actorUserID),
			"target_user_id":      strconv.FormatInt(uid, 10),
			"target_display_name": s.resolveDisplayName(ctx, uid),
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
//   - transferTo nil with other members present: rejected — owners must pick
//     a successor before leaving.
//   - transferTo nil with no other members: allowed; the channel is left
//     ownerless (the actor was the last member).
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
			"type":                   "owner_transferred",
			"actor_user_id":          strconv.FormatInt(actorUserID, 10),
			"actor_display_name":     s.resolveDisplayName(ctx, actorUserID),
			"new_owner_user_id":      strconv.FormatInt(*transferTo, 10),
			"new_owner_display_name": s.resolveDisplayName(ctx, *transferTo),
		})
		if sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON); err == nil && s.hub != nil {
			sysMsgJSON, _ := json.Marshal(sysMsg)
			s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)
		}
	} else if isOwner {
		// Owner leaving without a transferTo. Allowed only if there are no
		// other members to transfer to (i.e. the owner is the last person);
		// otherwise reject so a group can never end up unkickable while
		// people are still active in it.
		members, _ := s.repo.GetDmMembers(ctx, channelID)
		hasOthers := false
		for _, m := range members {
			if m.UserID != actorUserID {
				hasOthers = true
				break
			}
		}
		if hasOthers {
			return errors.New("owner must transfer ownership before leaving")
		}
		// Last-member-leaves path: clear owner_user_id so the (about-to-be-empty)
		// channel doesn't carry a stale FK pointing at someone who isn't a member.
		if err := s.repo.TransferDmGroupOwnership(ctx, channelID, nil); err != nil {
			return err
		}
	}

	if err := s.repo.RemoveDmMember(ctx, channelID, actorUserID); err != nil {
		return err
	}

	eventJSON, _ := json.Marshal(map[string]any{
		"type":               "member_left",
		"actor_user_id":      strconv.FormatInt(actorUserID, 10),
		"actor_display_name": s.resolveDisplayName(ctx, actorUserID),
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
		"type":                "member_kicked",
		"actor_user_id":       strconv.FormatInt(actorUserID, 10),
		"actor_display_name":  s.resolveDisplayName(ctx, actorUserID),
		"target_user_id":      strconv.FormatInt(targetUserID, 10),
		"target_display_name": s.resolveDisplayName(ctx, targetUserID),
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

// UpdateGroupName: any member may rename. Emits group_name_changed system msg.
func (s *Service) UpdateGroupName(ctx context.Context, channelID, actorUserID int64, newName string) error {
	if newName == "" {
		return errors.New("name cannot be empty")
	}
	if len(newName) > 100 {
		return errors.New("name too long (max 100 chars)")
	}
	ch, err := s.repo.GetDmChannelByID(ctx, channelID)
	if err != nil {
		return err
	}
	if !ch.IsGroup {
		return errors.New("not a group channel")
	}

	isMember, _ := s.repo.IsDmMember(ctx, channelID, actorUserID)
	if !isMember {
		return errors.New("not a member")
	}

	if err := s.repo.UpdateDmGroupName(ctx, channelID, newName); err != nil {
		return err
	}

	var oldName string
	if ch.Name != nil {
		oldName = *ch.Name
	}
	eventJSON, _ := json.Marshal(map[string]any{
		"type":               "group_name_changed",
		"actor_user_id":      strconv.FormatInt(actorUserID, 10),
		"actor_display_name": s.resolveDisplayName(ctx, actorUserID),
		"old_name":           oldName,
		"new_name":           newName,
	})
	if sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON); err == nil && s.hub != nil {
		sysMsgJSON, _ := json.Marshal(sysMsg)
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)
	}
	if s.hub != nil {
		updatePayload, _ := json.Marshal(map[string]any{
			"channel_id": strconv.FormatInt(channelID, 10),
			"name":       newName,
		})
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmChannelUpdate, updatePayload)
	}
	return nil
}

// UpdateGroupAvatar: any member may set/clear. Pass nil avatarURL to clear.
// No system message — too noisy; just DM_CHANNEL_UPDATE.
func (s *Service) UpdateGroupAvatar(ctx context.Context, channelID, actorUserID int64, avatarURL *string) error {
	ch, err := s.repo.GetDmChannelByID(ctx, channelID)
	if err != nil {
		return err
	}
	if !ch.IsGroup {
		return errors.New("not a group channel")
	}

	isMember, _ := s.repo.IsDmMember(ctx, channelID, actorUserID)
	if !isMember {
		return errors.New("not a member")
	}

	if err := s.repo.UpdateDmGroupAvatar(ctx, channelID, avatarURL); err != nil {
		return err
	}

	if s.hub != nil {
		var url string
		if avatarURL != nil {
			url = *avatarURL
		}
		updatePayload, _ := json.Marshal(map[string]any{
			"channel_id": strconv.FormatInt(channelID, 10),
			"avatar_url": url,
		})
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmChannelUpdate, updatePayload)
	}
	return nil
}

// TransferOwnership: owner-only standalone transfer (without leaving). New
// owner must be a member.
func (s *Service) TransferOwnership(ctx context.Context, channelID, actorUserID, newOwnerID int64) error {
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
	if newOwnerID == actorUserID {
		return errors.New("you are already the owner")
	}
	targetIsMember, _ := s.repo.IsDmMember(ctx, channelID, newOwnerID)
	if !targetIsMember {
		return errors.New("target is not a member")
	}

	if err := s.repo.TransferDmGroupOwnership(ctx, channelID, &newOwnerID); err != nil {
		return err
	}
	eventJSON, _ := json.Marshal(map[string]any{
		"type":                   "owner_transferred",
		"actor_user_id":          strconv.FormatInt(actorUserID, 10),
		"actor_display_name":     s.resolveDisplayName(ctx, actorUserID),
		"new_owner_user_id":      strconv.FormatInt(newOwnerID, 10),
		"new_owner_display_name": s.resolveDisplayName(ctx, newOwnerID),
	})
	if sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON); err == nil && s.hub != nil {
		sysMsgJSON, _ := json.Marshal(sysMsg)
		s.hub.BroadcastToChannel(fmt.Sprintf("dm:%d", channelID), ws.EventDmMessageCreate, sysMsgJSON)
	}
	return nil
}

// EmitCallStarted writes a `call_started` system_event into the DM channel and
// broadcasts the resulting message on the dm:{id} virtual channel.
func (s *Service) EmitCallStarted(ctx context.Context, channelID, actorUserID, startedAtMs int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":               "call_started",
		"actor_user_id":      strconv.FormatInt(actorUserID, 10),
		"actor_display_name": s.resolveDisplayName(ctx, actorUserID),
		"started_at_ms":      startedAtMs,
	})
	return s.broadcastSystemMessage(ctx, channelID, actorUserID, eventJSON)
}

// EmitCallEnded writes `call_ended` with the call's duration. lastLeaverUserID
// is the user whose Leave triggered empty-room detection; used as the row's author_id.
func (s *Service) EmitCallEnded(ctx context.Context, channelID, lastLeaverUserID, durationMs, startedAtMs int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":          "call_ended",
		"duration_ms":   durationMs,
		"started_at_ms": startedAtMs,
	})
	return s.broadcastSystemMessage(ctx, channelID, lastLeaverUserID, eventJSON)
}

// EmitCallMissed writes `call_missed` for ring timeout or caller-cancel.
func (s *Service) EmitCallMissed(ctx context.Context, channelID, callerUserID int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":                "call_missed",
		"caller_user_id":      strconv.FormatInt(callerUserID, 10),
		"caller_display_name": s.resolveDisplayName(ctx, callerUserID),
	})
	return s.broadcastSystemMessage(ctx, channelID, callerUserID, eventJSON)
}

// EmitCallDeclined writes `call_declined`.
func (s *Service) EmitCallDeclined(ctx context.Context, channelID, callerUserID, declinerUserID int64) error {
	eventJSON, _ := json.Marshal(map[string]any{
		"type":                  "call_declined",
		"caller_user_id":        strconv.FormatInt(callerUserID, 10),
		"caller_display_name":   s.resolveDisplayName(ctx, callerUserID),
		"decliner_user_id":      strconv.FormatInt(declinerUserID, 10),
		"decliner_display_name": s.resolveDisplayName(ctx, declinerUserID),
	})
	return s.broadcastSystemMessage(ctx, channelID, callerUserID, eventJSON)
}

// broadcastSystemMessage is the shared persist-and-broadcast routine used by
// the call emit helpers.
func (s *Service) broadcastSystemMessage(ctx context.Context, channelID, actorUserID int64, eventJSON []byte) error {
	sysMsg, err := s.repo.InsertSystemMessage(ctx, channelID, actorUserID, eventJSON)
	if err != nil {
		return err
	}
	if s.hub != nil && sysMsg != nil {
		virtualChannel := voice.VirtualChannel{Kind: voice.KindDM, ID: channelID}.String()
		sysMsgJSON, _ := json.Marshal(sysMsg)
		s.hub.BroadcastToChannel(virtualChannel, ws.EventDmMessageCreate, sysMsgJSON)
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
