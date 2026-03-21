package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"

	"parley/internal/audit"
	"parley/internal/db"
	"parley/internal/permissions"
	ws "parley/internal/websocket"
)

func (s *ServerService) AddMember(ctx context.Context, serverID, userID, nickname string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	if userID == "" {
		return errors.New("user ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID format")
	}
	userIDInt, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID format")
	}

	member := &db.ServerMember{
		ServerID: serverIDInt,
		UserID:   userIDInt,
		Nickname: nickname,
	}
	if err = s.repo.AddMember(ctx, member); err != nil {
		return err
	}

	s.broadcastMemberJoin(serverID, userID)
	return nil
}

func (s *ServerService) broadcastMemberJoin(serverID, userID string) {
	if s.hub == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
	if err != nil {
		log.Printf("Failed to marshal member join event: %v", err)
		return
	}
	s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberJoin, payload)
}

func (s *ServerService) RemoveMember(ctx context.Context, serverID, userID string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	if userID == "" {
		return errors.New("user ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID format")
	}
	userIDInt, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID format")
	}

	if err = s.repo.RemoveMember(ctx, serverIDInt, userIDInt); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errors.New("member not found")
		}
		return err
	}

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberLeave, payload)
	}
	return nil
}

func (s *ServerService) KickMember(ctx context.Context, serverID, userID string, actorID int64, actorUsername string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	if userID == "" {
		return errors.New("user ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID format")
	}
	userIDInt, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID format")
	}

	if err = s.repo.RemoveMember(ctx, serverIDInt, userIDInt); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errors.New("member not found")
		}
		return err
	}

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberKick, payload)
		s.hub.SendToUser(userID, ws.EventMemberKick, payload)
	}

	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.kick",
		TargetID:      strconv.FormatInt(userIDInt, 10),
		TargetType:    "user",
	})
	return nil
}

func (s *ServerService) BanMember(ctx context.Context, serverID, userID string, actorID int64, actorUsername, reason string) error {
	if serverID == "" {
		return errors.New("server ID is required")
	}
	if userID == "" {
		return errors.New("user ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID format")
	}
	userIDInt, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID format")
	}

	if err := s.repo.AddServerBan(ctx, serverIDInt, userIDInt, actorID, reason); err != nil {
		return err
	}
	_ = s.repo.RemoveMember(ctx, serverIDInt, userIDInt)

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
		s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberBan, payload)
		s.hub.SendToUser(userID, ws.EventMemberBan, payload)
	}

	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.ban",
		TargetID:      strconv.FormatInt(userIDInt, 10),
		TargetType:    "user",
		Reason:        reason,
	})
	return nil
}

func (s *ServerService) ListBans(ctx context.Context, serverID string) ([]db.ServerBan, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	return s.repo.ListServerBans(ctx, id)
}

func (s *ServerService) UnbanMember(ctx context.Context, serverID, userID string, actorID int64, actorUsername string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	if err := s.repo.RemoveServerBan(ctx, sID, uID); err != nil {
		return err
	}
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      sID,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.unban",
		TargetID:      strconv.FormatInt(uID, 10),
		TargetType:    "user",
	})
	return nil
}

