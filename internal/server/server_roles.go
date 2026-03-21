package server

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"parley/internal/audit"
	ws "parley/internal/websocket"
)

func (s *ServerService) GetServerRoles(ctx context.Context, serverID string) ([]Role, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	dbRoles, err := s.repo.GetServerRoles(ctx, id)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		roles[i] = dbRoleToRole(r)
	}
	return roles, nil
}

func (s *ServerService) CreateServerRole(ctx context.Context, serverID, name, color string, permissions int64, actorID int64, actorUsername string) (*Role, error) {
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	if name == "" {
		return nil, errors.New("role name is required")
	}
	if color == "" {
		color = "#99aab5"
	}
	dbRole, err := s.repo.CreateServerRole(ctx, serverIDInt, name, color, permissions)
	if err != nil {
		return nil, err
	}
	r := dbRoleToRole(*dbRole)
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "role.create",
		TargetID:      r.ID,
		TargetType:    "role",
		TargetName:    name,
	})
	return &r, nil
}

func (s *ServerService) DeleteServerRole(ctx context.Context, serverID, roleID string, actorID int64, actorUsername string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	if err := s.repo.DeleteServerRole(ctx, sID, rID); err != nil {
		return err
	}
	// Clean up all permission overwrites that reference this role.
	// target_type 0 = role overwrite.
	if err := s.repo.DeleteOverwritesByTarget(ctx, 0, rID); err != nil {
		// Log but don't fail the deletion.
		_ = err
	}
	// Broadcast ROLE_DELETE to server subscribers.
	if s.hub != nil {
		payload, err := json.Marshal(map[string]string{
			"role_id":   roleID,
			"server_id": serverID,
		})
		if err == nil {
			s.hub.BroadcastToChannel("server:"+serverID, ws.EventRoleDelete, payload)
		}
	}
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      sID,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "role.delete",
		TargetID:      roleID,
		TargetType:    "role",
	})
	return nil
}

func (s *ServerService) UpdateServerRole(ctx context.Context, serverID, roleID, name, color string, permissions int64, hoist bool, position int, actorID int64, actorUsername string) (*Role, error) {
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	roleIDInt, err := idToInt64(roleID)
	if err != nil {
		return nil, errors.New("invalid role ID")
	}
	beforeRole, _ := s.repo.GetServerRoleByID(ctx, roleIDInt)
	dbRole, err := s.repo.UpdateServerRole(ctx, serverIDInt, roleIDInt, name, color, permissions, hoist, position)
	if err != nil {
		return nil, err
	}
	r := dbRoleToRole(*dbRole)
	// Broadcast ROLE_UPDATE to server subscribers.
	if s.hub != nil {
		payload, err := json.Marshal(r)
		if err == nil {
			s.hub.BroadcastToChannel("server:"+serverID, ws.EventRoleUpdate, payload)
		}
	}
	// Broadcast MEMBER_ROLE_UPDATE to each member who has this role so their
	// frontend re-fetches channel permissions with the updated role data.
	members, err := s.repo.GetMembersByRole(ctx, serverIDInt, roleIDInt)
	if err == nil {
		for _, m := range members {
			uIDStr := int64ToID(m.UserID)
			s.broadcastRoleUpdate(ctx, serverID, uIDStr, serverIDInt, m.UserID)
		}
	}
	if beforeRole != nil {
		s.auditSvc.Log(ctx, audit.Entry{
			ServerID:      serverIDInt,
			ActorID:       &actorID,
			ActorUsername: actorUsername,
			Action:        "role.update",
			TargetID:      strconv.FormatInt(roleIDInt, 10),
			TargetType:    "role",
			TargetName:    name,
			Changes: map[string]any{
				"before": map[string]any{
					"name":        beforeRole.Name,
					"color":       beforeRole.Color,
					"permissions": beforeRole.Permissions,
					"hoist":       beforeRole.Hoist,
					"position":    beforeRole.Position,
				},
				"after": map[string]any{
					"name":        name,
					"color":       color,
					"permissions": permissions,
					"hoist":       hoist,
					"position":    position,
				},
			},
		})
	}
	return &r, nil
}

func (s *ServerService) GetMemberRoles(ctx context.Context, serverID, userID string) ([]Role, error) {
	sID, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}
	dbRoles, err := s.repo.GetMemberRoles(ctx, sID, uID)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		roles[i] = dbRoleToRole(r)
	}
	return roles, nil
}

func (s *ServerService) AssignRoleToMember(ctx context.Context, serverID, userID, roleID string, actorID int64, actorUsername string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	if err := s.repo.AssignRoleToMember(ctx, sID, uID, rID); err != nil {
		return err
	}
	s.broadcastRoleUpdate(ctx, serverID, userID, sID, uID)
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      sID,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.role_add",
		TargetID:      strconv.FormatInt(uID, 10),
		TargetType:    "user",
		TargetName:    roleID,
	})
	return nil
}

func (s *ServerService) RemoveRoleFromMember(ctx context.Context, serverID, userID, roleID string, actorID int64, actorUsername string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	uID, err := idToInt64(userID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	if err := s.repo.RemoveRoleFromMember(ctx, sID, uID, rID); err != nil {
		return err
	}
	s.broadcastRoleUpdate(ctx, serverID, userID, sID, uID)
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      sID,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.role_remove",
		TargetID:      strconv.FormatInt(uID, 10),
		TargetType:    "user",
		TargetName:    roleID,
	})
	return nil
}

func (s *ServerService) broadcastRoleUpdate(ctx context.Context, serverID, userID string, sID, uID int64) {
	if s.hub == nil {
		return
	}
	dbRoles, err := s.repo.GetMemberRoles(ctx, sID, uID)
	if err != nil {
		return
	}
	roles := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		roles[i] = dbRoleToRole(r)
	}
	payload, err := json.Marshal(map[string]interface{}{
		"server_id": serverID,
		"user_id":   userID,
		"roles":     roles,
	})
	if err != nil {
		return
	}
	s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberRoleUpdate, payload)
}
