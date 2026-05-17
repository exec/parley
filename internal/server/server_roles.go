package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"

	"golang.org/x/sync/errgroup"

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

func (s *ServerService) DeleteServerRole(ctx context.Context, serverID, roleID string, actorID int64, actorUsername, reason string) error {
	sID, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return errors.New("invalid role ID")
	}
	// Fetch members with this role BEFORE deleting, so we can invalidate their
	// cached permissions after the role is removed.
	affectedMembers, err := s.repo.GetMembersByRole(ctx, sID, rID)
	if err != nil {
		log.Printf("warning: failed to fetch members for cache invalidation on role %d: %v", rID, err)
	}

	if err := s.repo.DeleteServerRole(ctx, sID, rID); err != nil {
		return err
	}
	// Invalidate cached permission entries for all members who had this role.
	if s.memberCache != nil {
		for _, m := range affectedMembers {
			s.memberCache.InvalidatePermsForUser(sID, m.UserID)
		}
	}
	// Clean up all permission overwrites that reference this role.
	// target_type 0 = role overwrite.
	if err := s.repo.DeleteOverwritesByTarget(ctx, 0, rID); err != nil {
		// Log but don't fail the deletion.
		log.Printf("Failed to delete permission overwrites for role %d: %v", rID, err)
	}
	// Broadcast ROLE_DELETE to server subscribers.
	if s.hub != nil {
		payload, err := json.Marshal(map[string]string{
			"role_id":   roleID,
			"server_id": serverID,
		})
		if err != nil {
			log.Printf("Failed to marshal role delete event: %v", err)
		} else {
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
		Reason:        reason,
	})
	return nil
}

// ReorderServerRoles atomically normalizes role positions for a server.
// orderedNonEveryoneIDs lists the non-@everyone roles from top (highest
// hierarchy) to bottom. Returns the refreshed role list and fans out
// role-update websocket events for every role in the server.
func (s *ServerService) ReorderServerRoles(ctx context.Context, serverID string, orderedNonEveryoneIDs []int64) ([]Role, error) {
	sID, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	if err := s.repo.ReorderServerRoles(ctx, sID, orderedNonEveryoneIDs); err != nil {
		return nil, err
	}
	dbRoles, err := s.repo.GetServerRoles(ctx, sID)
	if err != nil {
		return nil, err
	}
	out := make([]Role, len(dbRoles))
	for i, r := range dbRoles {
		out[i] = dbRoleToRole(r)
	}
	if s.hub != nil {
		for _, r := range out {
			payload, err := json.Marshal(r)
			if err != nil {
				log.Printf("Failed to marshal role update event: %v", err)
				continue
			}
			s.hub.BroadcastToChannel("server:"+serverID, ws.EventRoleUpdate, payload)
		}
	}
	return out, nil
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
	beforeRole, err := s.repo.GetServerRoleByID(ctx, roleIDInt)
	if err != nil {
		log.Printf("Failed to fetch role %d before update for audit log: %v", roleIDInt, err)
	}
	dbRole, err := s.repo.UpdateServerRole(ctx, serverIDInt, roleIDInt, name, color, permissions, hoist, position)
	if err != nil {
		return nil, err
	}
	r := dbRoleToRole(*dbRole)
	// Broadcast ROLE_UPDATE to server subscribers.
	if s.hub != nil {
		payload, err := json.Marshal(r)
		if err != nil {
			log.Printf("Failed to marshal role update event: %v", err)
		} else {
			s.hub.BroadcastToChannel("server:"+serverID, ws.EventRoleUpdate, payload)
		}
	}
	// Broadcast MEMBER_ROLE_UPDATE to each member who has this role so their
	// frontend re-fetches channel permissions with the updated role data.
	// Also invalidate cached permission entries so that subsequent server-side
	// checks reflect the updated role permissions immediately.
	members, err := s.repo.GetMembersByRole(ctx, serverIDInt, roleIDInt)
	if err != nil {
		log.Printf("Failed to get members for role %d during update broadcast: %v", roleIDInt, err)
	} else {
		// Broadcast each member's role update concurrently across cores.
		// Each call does an independent DB query + marshal + hub broadcast;
		// running them serially is an N+1 of DB round-trips that blocks
		// the request. errgroup bounds concurrency to 8.
		var g errgroup.Group
		g.SetLimit(8)
		for _, m := range members {
			m := m
			g.Go(func() error {
				uIDStr := int64ToID(m.UserID)
				s.broadcastRoleUpdate(ctx, serverID, uIDStr, serverIDInt, m.UserID)
				if s.memberCache != nil {
					s.memberCache.InvalidatePermsForUser(serverIDInt, m.UserID)
				}
				return nil
			})
		}
		// broadcastRoleUpdate logs its own errors and returns nothing, so
		// g.Wait never yields a non-nil error; we still wait so all
		// broadcasts finish before the audit-log step.
		_ = g.Wait()
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
	if s.memberCache != nil {
		s.memberCache.InvalidatePermsForUser(sID, uID)
	}
	s.broadcastRoleUpdate(ctx, serverID, userID, sID, uID)
	role, err := s.repo.GetServerRoleByID(ctx, rID)
	if err != nil {
		log.Printf("Failed to fetch role %d name for audit log: %v", rID, err)
	}
	roleName := roleID
	if role != nil {
		roleName = role.Name
	}
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      sID,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.role_add",
		TargetID:      strconv.FormatInt(uID, 10),
		TargetType:    "user",
		TargetName:    roleName,
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
	if s.memberCache != nil {
		s.memberCache.InvalidatePermsForUser(sID, uID)
	}
	s.broadcastRoleUpdate(ctx, serverID, userID, sID, uID)
	role, err := s.repo.GetServerRoleByID(ctx, rID)
	if err != nil {
		log.Printf("Failed to fetch role %d name for audit log: %v", rID, err)
	}
	roleName := roleID
	if role != nil {
		roleName = role.Name
	}
	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      sID,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "member.role_remove",
		TargetID:      strconv.FormatInt(uID, 10),
		TargetType:    "user",
		TargetName:    roleName,
	})
	return nil
}

func (s *ServerService) broadcastRoleUpdate(ctx context.Context, serverID, userID string, sID, uID int64) {
	if s.hub == nil {
		return
	}
	dbRoles, err := s.repo.GetMemberRoles(ctx, sID, uID)
	if err != nil {
		log.Printf("Failed to fetch member roles for broadcast (server=%d, user=%d): %v", sID, uID, err)
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
		log.Printf("Failed to marshal member role update event: %v", err)
		return
	}
	s.hub.BroadcastToChannel("server:"+serverID, ws.EventMemberRoleUpdate, payload)
}
