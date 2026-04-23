package server

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"parley/internal/cache"
	"parley/internal/db"
	ws "parley/internal/websocket"
)

// --- Helper function tests ---

func TestNullString(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"", false},
		{"hello", true},
	}
	for _, tt := range tests {
		ns := nullString(tt.input)
		if ns.Valid != tt.valid {
			t.Errorf("nullString(%q).Valid = %v, want %v", tt.input, ns.Valid, tt.valid)
		}
		if tt.valid && ns.String != tt.input {
			t.Errorf("nullString(%q).String = %q", tt.input, ns.String)
		}
	}
}

func TestNullStringToString(t *testing.T) {
	tests := []struct {
		input sql.NullString
		want  string
	}{
		{sql.NullString{Valid: false}, ""},
		{sql.NullString{String: "hello", Valid: true}, "hello"},
	}
	for _, tt := range tests {
		got := nullStringToString(tt.input)
		if got != tt.want {
			t.Errorf("nullStringToString(%+v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInt64ToID(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{9999999, "9999999"},
		{-1, "-1"},
	}
	for _, tt := range tests {
		got := int64ToID(tt.input)
		if got != tt.want {
			t.Errorf("int64ToID(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIdToInt64(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"1", 1, false},
		{"0", 0, false},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := idToInt64(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("idToInt64(%q) should error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("idToInt64(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("idToInt64(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestGenerateInviteCode(t *testing.T) {
	code, err := generateInviteCode()
	if err != nil {
		t.Fatalf("generateInviteCode: %v", err)
	}
	if len(code) != 12 { // 6 bytes -> 12 hex chars
		t.Errorf("invite code length = %d, want 12", len(code))
	}

	// Uniqueness: generate several codes and check they differ
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		c, err := generateInviteCode()
		if err != nil {
			t.Fatal(err)
		}
		codes[c] = true
	}
	if len(codes) < 95 { // statistically they should all be unique
		t.Errorf("generated only %d unique codes out of 100", len(codes))
	}
}

func TestDbServerToService(t *testing.T) {
	now := time.Now()
	srv := &db.Server{
		ID:          42,
		Name:        "Test Server",
		IconURL:     sql.NullString{String: "https://example.com/icon.png", Valid: true},
		OwnerID:     7,
		VanityURL:   sql.NullString{Valid: false},
		Description: sql.NullString{String: "A test server", Valid: true},
		IsPublic:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	result := dbServerToService(srv)
	if result.ID != "42" {
		t.Errorf("ID = %q, want %q", result.ID, "42")
	}
	if result.Name != "Test Server" {
		t.Errorf("Name = %q", result.Name)
	}
	if result.IconURL != "https://example.com/icon.png" {
		t.Errorf("IconURL = %q", result.IconURL)
	}
	if result.OwnerID != "7" {
		t.Errorf("OwnerID = %q", result.OwnerID)
	}
	if result.VanityURL != "" {
		t.Errorf("VanityURL = %q, want empty", result.VanityURL)
	}
	if result.Description != "A test server" {
		t.Errorf("Description = %q", result.Description)
	}
	if !result.IsPublic {
		t.Error("IsPublic = false, want true")
	}
}

func TestDbRoleToRole(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		input    db.ServerRole
		wantName string
	}{
		{
			name: "normal role",
			input: db.ServerRole{
				ID:          1,
				ServerID:    10,
				Name:        "Admin",
				Color:       "#ff0000",
				Permissions: 0xFF,
				Hoist:       true,
				Position:    1,
				IsEveryone:  false,
				CreatedAt:   now,
			},
			wantName: "Admin",
		},
		{
			name: "everyone role gets renamed",
			input: db.ServerRole{
				ID:          2,
				ServerID:    10,
				Name:        "everyone",
				IsEveryone:  true,
				CreatedAt:   now,
			},
			wantName: "@everyone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := dbRoleToRole(tt.input)
			if r.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", r.Name, tt.wantName)
			}
			if r.ID != int64ToID(tt.input.ID) {
				t.Errorf("ID = %q, want %q", r.ID, int64ToID(tt.input.ID))
			}
			if r.IsEveryone != tt.input.IsEveryone {
				t.Errorf("IsEveryone = %v, want %v", r.IsEveryone, tt.input.IsEveryone)
			}
		})
	}
}

// --- Server CRUD validation tests ---

func TestCreateServer_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	tests := []struct {
		name    string
		srvName string
		ownerID string
		wantErr string
	}{
		{name: "empty name", srvName: "", ownerID: "1", wantErr: "server name is required"},
		{name: "empty owner", srvName: "Test", ownerID: "", wantErr: "owner ID is required"},
		{name: "invalid owner ID", srvName: "Test", ownerID: "abc", wantErr: "invalid owner ID format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateServer(context.Background(), tt.srvName, "", tt.ownerID)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGetServer_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		{name: "empty ID", id: "", wantErr: "server ID is required"},
		{name: "invalid ID", id: "abc", wantErr: "invalid server ID format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetServer(context.Background(), tt.id)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGetUserServers_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.GetUserServers(context.Background(), "")
	if err == nil || err.Error() != "user ID is required" {
		t.Errorf("got %v, want 'user ID is required'", err)
	}
	_, err = svc.GetUserServers(context.Background(), "abc")
	if err == nil || err.Error() != "invalid user ID format" {
		t.Errorf("got %v, want 'invalid user ID format'", err)
	}
}

func TestUpdateServer_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	tests := []struct {
		name    string
		id      string
		srvName string
		desc    string
		wantErr string
	}{
		{name: "empty ID", id: "", srvName: "Test", wantErr: "server ID is required"},
		{name: "empty name", id: "1", srvName: "", wantErr: "server name is required"},
		{name: "description too long", id: "1", srvName: "Test", desc: string(make([]byte, 201)), wantErr: "description must be 200 characters or fewer"},
		{name: "invalid ID", id: "abc", srvName: "Test", wantErr: "invalid server ID format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateServer(context.Background(), tt.id, tt.srvName, "", tt.desc, false)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDeleteServer_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	err := svc.DeleteServer(context.Background(), "")
	if err == nil || err.Error() != "server ID is required" {
		t.Errorf("got %v, want 'server ID is required'", err)
	}
	err = svc.DeleteServer(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID format" {
		t.Errorf("got %v, want 'invalid server ID format'", err)
	}
}

// --- Member validation tests ---

func TestAddMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	tests := []struct {
		name     string
		serverID string
		userID   string
		wantErr  string
	}{
		{name: "empty server", serverID: "", userID: "1", wantErr: "server ID is required"},
		{name: "empty user", serverID: "1", userID: "", wantErr: "user ID is required"},
		{name: "invalid server", serverID: "abc", userID: "1", wantErr: "invalid server ID format"},
		{name: "invalid user", serverID: "1", userID: "abc", wantErr: "invalid user ID format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.AddMember(context.Background(), tt.serverID, tt.userID, "")
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRemoveMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	tests := []struct {
		name     string
		serverID string
		userID   string
		wantErr  string
	}{
		{name: "empty server", serverID: "", userID: "1", wantErr: "server ID is required"},
		{name: "empty user", serverID: "1", userID: "", wantErr: "user ID is required"},
		{name: "invalid server", serverID: "abc", userID: "1", wantErr: "invalid server ID format"},
		{name: "invalid user", serverID: "1", userID: "abc", wantErr: "invalid user ID format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.RemoveMember(context.Background(), tt.serverID, tt.userID)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestKickMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	err := svc.KickMember(context.Background(), "", "1", 1, "admin")
	if err == nil || err.Error() != "server ID is required" {
		t.Errorf("got %v, want 'server ID is required'", err)
	}
	err = svc.KickMember(context.Background(), "1", "", 1, "admin")
	if err == nil || err.Error() != "user ID is required" {
		t.Errorf("got %v, want 'user ID is required'", err)
	}
}

func TestBanMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	err := svc.BanMember(context.Background(), "", "1", 1, "admin", "reason")
	if err == nil || err.Error() != "server ID is required" {
		t.Errorf("got %v, want 'server ID is required'", err)
	}
	err = svc.BanMember(context.Background(), "1", "", 1, "admin", "reason")
	if err == nil || err.Error() != "user ID is required" {
		t.Errorf("got %v, want 'user ID is required'", err)
	}
}

func TestGetMembers_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.GetMembers(context.Background(), "")
	if err == nil || err.Error() != "server ID is required" {
		t.Errorf("got %v, want 'server ID is required'", err)
	}
	_, err = svc.GetMembers(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID format" {
		t.Errorf("got %v, want 'invalid server ID format'", err)
	}
}

// --- Invite validation tests ---

func TestCreateInvite_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	tests := []struct {
		name      string
		serverID  string
		createdBy string
		wantErr   string
	}{
		{name: "empty server", serverID: "", createdBy: "1", wantErr: "server ID is required"},
		{name: "empty created by", serverID: "1", createdBy: "", wantErr: "created by is required"},
		{name: "invalid server", serverID: "abc", createdBy: "1", wantErr: "invalid server ID format"},
		{name: "invalid created by", serverID: "1", createdBy: "abc", wantErr: "invalid created by format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateInvite(context.Background(), tt.serverID, tt.createdBy, nil, nil, "user")
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGetInviteByCode_Empty(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.GetInviteByCode(context.Background(), "")
	if err == nil || err.Error() != "invite code is required" {
		t.Errorf("got %v, want 'invite code is required'", err)
	}
}

func TestGetServerByInviteCode_Empty(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.GetServerByInviteCode(context.Background(), "")
	if err == nil || err.Error() != "invite code is required" {
		t.Errorf("got %v, want 'invite code is required'", err)
	}
}

func TestJoinServerByInvite_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.JoinServerByInvite(context.Background(), "", "1")
	if err == nil || err.Error() != "invite code is required" {
		t.Errorf("got %v, want 'invite code is required'", err)
	}
	_, err = svc.JoinServerByInvite(context.Background(), "abc123", "")
	if err == nil || err.Error() != "user ID is required" {
		t.Errorf("got %v, want 'user ID is required'", err)
	}
}

func TestJoinServerByVanityURL_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.JoinServerByVanityURL(context.Background(), "", "1")
	if err == nil || err.Error() != "vanity URL is required" {
		t.Errorf("got %v, want 'vanity URL is required'", err)
	}
	_, err = svc.JoinServerByVanityURL(context.Background(), "my-server", "")
	if err == nil || err.Error() != "user ID is required" {
		t.Errorf("got %v, want 'user ID is required'", err)
	}
}

func TestSetVanityURL_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.SetVanityURL(context.Background(), "", "slug", 1, "admin")
	if err == nil || err.Error() != "server ID is required" {
		t.Errorf("got %v, want 'server ID is required'", err)
	}
	_, err = svc.SetVanityURL(context.Background(), "abc", "slug", 1, "admin")
	if err == nil || err.Error() != "invalid server ID format" {
		t.Errorf("got %v, want 'invalid server ID format'", err)
	}
}

// --- Role validation tests ---

func TestGetServerRoles_InvalidID(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.GetServerRoles(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

func TestCreateServerRole_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.CreateServerRole(context.Background(), "abc", "Admin", "#ff0000", 0, 1, "admin")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	_, err = svc.CreateServerRole(context.Background(), "1", "", "#ff0000", 0, 1, "admin")
	if err == nil || err.Error() != "role name is required" {
		t.Errorf("got %v, want 'role name is required'", err)
	}
}

func TestDeleteServerRole_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	err := svc.DeleteServerRole(context.Background(), "abc", "1", 1, "admin")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	err = svc.DeleteServerRole(context.Background(), "1", "abc", 1, "admin")
	if err == nil || err.Error() != "invalid role ID" {
		t.Errorf("got %v, want 'invalid role ID'", err)
	}
}

func TestUpdateServerRole_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.UpdateServerRole(context.Background(), "abc", "1", "Admin", "#ff0000", 0, false, 0, 1, "admin")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	_, err = svc.UpdateServerRole(context.Background(), "1", "abc", "Admin", "#ff0000", 0, false, 0, 1, "admin")
	if err == nil || err.Error() != "invalid role ID" {
		t.Errorf("got %v, want 'invalid role ID'", err)
	}
}

func TestGetMemberRoles_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.GetMemberRoles(context.Background(), "abc", "1")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	_, err = svc.GetMemberRoles(context.Background(), "1", "abc")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
}

func TestAssignRoleToMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	err := svc.AssignRoleToMember(context.Background(), "abc", "1", "1", 1, "admin")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	err = svc.AssignRoleToMember(context.Background(), "1", "abc", "1", 1, "admin")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
	err = svc.AssignRoleToMember(context.Background(), "1", "1", "abc", 1, "admin")
	if err == nil || err.Error() != "invalid role ID" {
		t.Errorf("got %v, want 'invalid role ID'", err)
	}
}

func TestRemoveRoleFromMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	err := svc.RemoveRoleFromMember(context.Background(), "abc", "1", "1", 1, "admin")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	err = svc.RemoveRoleFromMember(context.Background(), "1", "abc", "1", 1, "admin")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
	err = svc.RemoveRoleFromMember(context.Background(), "1", "1", "abc", 1, "admin")
	if err == nil || err.Error() != "invalid role ID" {
		t.Errorf("got %v, want 'invalid role ID'", err)
	}
}

func TestSetServerCategories_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)

	_, err := svc.SetServerCategories(context.Background(), "1", []int64{1, 2, 3, 4})
	if err == nil || err.Error() != "maximum 3 categories allowed" {
		t.Errorf("got %v, want 'maximum 3 categories allowed'", err)
	}
	_, err = svc.SetServerCategories(context.Background(), "abc", []int64{1})
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

func TestGetServerCategoryAssignments_InvalidID(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.GetServerCategoryAssignments(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

// --- Sentinel errors ---

func TestSentinelErrors(t *testing.T) {
	checks := []struct {
		err  error
		want string
	}{
		{ErrServerNotFound, "server not found"},
		{ErrMemberNotFound, "member not found"},
		{ErrInviteNotFound, "invite not found"},
		{ErrBanned, "you are banned from this server"},
		{ErrNotMember, "not a member of this server"},
		{ErrOwnerOnly, "only the server owner can set the vanity URL"},
		{ErrForbidden, "forbidden"},
	}
	for _, c := range checks {
		if c.err.Error() != c.want {
			t.Errorf("sentinel error = %q, want %q", c.err.Error(), c.want)
		}
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	sentinels := []error{
		ErrServerNotFound,
		ErrMemberNotFound,
		ErrInviteNotFound,
		ErrBanned,
		ErrNotMember,
		ErrOwnerOnly,
		ErrForbidden,
	}
	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("%q should not match %q", sentinels[i], sentinels[j])
			}
		}
	}
}

// --- ListBans / UnbanMember / GetServerInvites / RevokeInvite / GetInviteMembers ID validation ---

func TestListBans_InvalidID(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.ListBans(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

func TestUnbanMember_Validation(t *testing.T) {
	svc := NewServerService(nil, nil)
	err := svc.UnbanMember(context.Background(), "abc", "1", 1, "admin")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
	err = svc.UnbanMember(context.Background(), "1", "abc", 1, "admin")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
}

func TestGetServerInvites_InvalidID(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.GetServerInvites(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

func TestGetMembersWithRoles_InvalidID(t *testing.T) {
	svc := NewServerService(nil, nil)
	_, err := svc.GetMembersWithRoles(context.Background(), "abc")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

// TestInvalidateMembership_DropsCache verifies that invalidateMembership drops
// the cached membership entry and any per-user perm entries for that server.
func TestInvalidateMembership_DropsCache(t *testing.T) {
	svc := NewServerService(nil, nil)
	mc := cache.NewMembershipCache(30 * time.Second)
	svc.SetMemberCache(mc)

	const sID, uID int64 = 42, 7
	mc.SetMember(sID, uID, true)
	mc.SetPerm(sID, uID, 100, 1, true)

	if isMember, ok := mc.GetMember(sID, uID); !ok || !isMember {
		t.Fatal("precondition: membership must be cached before invalidate")
	}
	if allowed, ok := mc.GetPerm(sID, uID, 100, 1); !ok || !allowed {
		t.Fatal("precondition: perm must be cached before invalidate")
	}

	svc.invalidateMembership("42", "7", sID, uID)

	if _, ok := mc.GetMember(sID, uID); ok {
		t.Error("membership entry should be invalidated")
	}
	if _, ok := mc.GetPerm(sID, uID, 100, 1); ok {
		t.Error("perm entry should be invalidated")
	}
}

// TestInvalidateMembership_NilHubIsSafe verifies that invalidateMembership
// is a no-op on the hub side when no hub has been wired (e.g., unit tests).
func TestInvalidateMembership_NilHubIsSafe(t *testing.T) {
	svc := NewServerService(nil, nil)
	mc := cache.NewMembershipCache(30 * time.Second)
	svc.SetMemberCache(mc)
	mc.SetMember(1, 2, true)

	// Must not panic when hub is nil.
	svc.invalidateMembership("1", "2", 1, 2)

	if _, ok := mc.GetMember(1, 2); ok {
		t.Error("cache entry must still be dropped even without a hub")
	}
}

// TestInvalidateMembership_NilCacheLogsWarning verifies that the helper
// tolerates a nil cache (e.g., early startup, legacy wiring).
func TestInvalidateMembership_NilCacheLogsWarning(t *testing.T) {
	svc := NewServerService(nil, nil)
	// Must not panic when memberCache is nil and hub is nil.
	svc.invalidateMembership("1", "2", 1, 2)
}

// TestInvalidateMembership_UnsubscribesFromServer verifies that
// invalidateMembership drops the user's WS subscriptions to the kicked
// server (both the virtual "server:{id}" channel and numeric channels
// resolved to that server), while leaving DM subscriptions intact.
func TestInvalidateMembership_UnsubscribesFromServer(t *testing.T) {
	svc := NewServerService(nil, nil)
	hub := ws.NewHub()
	hub.SetChannelServerResolver(func(channelID string) (string, bool) {
		if channelID == "101" {
			return "42", true
		}
		return "", false
	})
	svc.SetHub(hub)

	client := ws.NewTestClient(hub, "7")
	hub.RegisterClient(client)
	hub.SubscribeToChannel("server:42", client)
	hub.SubscribeToChannel("101", client)
	hub.SubscribeToChannel("dm:99", client)

	svc.invalidateMembership("42", "7", 42, 7)

	if hub.ClientSubscribed(client, "server:42") {
		t.Error("server:42 should be unsubscribed")
	}
	if hub.ClientSubscribed(client, "101") {
		t.Error("channel 101 (server 42) should be unsubscribed")
	}
	if !hub.ClientSubscribed(client, "dm:99") {
		t.Error("DM subscription must remain")
	}
}

