package permissions

import (
	"context"
	"strconv"

	"parley/internal/db"
)

// Server-only permissions (bits 0-13)
const (
	PermAdministrator     int64 = 1 << 0
	PermManageServer      int64 = 1 << 1
	PermManageRoles       int64 = 1 << 2
	PermManageChannels    int64 = 1 << 3
	PermKickMembers       int64 = 1 << 4
	PermBanMembers        int64 = 1 << 5
	PermManageNicknames   int64 = 1 << 6
	PermChangeNickname    int64 = 1 << 7
	PermCreateInvite      int64 = 1 << 8
	PermViewAuditLog      int64 = 1 << 9
	PermManageWebhooks    int64 = 1 << 10
	PermManageExpressions int64 = 1 << 11
	PermManageEvents      int64 = 1 << 12
	PermModerateMember    int64 = 1 << 13
)

// Channel permissions — Text & Bin (bits 16-31)
const (
	PermViewChannel           int64 = 1 << 16
	PermSendMessages          int64 = 1 << 17
	PermEmbedLinks            int64 = 1 << 18
	PermAttachFiles           int64 = 1 << 19
	PermAddReactions          int64 = 1 << 20
	PermMentionEveryone       int64 = 1 << 21
	PermManageMessages        int64 = 1 << 22
	PermReadMessageHistory    int64 = 1 << 23
	PermUseExternalEmoji      int64 = 1 << 24
	PermPinMessages           int64 = 1 << 25
	PermManageThreads         int64 = 1 << 26
	PermCreatePublicThreads   int64 = 1 << 27
	PermSendMessagesInThreads int64 = 1 << 28
	PermCreatePosts           int64 = 1 << 29
	PermManagePosts           int64 = 1 << 30
	PermManageTags            int64 = 1 << 31
)

// Channel permissions — Voice (bits 32-41)
const (
	PermConnect            int64 = 1 << 32
	PermSpeak              int64 = 1 << 33
	PermMuteMembers        int64 = 1 << 34
	PermDeafenMembers      int64 = 1 << 35
	PermMoveMembers        int64 = 1 << 36
	PermUseVAD             int64 = 1 << 37
	PermPrioritySpeaker    int64 = 1 << 38
	PermStream             int64 = 1 << 39
	PermUseSoundboard      int64 = 1 << 40
	PermSendVoiceMessages  int64 = 1 << 41
)

// Masks
const (
	PermAllPermissions int64 = (1 << 42) - 1
	PermChannelMask    int64 = PermAllPermissions &^ ((1 << 16) - 1)
	PermServerOnlyMask int64 = (1 << 14) - 1
)

// Default @everyone permissions
const PermDefaultEveryone int64 = PermViewChannel | PermSendMessages | PermReadMessageHistory |
	PermAddReactions | PermEmbedLinks | PermAttachFiles | PermConnect | PermSpeak |
	PermUseVAD | PermChangeNickname | PermCreateInvite | PermCreatePosts

// Overwrite represents a permission overwrite for a channel.
type Overwrite struct {
	TargetType int   // 0 = role, 1 = member
	TargetID   int64
	Allow      int64
	Deny       int64
}

// ComputeBasePermissions computes server-wide permissions.
func ComputeBasePermissions(everyonePerms int64, memberRolePerms []int64, isOwner bool) int64 {
	if isOwner {
		return PermAllPermissions
	}
	perms := everyonePerms
	for _, rp := range memberRolePerms {
		perms |= rp
	}
	if perms&PermAdministrator != 0 {
		return PermAllPermissions
	}
	return perms
}

// ComputeChannelPermissions applies overwrites to base permissions.
func ComputeChannelPermissions(basePerms int64, memberID int64, memberRoleIDs []int64, everyoneRoleID int64, overwrites []Overwrite) int64 {
	if basePerms&PermAdministrator != 0 {
		return PermAllPermissions
	}
	perms := basePerms

	// Step 1: @everyone overwrites
	for _, ow := range overwrites {
		if ow.TargetType == 0 && ow.TargetID == everyoneRoleID {
			perms &= ^ow.Deny
			perms |= ow.Allow
			break
		}
	}

	// Step 2: Role overwrites (combined)
	var roleAllow, roleDeny int64
	roleSet := make(map[int64]bool, len(memberRoleIDs))
	for _, rid := range memberRoleIDs {
		roleSet[rid] = true
	}
	for _, ow := range overwrites {
		if ow.TargetType == 0 && ow.TargetID != everyoneRoleID && roleSet[ow.TargetID] {
			roleAllow |= ow.Allow
			roleDeny |= ow.Deny
		}
	}
	perms &= ^roleDeny
	perms |= roleAllow

	// Step 3: Member-specific overwrite
	for _, ow := range overwrites {
		if ow.TargetType == 1 && ow.TargetID == memberID {
			perms &= ^ow.Deny
			perms |= ow.Allow
			break
		}
	}

	// Implicit denials
	if perms&PermViewChannel == 0 {
		perms &= ^PermChannelMask
	}
	if perms&PermSendMessages == 0 {
		perms &= ^(PermMentionEveryone | PermAttachFiles | PermEmbedLinks)
	}
	if perms&PermConnect == 0 {
		perms &= ^(PermSpeak | PermMuteMembers | PermDeafenMembers | PermMoveMembers | PermUseVAD | PermPrioritySpeaker | PermStream | PermUseSoundboard | PermSendVoiceMessages)
	}

	return perms
}

// HasPerm checks a computed permission set for a specific permission.
func HasPerm(perms int64, perm int64) bool {
	return perms&perm == perm
}

// GetEffectivePermissions returns the OR of all role permission bits for a user in a server.
// If the user is the server owner or has PermAdministrator, all bits are set.
//
// DEPRECATED: Use ComputeBasePermissions with role data fetched from the repository instead.
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
	if perms&PermAdministrator != 0 {
		return ^int64(0), nil // Administrator implies all permissions
	}
	return perms, nil
}

// HasPermission returns true if the user has the given permission bit in the server.
//
// DEPRECATED: Use ComputeBasePermissions + HasPerm instead.
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
