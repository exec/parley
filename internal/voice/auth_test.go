package voice

import (
	"context"
	"strconv"
	"testing"

	"parley/internal/db"
)

// authRepoFake is a hand-rolled stub implementing the small surface we need.
type authRepoFake struct {
	dmMembers     map[int64]map[int64]bool            // dmID -> userID -> isMember
	dmOwnerByID   map[int64]*int64                    // dmID -> *ownerUserID, nil if no owner
	dmIsGroupByID map[int64]bool                      // dmID -> is_group
	srvMember     map[int64]map[int64]*db.ServerMember // serverID -> userID -> member
	srvOwner      map[int64]int64                     // serverID -> ownerID
	chByID        map[int64]*db.Channel               // channelID -> channel
}

func (r *authRepoFake) IsDmMember(_ context.Context, dmID, uid int64) (bool, error) {
	return r.dmMembers[dmID][uid], nil
}
func (r *authRepoFake) GetDmChannelByID(_ context.Context, dmID int64) (*db.DmChannel, error) {
	return &db.DmChannel{ID: dmID, IsGroup: r.dmIsGroupByID[dmID], OwnerUserID: r.dmOwnerByID[dmID]}, nil
}
func (r *authRepoFake) GetMember(_ context.Context, serverID, uid int64) (*db.ServerMember, error) {
	return r.srvMember[serverID][uid], nil
}
func (r *authRepoFake) GetServerByID(_ context.Context, serverID int64) (*db.Server, error) {
	return &db.Server{ID: serverID, OwnerID: r.srvOwner[serverID]}, nil
}
func (r *authRepoFake) GetChannelByID(_ context.Context, channelID int64) (*db.Channel, error) {
	return r.chByID[channelID], nil
}
func (r *authRepoFake) GetDmMembers(_ context.Context, dmID int64) ([]db.DmChannelMember, error) {
	out := []db.DmChannelMember{}
	for uid := range r.dmMembers[dmID] {
		out = append(out, db.DmChannelMember{DmChannelID: dmID, UserID: uid})
	}
	return out, nil
}
func (r *authRepoFake) GetUserByID(_ context.Context, id int64) (*db.User, error) {
	return &db.User{ID: id, Username: "user" + strconv.FormatInt(id, 10)}, nil
}
func (r *authRepoFake) GetUserDmChannels(_ context.Context, _ int64) ([]db.DmChannel, error) {
	return nil, nil
}

func ptrInt64(v int64) *int64 { return &v }

func TestAuthorizeJoin_DM(t *testing.T) {
	repo := &authRepoFake{
		dmMembers: map[int64]map[int64]bool{
			10: {1: true, 2: true},
		},
	}
	a := &Authorizer{repo: repo}
	cases := []struct {
		vc   VirtualChannel
		uid  int64
		want bool
	}{
		{VirtualChannel{Kind: KindDM, ID: 10}, 1, true},
		{VirtualChannel{Kind: KindDM, ID: 10}, 2, true},
		{VirtualChannel{Kind: KindDM, ID: 10}, 3, false},
		{VirtualChannel{Kind: KindDM, ID: 99}, 1, false},
	}
	for _, c := range cases {
		ok, err := a.AuthorizeJoin(context.Background(), c.vc, c.uid)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ok != c.want {
			t.Errorf("AuthorizeJoin(%v,%d) = %v, want %v", c.vc, c.uid, ok, c.want)
		}
	}
}

func TestAuthorizeMute_DM_GC_OwnerOnly(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true, 3: true}},
		dmOwnerByID:   map[int64]*int64{10: ptrInt64(1)},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	a := &Authorizer{repo: repo}
	vc := VirtualChannel{Kind: KindDM, ID: 10}

	// owner (1) muting any other GC member is allowed
	ok, err := a.AuthorizeMute(context.Background(), vc, 1, 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok {
		t.Fatal("owner should be allowed to mute member")
	}
	// non-owner (2) muting member (3) is denied
	ok, err = a.AuthorizeMute(context.Background(), vc, 2, 3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("non-owner must not mute")
	}
	// owner cannot mute themselves
	ok, err = a.AuthorizeMute(context.Background(), vc, 1, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("self-mute via force-mute is nonsense")
	}
}

func TestAuthorizeMute_DM_OneOnOne_Denied(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmIsGroupByID: map[int64]bool{10: false},
	}
	a := &Authorizer{repo: repo}
	vc := VirtualChannel{Kind: KindDM, ID: 10}
	ok, err := a.AuthorizeMute(context.Background(), vc, 1, 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("force-mute is not allowed in 1:1 DM")
	}
}

func TestAuthorizeMute_DM_GC_NoOwner_Denied(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmOwnerByID:   map[int64]*int64{10: nil}, // owner absent
		dmIsGroupByID: map[int64]bool{10: true},
	}
	a := &Authorizer{repo: repo}
	vc := VirtualChannel{Kind: KindDM, ID: 10}
	if ok, _ := a.AuthorizeMute(context.Background(), vc, 1, 2); ok {
		t.Fatal("must deny when GC has no owner")
	}
}

func TestAuthorizeKick_DM_GC_OwnerOnly(t *testing.T) {
	repo := &authRepoFake{
		dmMembers:     map[int64]map[int64]bool{10: {1: true, 2: true}},
		dmOwnerByID:   map[int64]*int64{10: ptrInt64(1)},
		dmIsGroupByID: map[int64]bool{10: true},
	}
	a := &Authorizer{repo: repo}
	vc := VirtualChannel{Kind: KindDM, ID: 10}
	if ok, err := a.AuthorizeKick(context.Background(), vc, 1, 2); err != nil || !ok {
		t.Fatalf("owner kick: ok=%v err=%v", ok, err)
	}
	if ok, _ := a.AuthorizeKick(context.Background(), vc, 2, 1); ok {
		t.Fatal("non-owner kick: must deny")
	}
}
