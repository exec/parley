package permissions

import (
	"testing"

	"parley/internal/db"
)

// ---------------------------------------------------------------------------
// ComputeBasePermissions
// ---------------------------------------------------------------------------

func TestComputeBasePermissions_OwnerGetsAll(t *testing.T) {
	perms := ComputeBasePermissions(0, nil, true)
	if perms != PermAllPermissions {
		t.Errorf("owner should get PermAllPermissions, got %d", perms)
	}
}

func TestComputeBasePermissions_OwnerIgnoresRoles(t *testing.T) {
	// Even with zero everyone perms and no roles, owner gets all.
	perms := ComputeBasePermissions(0, []int64{PermViewChannel}, true)
	if perms != PermAllPermissions {
		t.Errorf("owner should get PermAllPermissions regardless of roles, got %d", perms)
	}
}

func TestComputeBasePermissions_EveryoneOnly(t *testing.T) {
	perms := ComputeBasePermissions(PermViewChannel|PermSendMessages, nil, false)
	want := PermViewChannel | PermSendMessages
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeBasePermissions_RolesORed(t *testing.T) {
	everyone := PermViewChannel
	roles := []int64{PermSendMessages, PermAttachFiles}
	perms := ComputeBasePermissions(everyone, roles, false)
	want := PermViewChannel | PermSendMessages | PermAttachFiles
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeBasePermissions_AdminGetsAll(t *testing.T) {
	// A role granting Administrator should yield all permissions.
	perms := ComputeBasePermissions(PermViewChannel, []int64{PermAdministrator}, false)
	if perms != PermAllPermissions {
		t.Errorf("admin role should yield PermAllPermissions, got %d", perms)
	}
}

func TestComputeBasePermissions_AdminInEveryoneRole(t *testing.T) {
	perms := ComputeBasePermissions(PermAdministrator, nil, false)
	if perms != PermAllPermissions {
		t.Errorf("admin in @everyone should yield PermAllPermissions, got %d", perms)
	}
}

func TestComputeBasePermissions_NoPerms(t *testing.T) {
	perms := ComputeBasePermissions(0, nil, false)
	if perms != 0 {
		t.Errorf("expected 0, got %d", perms)
	}
}

func TestComputeBasePermissions_EmptyRoleSlice(t *testing.T) {
	perms := ComputeBasePermissions(PermViewChannel, []int64{}, false)
	if perms != PermViewChannel {
		t.Errorf("empty roles should not change everyone perms, got %d", perms)
	}
}

func TestComputeBasePermissions_MultipleRolesWithOverlap(t *testing.T) {
	everyone := PermViewChannel | PermSendMessages
	roles := []int64{
		PermSendMessages | PermAttachFiles,
		PermAttachFiles | PermEmbedLinks,
	}
	perms := ComputeBasePermissions(everyone, roles, false)
	want := PermViewChannel | PermSendMessages | PermAttachFiles | PermEmbedLinks
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

// ---------------------------------------------------------------------------
// ComputeChannelPermissions
// ---------------------------------------------------------------------------

func TestComputeChannelPermissions_AdminBypass(t *testing.T) {
	perms := ComputeChannelPermissions(PermAdministrator, 1, nil, 100, nil)
	if perms != PermAllPermissions {
		t.Errorf("admin should bypass to PermAllPermissions, got %d", perms)
	}
}

func TestComputeChannelPermissions_NoOverwrites(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	perms := ComputeChannelPermissions(base, 1, nil, 100, nil)
	if perms != base {
		t.Errorf("no overwrites should return base, got %d, want %d", perms, base)
	}
}

func TestComputeChannelPermissions_EveryoneDeny(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	overwrites := []Overwrite{
		{TargetType: 0, TargetID: 100, Allow: 0, Deny: PermSendMessages},
	}
	perms := ComputeChannelPermissions(base, 1, nil, 100, overwrites)
	want := PermViewChannel
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_EveryoneAllow(t *testing.T) {
	base := PermViewChannel
	overwrites := []Overwrite{
		{TargetType: 0, TargetID: 100, Allow: PermSendMessages, Deny: 0},
	}
	perms := ComputeChannelPermissions(base, 1, nil, 100, overwrites)
	want := PermViewChannel | PermSendMessages
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_RoleOverwriteOverridesEveryone(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	roleID := int64(200)
	overwrites := []Overwrite{
		// @everyone denies SendMessages
		{TargetType: 0, TargetID: 100, Allow: 0, Deny: PermSendMessages},
		// Role 200 allows SendMessages back
		{TargetType: 0, TargetID: roleID, Allow: PermSendMessages, Deny: 0},
	}
	perms := ComputeChannelPermissions(base, 1, []int64{roleID}, 100, overwrites)
	want := PermViewChannel | PermSendMessages
	if perms != want {
		t.Errorf("role overwrite should restore SendMessages, got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_MultipleRoleOverwritesCombined(t *testing.T) {
	base := PermViewChannel
	role1 := int64(201)
	role2 := int64(202)
	overwrites := []Overwrite{
		{TargetType: 0, TargetID: role1, Allow: PermSendMessages, Deny: 0},
		{TargetType: 0, TargetID: role2, Allow: PermAttachFiles, Deny: 0},
	}
	perms := ComputeChannelPermissions(base, 1, []int64{role1, role2}, 100, overwrites)
	want := PermViewChannel | PermSendMessages | PermAttachFiles
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_RoleDenyAndAllowCombined(t *testing.T) {
	// When multiple role overwrites exist, all deny bits are combined and all
	// allow bits are combined. Allow is applied after deny, so if the same bit
	// appears in both, it ends up allowed.
	base := PermViewChannel | PermSendMessages | PermAttachFiles
	role1 := int64(201)
	role2 := int64(202)
	overwrites := []Overwrite{
		// role1 denies SendMessages
		{TargetType: 0, TargetID: role1, Allow: 0, Deny: PermSendMessages},
		// role2 allows SendMessages
		{TargetType: 0, TargetID: role2, Allow: PermSendMessages, Deny: 0},
	}
	perms := ComputeChannelPermissions(base, 1, []int64{role1, role2}, 100, overwrites)
	// Allow applied after deny => SendMessages restored
	want := PermViewChannel | PermSendMessages | PermAttachFiles
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_MemberOverwriteOverridesRole(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	memberID := int64(42)
	roleID := int64(200)
	overwrites := []Overwrite{
		// Role denies SendMessages
		{TargetType: 0, TargetID: roleID, Allow: 0, Deny: PermSendMessages},
		// Member-specific allows it back
		{TargetType: 1, TargetID: memberID, Allow: PermSendMessages, Deny: 0},
	}
	perms := ComputeChannelPermissions(base, memberID, []int64{roleID}, 100, overwrites)
	want := PermViewChannel | PermSendMessages
	if perms != want {
		t.Errorf("member overwrite should restore SendMessages, got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_MemberDeny(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	memberID := int64(42)
	overwrites := []Overwrite{
		{TargetType: 1, TargetID: memberID, Allow: 0, Deny: PermSendMessages},
	}
	perms := ComputeChannelPermissions(base, memberID, nil, 100, overwrites)
	want := PermViewChannel
	if perms != want {
		t.Errorf("got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_UnrelatedOverwritesIgnored(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	overwrites := []Overwrite{
		// Role overwrite for a role the member doesn't have
		{TargetType: 0, TargetID: 999, Allow: 0, Deny: PermSendMessages},
		// Member overwrite for a different member
		{TargetType: 1, TargetID: 999, Allow: 0, Deny: PermViewChannel},
	}
	perms := ComputeChannelPermissions(base, 42, []int64{200}, 100, overwrites)
	if perms != base {
		t.Errorf("unrelated overwrites should be ignored, got %d, want %d", perms, base)
	}
}

// ---------------------------------------------------------------------------
// Implicit denials
// ---------------------------------------------------------------------------

func TestComputeChannelPermissions_ImplicitDeny_NoViewChannel(t *testing.T) {
	// Without ViewChannel, all channel-scoped bits are stripped.
	base := PermSendMessages | PermAttachFiles | PermConnect | PermSpeak
	perms := ComputeChannelPermissions(base, 1, nil, 100, nil)
	if perms&PermChannelMask != 0 {
		t.Errorf("without ViewChannel, all channel bits should be 0, got %d", perms)
	}
}

func TestComputeChannelPermissions_ImplicitDeny_NoSendMessages(t *testing.T) {
	base := PermViewChannel // no SendMessages
	perms := ComputeChannelPermissions(base, 1, nil, 100, nil)
	denied := PermMentionEveryone | PermAttachFiles | PermEmbedLinks
	if perms&denied != 0 {
		t.Errorf("without SendMessages, mention/attach/embed should be denied, got %d", perms)
	}
}

func TestComputeChannelPermissions_ImplicitDeny_NoConnect(t *testing.T) {
	base := PermViewChannel // no Connect
	perms := ComputeChannelPermissions(base, 1, nil, 100, nil)
	voiceBits := PermSpeak | PermMuteMembers | PermDeafenMembers | PermMoveMembers |
		PermUseVAD | PermPrioritySpeaker | PermStream | PermUseSoundboard | PermSendVoiceMessages
	if perms&voiceBits != 0 {
		t.Errorf("without Connect, voice bits should be denied, got %d", perms)
	}
}

func TestComputeChannelPermissions_ImplicitDeny_ViewChannelPreservesServerBits(t *testing.T) {
	// Server-only bits (0-13) should NOT be stripped even without ViewChannel.
	base := PermManageServer | PermKickMembers
	perms := ComputeChannelPermissions(base, 1, nil, 100, nil)
	want := PermManageServer | PermKickMembers
	if perms != want {
		t.Errorf("server-only bits should survive, got %d, want %d", perms, want)
	}
}

func TestComputeChannelPermissions_ImplicitDeny_SendMessagesKeepsViewChannel(t *testing.T) {
	// Without SendMessages, ViewChannel itself is not stripped.
	base := PermViewChannel | PermReadMessageHistory
	perms := ComputeChannelPermissions(base, 1, nil, 100, nil)
	if perms&PermViewChannel == 0 {
		t.Error("ViewChannel should survive even without SendMessages")
	}
	if perms&PermReadMessageHistory == 0 {
		t.Error("ReadMessageHistory should survive without SendMessages")
	}
}

// ---------------------------------------------------------------------------
// Full overwrite cascade (everyone -> roles -> member)
// ---------------------------------------------------------------------------

func TestComputeChannelPermissions_FullCascade(t *testing.T) {
	base := PermViewChannel | PermSendMessages | PermAttachFiles | PermEmbedLinks
	everyoneRoleID := int64(100)
	memberID := int64(42)
	roleA := int64(201)
	roleB := int64(202)

	overwrites := []Overwrite{
		// Step 1: @everyone denies SendMessages and AttachFiles
		{TargetType: 0, TargetID: everyoneRoleID, Allow: 0, Deny: PermSendMessages | PermAttachFiles},
		// Step 2: roleA allows SendMessages back
		{TargetType: 0, TargetID: roleA, Allow: PermSendMessages, Deny: 0},
		// Step 2: roleB denies EmbedLinks
		{TargetType: 0, TargetID: roleB, Allow: 0, Deny: PermEmbedLinks},
		// Step 3: member allows AttachFiles back
		{TargetType: 1, TargetID: memberID, Allow: PermAttachFiles, Deny: 0},
	}

	perms := ComputeChannelPermissions(base, memberID, []int64{roleA, roleB}, everyoneRoleID, overwrites)
	want := PermViewChannel | PermSendMessages | PermAttachFiles
	if perms != want {
		t.Errorf("full cascade: got %d, want %d", perms, want)
	}
}

// ---------------------------------------------------------------------------
// HasPerm
// ---------------------------------------------------------------------------

func TestHasPerm_Single(t *testing.T) {
	perms := PermViewChannel | PermSendMessages
	if !HasPerm(perms, PermViewChannel) {
		t.Error("should have ViewChannel")
	}
	if !HasPerm(perms, PermSendMessages) {
		t.Error("should have SendMessages")
	}
	if HasPerm(perms, PermAttachFiles) {
		t.Error("should not have AttachFiles")
	}
}

func TestHasPerm_Multiple(t *testing.T) {
	perms := PermViewChannel | PermSendMessages | PermAttachFiles
	combined := PermViewChannel | PermSendMessages
	if !HasPerm(perms, combined) {
		t.Error("should have both ViewChannel and SendMessages")
	}
}

func TestHasPerm_MultiplePartialMiss(t *testing.T) {
	perms := PermViewChannel
	combined := PermViewChannel | PermSendMessages
	if HasPerm(perms, combined) {
		t.Error("should not have both when only one is present")
	}
}

func TestHasPerm_Zero(t *testing.T) {
	// Checking for perm 0 should always be true (no bits required).
	if !HasPerm(0, 0) {
		t.Error("HasPerm(0, 0) should be true")
	}
	if !HasPerm(PermViewChannel, 0) {
		t.Error("HasPerm(any, 0) should be true")
	}
}

func TestHasPerm_AllPermissions(t *testing.T) {
	if !HasPerm(PermAllPermissions, PermViewChannel) {
		t.Error("AllPermissions should contain ViewChannel")
	}
	if !HasPerm(PermAllPermissions, PermSendVoiceMessages) {
		t.Error("AllPermissions should contain SendVoiceMessages (highest bit)")
	}
}

// ---------------------------------------------------------------------------
// ParseInt64
// ---------------------------------------------------------------------------

func TestParseInt64_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"1", 1},
		{"-1", -1},
		{"9223372036854775807", 9223372036854775807},  // max int64
		{"-9223372036854775808", -9223372036854775808}, // min int64
		{"123456789", 123456789},
	}
	for _, tt := range tests {
		got, err := ParseInt64(tt.input)
		if err != nil {
			t.Errorf("ParseInt64(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseInt64(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseInt64_Invalid(t *testing.T) {
	invalids := []string{"", "abc", "12.5", "9223372036854775808", " 1"}
	for _, s := range invalids {
		_, err := ParseInt64(s)
		if err == nil {
			t.Errorf("ParseInt64(%q) expected error, got nil", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Constants sanity checks
// ---------------------------------------------------------------------------

func TestPermMasks(t *testing.T) {
	// PermAllPermissions should have bits 0-42 set.
	if PermAllPermissions != (1<<43)-1 {
		t.Errorf("PermAllPermissions = %d, want (1<<43)-1", PermAllPermissions)
	}

	// PermChannelMask should have no bits in the server-only range (0-15).
	if PermChannelMask&((1<<16)-1) != 0 {
		t.Error("PermChannelMask should have no bits in the 0-15 range")
	}

	// PermServerOnlyMask should cover bits 0-13.
	if PermServerOnlyMask != (1<<14)-1 {
		t.Errorf("PermServerOnlyMask = %d, want (1<<14)-1", PermServerOnlyMask)
	}
}

func TestPermDefaultEveryone(t *testing.T) {
	// Default everyone should include ViewChannel and SendMessages.
	if PermDefaultEveryone&PermViewChannel == 0 {
		t.Error("default everyone should include ViewChannel")
	}
	if PermDefaultEveryone&PermSendMessages == 0 {
		t.Error("default everyone should include SendMessages")
	}
	// Should not include admin.
	if PermDefaultEveryone&PermAdministrator != 0 {
		t.Error("default everyone should not include Administrator")
	}
}

// ---------------------------------------------------------------------------
// Overwrite type alias
// ---------------------------------------------------------------------------

func TestOverwriteTypeAlias(t *testing.T) {
	// Verify that permissions.Overwrite is the same type as db.Overwrite.
	var ow Overwrite = db.Overwrite{
		TargetType: 0,
		TargetID:   1,
		Allow:      PermViewChannel,
		Deny:       PermSendMessages,
	}
	if ow.Allow != PermViewChannel {
		t.Error("Overwrite type alias should work with db.Overwrite values")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestComputeChannelPermissions_EmptyOverwrites(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	perms := ComputeChannelPermissions(base, 1, []int64{200, 201}, 100, []Overwrite{})
	if perms != base {
		t.Errorf("empty overwrites should return base, got %d", perms)
	}
}

func TestComputeChannelPermissions_EveryoneAllowAndDenySameBit(t *testing.T) {
	// If @everyone overwrite both allows and denies the same bit,
	// deny is applied first, then allow — so the bit ends up set.
	base := PermViewChannel
	overwrites := []Overwrite{
		{TargetType: 0, TargetID: 100, Allow: PermSendMessages, Deny: PermSendMessages},
	}
	perms := ComputeChannelPermissions(base, 1, nil, 100, overwrites)
	if perms&PermSendMessages == 0 {
		t.Error("when same bit is in both allow and deny of @everyone, allow should win (applied after deny)")
	}
}

func TestComputeChannelPermissions_MemberAllowAndDenySameBit(t *testing.T) {
	base := PermViewChannel | PermSendMessages
	memberID := int64(42)
	overwrites := []Overwrite{
		{TargetType: 1, TargetID: memberID, Allow: PermSendMessages, Deny: PermSendMessages},
	}
	perms := ComputeChannelPermissions(base, memberID, nil, 100, overwrites)
	if perms&PermSendMessages == 0 {
		t.Error("when same bit is in both allow and deny of member overwrite, allow should win")
	}
}

func TestComputeBasePermissions_VoiceBitsPreserved(t *testing.T) {
	// Verify high bits (voice permissions, bits 32-41) are handled correctly.
	everyone := PermConnect | PermSpeak | PermStream
	perms := ComputeBasePermissions(everyone, nil, false)
	if perms != everyone {
		t.Errorf("high bits should survive, got %d, want %d", perms, everyone)
	}
}

func TestComputeChannelPermissions_HighBitsInOverwrites(t *testing.T) {
	base := PermViewChannel | PermConnect | PermSpeak
	overwrites := []Overwrite{
		{TargetType: 0, TargetID: 100, Allow: 0, Deny: PermSpeak},
	}
	perms := ComputeChannelPermissions(base, 1, nil, 100, overwrites)
	want := PermViewChannel | PermConnect
	if perms != want {
		t.Errorf("high bit deny should work, got %d, want %d", perms, want)
	}
}

// ---------------------------------------------------------------------------
// GetEffectivePermissions, HasPermission, HasChannelPermission,
// HasChannelPermissionCached
//
// These functions require a *db.Repository backed by a real SQL database.
// db.Repository is a concrete struct (not an interface), so it cannot be
// easily mocked without refactoring. Integration tests with a test database
// should be added for these functions.
// ---------------------------------------------------------------------------