func (s *ServerService) GetMembers(ctx context.Context, serverID string) ([]*ServerMember, error) {
	if serverID == "" {
		return nil, errors.New("server ID is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	members, err := s.repo.GetServerMembers(ctx, serverIDInt)
	if err != nil {
		return nil, err
	}

	result := make([]*ServerMember, len(members))
	for i, member := range members {
		result[i] = &ServerMember{
			ID:          int64ToID(member.ID),
			ServerID:    int64ToID(member.ServerID),
			UserID:      int64ToID(member.UserID),
			Username:    member.Username,
			DisplayName: member.DisplayName,
			Nickname:    member.Nickname,
			AvatarURL:   member.AvatarURL,
			BannerURL:   member.BannerURL,
			Bio:         member.Bio,
			Badges:      member.Badges,
			JoinedAt:    member.JoinedAt,
			Roles:       []Role{},
			IsBot:       member.IsBot,
			BotDegraded: member.BotDegraded,
		}
	}
	return result, nil
}

func (s *ServerService) GetMembersWithRoles(ctx context.Context, serverID string) ([]*ServerMember, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	dbMembers, err := s.repo.GetServerMembersWithRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	members := make([]*ServerMember, len(dbMembers))
	for i, m := range dbMembers {
		roles := make([]Role, len(m.Roles))
		for j, r := range m.Roles {
			roles[j] = dbRoleToRole(r)
		}
		members[i] = &ServerMember{
			ID:          strconv.FormatInt(m.ID, 10),
			ServerID:    strconv.FormatInt(m.ServerID, 10),
			UserID:      strconv.FormatInt(m.UserID, 10),
			Username:    m.Username,
			DisplayName: m.DisplayName,
			Nickname:    m.Nickname,
			AvatarURL:   m.AvatarURL,
			BannerURL:   m.BannerURL,
			Bio:         m.Bio,
			Badges:      m.Badges,
			JoinedAt:    m.JoinedAt,
			Roles:       roles,
			IsBot:       m.IsBot,
			BotDegraded: m.BotDegraded,
		}
	}
	return members, nil
}

// CanKickBan returns true if actorID has permission to kick/ban members in the server.
// Kept for backward compatibility; use CanKick/CanBan for separate checks.
func (s *ServerService) CanKickBan(ctx context.Context, serverID, actorID string) (isOwner bool, allowed bool, err error) {
	return s.CanKick(ctx, serverID, actorID)
}

// CanKick returns true if actorID has permission to kick members in the server.
func (s *ServerService) CanKick(ctx context.Context, serverID, actorID string) (isOwner bool, allowed bool, err error) {
	srv, err := s.GetServer(ctx, serverID)
	if err != nil {
		return false, false, err
	}
	if srv.OwnerID == actorID {
		return true, true, nil
	}
	sID, _ := idToInt64(serverID)
	aID, err := idToInt64(actorID)
	if err != nil {
		return false, false, errors.New("invalid actor ID")
	}
	ownerID, _ := idToInt64(srv.OwnerID)
	hasPerm, err := permissions.HasPermission(ctx, s.repo, sID, aID, ownerID, permissions.PermKickMembers)
	if err != nil {
		return false, false, err
	}
	return false, hasPerm, nil
}

// CanBan returns true if actorID has permission to ban members in the server.
func (s *ServerService) CanBan(ctx context.Context, serverID, actorID string) (isOwner bool, allowed bool, err error) {
	srv, err := s.GetServer(ctx, serverID)
	if err != nil {
		return false, false, err
	}
	if srv.OwnerID == actorID {
		return true, true, nil
	}
	sID, _ := idToInt64(serverID)
	aID, err := idToInt64(actorID)
	if err != nil {
		return false, false, errors.New("invalid actor ID")
	}
	ownerID, _ := idToInt64(srv.OwnerID)
	hasPerm, err := permissions.HasPermission(ctx, s.repo, sID, aID, ownerID, permissions.PermBanMembers)
	if err != nil {
		return false, false, err
	}
	return false, hasPerm, nil
}

// RoleHierarchyCheck returns true if actorID's highest role is above targetID's highest role.
// Owner always passes. Returns (isOwner, passes, error).
func (s *ServerService) RoleHierarchyCheck(ctx context.Context, serverID, actorID, targetID string) (bool, bool, error) {
	return s.roleHierarchyCheck(ctx, serverID, actorID, targetID)
}

// roleHierarchyCheck is the internal implementation.
func (s *ServerService) roleHierarchyCheck(ctx context.Context, serverID, actorID, targetID string) (bool, bool, error) {
	srv, err := s.GetServer(ctx, serverID)
	if err != nil {
		return false, false, err
	}
	if srv.OwnerID == actorID {
		return true, true, nil
	}
	sID, _ := idToInt64(serverID)
	aID, _ := idToInt64(actorID)
	tID, _ := idToInt64(targetID)
	actorHighest, err := s.repo.GetHighestRolePosition(ctx, sID, aID)
	if err != nil {
		return false, false, err
	}
	targetHighest, err := s.repo.GetHighestRolePosition(ctx, sID, tID)
	if err != nil {
		return false, false, err
	}
	// Actor's position must be strictly greater than target's.
	return false, actorHighest > targetHighest, nil
}

// GetMyPermissions returns the effective permission bitfield and owner status for a user in a server.
func (s *ServerService) GetMyPermissions(ctx context.Context, serverID, userID string) (int64, bool, error) {
	srv, err := s.GetServer(ctx, serverID)
	if err != nil {
		return 0, false, err
	}
	isOwner := srv.OwnerID == userID
	if isOwner {
		return ^int64(0), true, nil
	}
	sID, _ := idToInt64(serverID)
	uID, err := idToInt64(userID)
	if err != nil {
		return 0, false, errors.New("invalid user ID")
	}
	ownerID, _ := idToInt64(srv.OwnerID)
	perms, err := permissions.GetEffectivePermissions(ctx, s.repo, sID, uID, ownerID)
	if err != nil {
		return 0, false, err
	}
	return perms, false, nil
}
