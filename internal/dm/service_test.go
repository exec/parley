package dm

import (
	"context"
	"encoding/json"
	"testing"

	"parley/internal/db"
	ws "parley/internal/websocket"
)

type fakeDmRepo struct {
	members  map[int64]map[int64]bool
	channels map[int64]*db.DmChannel
	sysMsgs  map[int64][]*db.DmMessage
	seq      int64
}

func (r *fakeDmRepo) IsDmMember(_ context.Context, dmID, uid int64) (bool, error) {
	return r.members[dmID][uid], nil
}
func (r *fakeDmRepo) InsertSystemMessage(_ context.Context, channelID, actorID int64, eventJSON []byte) (*db.DmMessage, error) {
	r.seq++
	raw := json.RawMessage(eventJSON)
	msg := &db.DmMessage{ID: r.seq, DmChannelID: channelID, AuthorID: actorID, SystemEvent: &raw}
	r.sysMsgs[channelID] = append(r.sysMsgs[channelID], msg)
	return msg, nil
}
func (r *fakeDmRepo) GetDmChannelByID(_ context.Context, dmID int64) (*db.DmChannel, error) {
	return r.channels[dmID], nil
}
func (r *fakeDmRepo) GetDmMembers(_ context.Context, dmID int64) ([]db.DmChannelMember, error) {
	var out []db.DmChannelMember
	for uid := range r.members[dmID] {
		out = append(out, db.DmChannelMember{UserID: uid, DmChannelID: dmID})
	}
	return out, nil
}
func (r *fakeDmRepo) GetOrCreateDmChannel(_ context.Context, _, _ int64) (*db.DmChannel, error) {
	return nil, nil
}
func (r *fakeDmRepo) CreateGroupChannel(_ context.Context, _ int64, _ string, _ []int64) (*db.DmChannel, error) {
	return nil, nil
}
func (r *fakeDmRepo) AddDmMember(_ context.Context, _, _ int64) error    { return nil }
func (r *fakeDmRepo) RemoveDmMember(_ context.Context, _, _ int64) error { return nil }
func (r *fakeDmRepo) TransferDmGroupOwnership(_ context.Context, _ int64, _ *int64) error {
	return nil
}
func (r *fakeDmRepo) UpdateDmGroupName(_ context.Context, _ int64, _ string) error { return nil }
func (r *fakeDmRepo) UpdateDmGroupAvatar(_ context.Context, _ int64, _ *string) error {
	return nil
}
func (r *fakeDmRepo) GetUserDisplayName(_ context.Context, userID int64) (string, error) {
	return "testuser", nil
}

func (r *fakeDmRepo) SeedDmChannel(id int64, isGroup bool, ownerID int64, members []int64) {
	var owner *int64
	if ownerID != 0 {
		owner = &ownerID
	}
	r.channels[id] = &db.DmChannel{ID: id, IsGroup: isGroup, OwnerUserID: owner}
	if r.members[id] == nil {
		r.members[id] = map[int64]bool{}
	}
	for _, u := range members {
		r.members[id][u] = true
	}
}
func (r *fakeDmRepo) LastSystemMessageFor(dmID int64) *db.DmMessage {
	msgs := r.sysMsgs[dmID]
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}

type recordedBroadcast struct {
	topic, eventType string
	payload          []byte
}

type fakeHub struct {
	broadcasts []recordedBroadcast
}

func (h *fakeHub) BroadcastToChannel(topic, eventType string, payload []byte) {
	h.broadcasts = append(h.broadcasts, recordedBroadcast{topic, eventType, payload})
}
func (h *fakeHub) SendToUser(_ string, _ string, _ []byte) error  { return nil }
func (h *fakeHub) SendBytesToUsers(_ []int64, _ string, _ []byte) {}
func (h *fakeHub) BroadcastedTo(topic, eventType string) bool {
	for _, b := range h.broadcasts {
		if b.topic == topic && b.eventType == eventType {
			return true
		}
	}
	return false
}

func newDmTestHarness(t *testing.T) (*fakeDmRepo, *fakeHub) {
	t.Helper()
	return &fakeDmRepo{
		members:  map[int64]map[int64]bool{},
		channels: map[int64]*db.DmChannel{},
		sysMsgs:  map[int64][]*db.DmMessage{},
	}, &fakeHub{}
}

