package server

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"parley/internal/audit"
	"parley/internal/db"
)

func (s *ServerService) CreateInvite(ctx context.Context, serverID, createdBy string, maxUses *int, expiresAt *time.Time, actorUsername string) (*Invite, error) {
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
		MaxUses:   maxUses,
		ExpiresAt: expiresAt,
	}
	if err = s.repo.CreateInvite(ctx, invite); err != nil {
		return nil, err
	}

	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &createdByInt,
		ActorUsername: actorUsername,
		Action:        "invite.create",
		TargetID:      invite.Code,
		TargetType:    "invite",
		TargetName:    invite.Code,
	})

	return &Invite{
		ID:        int64ToID(invite.ID),
		ServerID:  int64ToID(invite.ServerID),
		Code:      invite.Code,
		CreatedBy: int64ToID(invite.CreatedBy),
		CreatedAt: invite.CreatedAt,
		MaxUses:   invite.MaxUses,
		ExpiresAt: invite.ExpiresAt,
		UseCount:  0,
		IsActive:  true,
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

	now := time.Now()
	isActive := invite.RevokedAt == nil &&
		(invite.ExpiresAt == nil || now.Before(*invite.ExpiresAt)) &&
		(invite.MaxUses == nil || invite.UseCount < *invite.MaxUses)
	return &Invite{
		ID:        int64ToID(invite.ID),
		ServerID:  int64ToID(invite.ServerID),
		Code:      invite.Code,
		CreatedBy: int64ToID(invite.CreatedBy),
		CreatedAt: invite.CreatedAt,
		MaxUses:   invite.MaxUses,
		ExpiresAt: invite.ExpiresAt,
		UseCount:  invite.UseCount,
		IsActive:  isActive,
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

	if banned, err := s.repo.IsServerBanned(ctx, server.ID, userIDInt); err != nil {
		return nil, err
	} else if banned {
		return nil, errors.New("you are banned from this server")
	}

	member := &db.ServerMember{
		ServerID:   server.ID,
		UserID:     userIDInt,
		Nickname:   "",
		InviteCode: code,
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

	if banned, err := s.repo.IsServerBanned(ctx, server.ID, userIDInt); err != nil {
		return nil, err
	} else if banned {
		return nil, errors.New("you are banned from this server")
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

func (s *ServerService) GetServerInvites(ctx context.Context, serverID string) ([]*Invite, error) {
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}

	invites, err := s.repo.GetServerInvites(ctx, serverIDInt)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	result := make([]*Invite, 0, len(invites))
	for _, inv := range invites {
		isActive := inv.RevokedAt == nil &&
			(inv.ExpiresAt == nil || now.Before(*inv.ExpiresAt)) &&
			(inv.MaxUses == nil || inv.UseCount < *inv.MaxUses)
		result = append(result, &Invite{
			ID:              int64ToID(inv.ID),
			ServerID:        int64ToID(inv.ServerID),
			Code:            inv.Code,
			CreatedBy:       int64ToID(inv.CreatedBy),
			CreatorUsername: inv.CreatorUsername,
			CreatedAt:       inv.CreatedAt,
			MaxUses:         inv.MaxUses,
			ExpiresAt:       inv.ExpiresAt,
			UseCount:        inv.UseCount,
			IsActive:        isActive,
		})
	}
	return result, nil
}

func (s *ServerService) RevokeInvite(ctx context.Context, serverID, code, requestingUserID string, actorUsername string) error {
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return errors.New("invalid server ID")
	}

	// Check requester is a member
	userIDInt, err := idToInt64(requestingUserID)
	if err != nil {
		return errors.New("invalid user ID")
	}
	_, err = s.repo.GetMember(ctx, serverIDInt, userIDInt)
	if err != nil {
		return errors.New("not a member of this server")
	}

	if err := s.repo.RevokeInvite(ctx, code, serverIDInt); err != nil {
		return err
	}

	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &userIDInt,
		ActorUsername: actorUsername,
		Action:        "invite.revoke",
		TargetID:      code,
		TargetType:    "invite",
		TargetName:    code,
	})

	return nil
}

func (s *ServerService) GetInviteMembers(ctx context.Context, serverID, code, requestingUserID string) ([]*db.InviteMember, error) {
	serverIDInt, err := idToInt64(serverID)
	if err != nil {
		return nil, errors.New("invalid server ID")
	}

	userIDInt, err := idToInt64(requestingUserID)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}
	_, err = s.repo.GetMember(ctx, serverIDInt, userIDInt)
	if err != nil {
		return nil, errors.New("not a member of this server")
	}

	return s.repo.GetMembersByInviteCode(ctx, code, serverIDInt)
}

func (s *ServerService) SetVanityURL(ctx context.Context, serverID, vanityURL string, actorID int64, actorUsername string) (*Server, error) {
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
	if srv.OwnerID != actorID {
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

	s.auditSvc.Log(ctx, audit.Entry{
		ServerID:      serverIDInt,
		ActorID:       &actorID,
		ActorUsername: actorUsername,
		Action:        "server.vanity_update",
		TargetType:    "server",
		TargetName:    vanityURL,
	})

	return dbServerToService(srv), nil
}
