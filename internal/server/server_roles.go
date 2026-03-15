package server

import (
	"context"
	"encoding/json"
	"errors"

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

func (s *ServerService) CreateServerRole(ctx context.Context, serverID, name, color string, permissions int64) (*Role, error) {
	id, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	if name == "" {
		return nil, errors.New("role name is required")
	}
	if color == "" {
		color = "#99aab5"
	}
	dbRole, err := s.repo.CreateServerRole(ctx, id, name, color, permissions)
	if err != nil {
		return nil, err
	}
	r := dbRoleToRole(*dbRole)
	return &r, nil
}

func (s *ServerService) DeleteServerRole(ctx context.Context, serverID, roleID string) error {
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
	return nil
}

func (s *ServerService) UpdateServerRole(ctx context.Context, serverID, roleID, name, color string, permissions int64, hoist bool, position int) (*Role, error) {
	sID, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}
	rID, err := idToInt64(roleID)
	if err != nil {
		return nil, errors.New("invalid role ID")
	}
	dbRole, err := s.repo.UpdateServerRole(ctx, sID, rID, name, color, permissions, hoist, position)
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

func (s *ServerService) AssignRoleToMember(ctx context.Context, serverID, userID, roleID string) error {
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
	return nil
}

func (s *ServerService) RemoveRoleFromMember(ctx context.Context, serverID, userID, roleID string) error {
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