func newTestService(repo *fakeDmRepo, hub *fakeHub) *Service {
	return &Service{repo: repo, hub: hub}
}

func TestEmitCallStarted_BroadcastsAndPersists(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := newTestService(repo, hub)

	const channelID int64 = 42
	const actorID int64 = 7
	repo.SeedDmChannel(channelID, false, 0, []int64{actorID, 8})

	if err := svc.EmitCallStarted(context.Background(), channelID, actorID, 1714000000000); err != nil {
		t.Fatalf("EmitCallStarted: %v", err)
	}
	got := repo.LastSystemMessageFor(channelID)
	if got == nil {
		t.Fatal("expected system message inserted")
	}
	var ev map[string]any
	if err := json.Unmarshal(*got.SystemEvent, &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if ev["type"] != "call_started" {
		t.Errorf("type = %v, want call_started", ev["type"])
	}
	if ev["actor_user_id"] != "7" {
		t.Errorf("actor_user_id = %v, want \"7\"", ev["actor_user_id"])
	}
	if ev["started_at_ms"] != float64(1714000000000) {
		t.Errorf("started_at_ms = %v", ev["started_at_ms"])
	}
	if !hub.BroadcastedTo("dm:42", ws.EventDmMessageCreate) {
		t.Error("expected broadcast on dm:42")
	}
}

func TestEmitCallEnded_DurationOnly(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := newTestService(repo, hub)
	const channelID int64 = 42
	const lastLeaver int64 = 7
	repo.SeedDmChannel(channelID, false, 0, []int64{1, 2})

	if err := svc.EmitCallEnded(context.Background(), channelID, lastLeaver, 145000, 1714000000000); err != nil {
		t.Fatalf("EmitCallEnded: %v", err)
	}
	got := repo.LastSystemMessageFor(channelID)
	var ev map[string]any
	json.Unmarshal(*got.SystemEvent, &ev)
	if ev["type"] != "call_ended" {
		t.Errorf("type = %v", ev["type"])
	}
	if ev["duration_ms"] != float64(145000) {
		t.Errorf("duration_ms = %v", ev["duration_ms"])
	}
	if got.AuthorID != lastLeaver {
		t.Errorf("author = %d, want %d", got.AuthorID, lastLeaver)
	}
	if !hub.BroadcastedTo("dm:42", ws.EventDmMessageCreate) {
		t.Error("expected broadcast")
	}
}

func TestEmitCallMissed_NoDecliner(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := newTestService(repo, hub)
	repo.SeedDmChannel(42, false, 0, []int64{1, 2})

	if err := svc.EmitCallMissed(context.Background(), 42, 1); err != nil {
		t.Fatalf("EmitCallMissed: %v", err)
	}
	var ev map[string]any
	json.Unmarshal(*repo.LastSystemMessageFor(42).SystemEvent, &ev)
	if ev["type"] != "call_missed" || ev["caller_user_id"] != "1" {
		t.Errorf("got %+v", ev)
	}
	if _, hasDecliner := ev["decliner_user_id"]; hasDecliner {
		t.Error("missed event must not carry decliner_user_id")
	}
	if !hub.BroadcastedTo("dm:42", ws.EventDmMessageCreate) {
		t.Error("expected broadcast on dm:42")
	}
}

func TestEmitCallDeclined_HasDecliner(t *testing.T) {
	repo, hub := newDmTestHarness(t)
	svc := newTestService(repo, hub)
	repo.SeedDmChannel(42, false, 0, []int64{1, 2})

	if err := svc.EmitCallDeclined(context.Background(), 42, 1, 2); err != nil {
		t.Fatalf("EmitCallDeclined: %v", err)
	}
	var ev map[string]any
	json.Unmarshal(*repo.LastSystemMessageFor(42).SystemEvent, &ev)
	if ev["type"] != "call_declined" || ev["caller_user_id"] != "1" || ev["decliner_user_id"] != "2" {
		t.Errorf("got %+v", ev)
	}
	if !hub.BroadcastedTo("dm:42", ws.EventDmMessageCreate) {
		t.Error("expected broadcast on dm:42")
	}
}
