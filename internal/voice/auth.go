package voice

import (
	"context"

	"parley/internal/db"
	"parley/internal/permissions"
)

// authzRepo is the slice of repository methods the Authorizer needs.
// Defined as an interface so tests can stub it without dragging in the full Repository.
type authzRepo interface {
	IsDmMember(ctx context.Context, dmID, userID int64) (bool, error)
	GetDmChannelByID(ctx context.Context, dmID int64) (*db.DmChannel, error)
	GetMember(ctx context.Context, serverID, userID int64) (*db.ServerMember, error)
	GetServerByID(ctx context.Context, serverID int64) (*db.Server, error)
	GetChannelByID(ctx context.Context, channelID int64) (*db.Channel, error)
}

// permChecker is the small surface of the permissions package used here.
// Wrapping it as a function-typed field lets tests substitute a fake.
type permChecker func(ctx context.Context, repo *db.Repository, serverID, userID, ownerID, channelID int64, perm int64) (bool, error)

type Authorizer struct {
	repo authzRepo
	// hasChannelPerm defaults to permissions.HasChannelPermission. Override in tests.
	hasChannelPerm permChecker
	// permRepo is the value passed as the Repo argument to hasChannelPerm.
	permRepo *db.Repository
}

func NewAuthorizer(repo *db.Repository) *Authorizer {
	return &Authorizer{
		repo:           repo,
		hasChannelPerm: permissions.HasChannelPermission,
		permRepo:       repo,
	}
}

// AuthorizeJoin returns true if userID is allowed to join the voice room vc.
func (a *Authorizer) AuthorizeJoin(ctx context.Context, vc VirtualChannel, userID int64) (bool, error) {
	switch vc.Kind {
	case KindDM:
		return a.repo.IsDmMember(ctx, vc.ID, userID)
	case KindServer:
		ch, err := a.repo.GetChannelByID(ctx, vc.ID)
		if err != nil || ch == nil {
			return false, err
		}
		m, err := a.repo.GetMember(ctx, ch.ServerID, userID)
		if err != nil || m == nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// AuthorizeMute returns true if actorID may force-mute targetID in vc.
// Server channels: PermMuteMembers role check.
// DM (1:1):       always false.
// DM (GC):        owner-only and never self.
func (a *Authorizer) AuthorizeMute(ctx context.Context, vc VirtualChannel, actorID, targetID int64) (bool, error) {
	if actorID == targetID {
		return false, nil
	}
	switch vc.Kind {
	case KindDM:
		dm, err := a.repo.GetDmChannelByID(ctx, vc.ID)
		if err != nil || dm == nil || !dm.IsGroup {
			return false, err
		}
		if dm.OwnerUserID == nil {
			return false, nil
		}
		return *dm.OwnerUserID == actorID, nil
	case KindServer:
		return a.serverChannelPerm(ctx, vc.ID, actorID, permissions.PermMuteMembers)
	}
	return false, nil
}

// AuthorizeKick mirrors AuthorizeMute but for force-disconnect (PermMoveMembers on servers).
func (a *Authorizer) AuthorizeKick(ctx context.Context, vc VirtualChannel, actorID, targetID int64) (bool, error) {
	if actorID == targetID {
		return false, nil
	}
	switch vc.Kind {
	case KindDM:
		dm, err := a.repo.GetDmChannelByID(ctx, vc.ID)
		if err != nil || dm == nil || !dm.IsGroup {
			return false, err
		}
		if dm.OwnerUserID == nil {
			return false, nil
		}
		return *dm.OwnerUserID == actorID, nil
	case KindServer:
		return a.serverChannelPerm(ctx, vc.ID, actorID, permissions.PermMoveMembers)
	}
	return false, nil
}

func (a *Authorizer) serverChannelPerm(ctx context.Context, channelID, userID int64, perm int64) (bool, error) {
	ch, err := a.repo.GetChannelByID(ctx, channelID)
	if err != nil || ch == nil {
		return false, err
	}
	srv, err := a.repo.GetServerByID(ctx, ch.ServerID)
	if err != nil || srv == nil {
		return false, err
	}
	return a.hasChannelPerm(ctx, a.permRepo, ch.ServerID, userID, srv.OwnerID, channelID, perm)
}
