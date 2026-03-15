package permissions

import (
	"context"
	"strconv"

	"parley/internal/db"
)

// Permission bit constants — must match the frontend ManageRolesModal PERMISSIONS array.
const (
	PermSendMessages   int64 = 1
	PermManageMessages int64 = 2
	PermManageChannels int64 = 4
	PermKickMembers    int64 = 8
	PermManageServer   int64 = 16
)

// GetEffectivePermissions returns the OR of all role permission bits for a user in a server.
// If the user is the server owner, all bits are set (owner bypasses everything).
func GetEffectivePermissions(ctx context.Context, repo *db.Repository, serverID, userID, ownerID int64) (int64, error) {
	if userID == ownerID {
		return ^int64(0), nil // all permissions
	}
	roles, err := repo.GetMemberRoles(ctx, serverID, userID)
	if err != nil {
		return 0, err
	}
	var perms int64
	for _, role := range roles {
		perms |= role.Permissions
	}
	return perms, nil
}

// HasPermission returns true if the user has the given permission bit in the server.
func HasPermission(ctx context.Context, repo *db.Repository, serverID, userID, ownerID int64, perm int64) (bool, error) {
	effective, err := GetEffectivePermissions(ctx, repo, serverID, userID, ownerID)
	if err != nil {
		return false, err
	}
	return effective&perm != 0, nil
}

// ParseInt64 is a convenience wrapper for strconv.ParseInt used by handlers.
func ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
