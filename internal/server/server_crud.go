package server

import (
	"context"
	"encoding/json"
	"errors"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

func (s *ServerService) CreateServer(ctx context.Context, name, iconURL string, ownerID string) (*Server, error) {
	if name == "" {
		return nil, errors.New("server name is required")
	}
	if ownerID == "" {
		return nil, errors.New("owner ID is required")
	}

	ownerIDInt, err := idToInt64(ownerID)
	if err != nil {
		return nil, errors.New("invalid owner ID format")
	}

	server := &db.Server{
		Name:    name,
		IconURL: nullString(iconURL),
		OwnerID: ownerIDInt,
	}
	if err = s.repo.CreateServer(ctx, server); err != nil {
		return nil, err
	}

	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   ownerIDInt,
		Nickname: "",
	}
	if err = s.repo.AddMember(ctx, member); err != nil {
		return nil, err
	}

	return dbServerToService(server), nil
}

func (s *ServerService) GetServer(ctx context.Context, id string) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	server, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

func (s *ServerService) GetUserServers(ctx context.Context, userID string) ([]*Server, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	servers, err := s.repo.GetServersByUserID(ctx, userIDInt)
	if err != nil {
		return nil, err
	}

	result := make([]*Server, len(servers))
	for i, server := range servers {
		result[i] = dbServerToService(server)
	}
	return result, nil
}

func (s *ServerService) UpdateServer(ctx context.Context, id, name, iconURL string) (*Server, error) {
	if id == "" {
		return nil, errors.New("server ID is required")
	}
	if name == "" {
		return nil, errors.New("server name is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	server, err := s.repo.GetServerByID(ctx, serverID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	server.Name = name
	server.IconURL = nullString(iconURL)

	if err = s.repo.UpdateServer(ctx, server); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	srv := dbServerToService(server)
	if s.hub != nil {
		if payload, err := json.Marshal(srv); err == nil {
			s.hub.BroadcastToChannel("server:"+id, ws.EventServerUpdate, payload)
		}
	}
	return srv, nil
}

func (s *ServerService) DeleteServer(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("server ID is required")
	}

	serverID, err := idToInt64(id)
	if err != nil {
		return errors.New("invalid server ID format")
	}

	if s.hub != nil {
		payload, _ := json.Marshal(map[string]string{"server_id": id})
		s.hub.BroadcastToChannel("server:"+id, ws.EventServerDelete, payload)
	}

	if err = s.repo.DeleteServer(ctx, serverID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errors.New("server not found")
		}
		return err
	}
	return nil
}

func (s *ServerService) GetServerByVanityURL(ctx context.Context, vanityURL string) (*Server, error) {
	srv, err := s.repo.GetServerByVanityURL(ctx, vanityURL)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}
	return dbServerToService(srv), nil
}
