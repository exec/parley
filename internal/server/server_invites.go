package server

import (
	"context"
	"database/sql"
	"errors"

	"parley/internal/db"
)

func (s *ServerService) CreateInvite(ctx context.Context, serverID, createdBy string) (*Invite, error) {
	if serverID == "" {
		return nil, errors.New("server ID is required")
	}
	if createdBy == "" {
		return nil, errors.New("created by is required")
	}

	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}
	createdByInt, err := idToInt64(createdBy)
	if err != nil {
		return nil, errors.New("invalid created by format")
	}

	// Generate random 8-character code, retrying if it collides with invite codes or vanity URLs
	var code string
	for attempts := 0; attempts < 10; attempts++ {
		candidate, genErr := generateInviteCode()
		if genErr != nil {
			return nil, genErr
		}
		exists, checkErr := s.repo.InviteCodeExists(ctx, candidate)
		if checkErr != nil {
			return nil, checkErr
		}
		if !exists {
			code = candidate
			break
		}
	}
	if code == "" {
		return nil, errors.New("failed to generate unique invite code")
	}

	invite := &db.Invite{
		ServerID:  serverIDInt,
		Code:      code,
		CreatedBy: createdByInt,
	}
	if err = s.repo.CreateInvite(ctx, invite); err != nil {
		return nil, err
	}

	return &Invite{
		ID:        int64ToID(invite.ID),
		ServerID:  int64ToID(invite.ServerID),
		Code:      invite.Code,
		CreatedBy: int64ToID(invite.CreatedBy),
		CreatedAt: invite.CreatedAt,
	}, nil
}

func (s *ServerService) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	if code == "" {
		return nil, errors.New("invite code is required")
	}

	invite, err := s.repo.GetInviteByCode(ctx, code)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	return &Invite{
		ID:        int64ToID(invite.ID),
		ServerID:  int64ToID(invite.ServerID),
		Code:      invite.Code,
		CreatedBy: int64ToID(invite.CreatedBy),
		CreatedAt: invite.CreatedAt,
	}, nil
}

func (s *ServerService) GetServerByInviteCode(ctx context.Context, code string) (*Server, error) {
	if code == "" {
		return nil, errors.New("invite code is required")
	}

	server, err := s.repo.GetServerByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

func (s *ServerService) JoinServerByInvite(ctx context.Context, code, userID string) (*Server, error) {
	if code == "" {
		return nil, errors.New("invite code is required")
	}
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	server, err := s.repo.GetServerByInviteCode(ctx, code)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   userIDInt,
		Nickname: "",
	}
	if err = s.repo.AddMember(ctx, member); err != nil {
		// Already a member — just return the server
		if _, getErr := s.repo.GetMember(ctx, server.ID, userIDInt); getErr == nil {
			return dbServerToService(server), nil
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

func (s *ServerService) JoinServerByVanityURL(ctx context.Context, vanityURL, userID string) (*Server, error) {
	if vanityURL == "" {
		return nil, errors.New("vanity URL is required")
	}
	if userID == "" {
		return nil, errors.New("user ID is required")
	}

	server, err := s.repo.GetServerByVanityURL(ctx, vanityURL)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, errors.New("server not found")
		}
		return nil, err
	}

	userIDInt, err := idToInt64(userID)
	if err != nil {
		return nil, errors.New("invalid user ID format")
	}

	member := &db.ServerMember{
		ServerID: server.ID,
		UserID:   userIDInt,
		Nickname: "",
	}
	if err = s.repo.AddMember(ctx, member); err != nil {
		if _, getErr := s.repo.GetMember(ctx, server.ID, userIDInt); getErr == nil {
			return dbServerToService(server), nil
		}
		return nil, err
	}

	return dbServerToService(server), nil
}

func (s *ServerService) SetVanityURL(ctx context.Context, serverID, userID, vanityURL string) (*Server, error) {
	if serverID == "" {
		return nil, errors.New("server ID is required")
	}
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID format")
	}

	srv, err := s.repo.GetServerByID(ctx, serverIDInt)
	if err != nil {
		return nil, errors.New("server not found")
	}
	userIDInt, _ := idToInt64(userID)
	if srv.OwnerID != userIDInt {
		return nil, errors.New("only the server owner can set the vanity URL")
	}

	slug := sql.NullString{}
	if vanityURL != "" {
		exists, checkErr := s.repo.InviteCodeExists(ctx, vanityURL, serverIDInt)
		if checkErr != nil {
			return nil, checkErr
		}
		if exists {
			return nil, errors.New("that URL is already in use — choose a different one")
		}
		slug = sql.NullString{String: vanityURL, Valid: true}
	}

	if err := s.repo.SetVanityURL(ctx, serverIDInt, slug); err != nil {
		return nil, err
	}

	srv.VanityURL = slug
	return dbServerToService(srv), nil
}
