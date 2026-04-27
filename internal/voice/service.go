package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	lkauth "github.com/livekit/protocol/auth"
	"github.com/redis/go-redis/v9"
)

// Participant holds info about a user currently in a voice channel.
type Participant struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Service handles LiveKit token generation and Redis voice presence.
type Service struct {
	apiKey    string
	apiSecret string
	serverURL string
	rdb       *redis.Client
}

func NewService(rdb *redis.Client) *Service {
	return &Service{
		apiKey:    os.Getenv("LIVEKIT_API_KEY"),
		apiSecret: os.Getenv("LIVEKIT_API_SECRET"),
		serverURL: os.Getenv("LIVEKIT_URL"),
		rdb:       rdb,
	}
}

func (s *Service) Configured() bool {
	return s.apiKey != "" && s.apiSecret != "" && s.serverURL != ""
}

// IssueToken generates a LiveKit JWT for the given user to join the given room (channel ID).
func (s *Service) IssueToken(userID, username, channelID string) (string, error) {
	canPublish := true
	canSubscribe := true
	canPublishData := true

	at := lkauth.NewAccessToken(s.apiKey, s.apiSecret).
		SetIdentity(userID).
		SetName(username).
		SetValidFor(6 * time.Hour).
		SetVideoGrant(&lkauth.VideoGrant{
			Room:           channelID,
			RoomJoin:       true,
			CanPublish:     &canPublish,
			CanSubscribe:   &canSubscribe,
			CanPublishData: &canPublishData,
		})

	return at.ToJWT()
}

func (s *Service) ServerURL() string { return s.serverURL }

// voiceHeartbeatTTL is the expiry for per-user voice heartbeat keys.
// In production, clients should refresh this with periodic heartbeats.
const voiceHeartbeatTTL = 30 * time.Second

// presenceKey returns the Redis key for a voice channel's participant set.
func presenceKey(channelID string) string {
	return fmt.Sprintf("voice:%s", channelID)
}

// heartbeatKey returns the Redis key for a per-user voice heartbeat.
func heartbeatKey(channelID, userID string) string {
	return fmt.Sprintf("voice:heartbeat:%s:%s", channelID, userID)
}

// startedAtKey is the call-start timestamp key for the room.
func startedAtKey(channelID string) string {
	return fmt.Sprintf("voice:%s:started_at", channelID)
}

// callEndedLockKey gates duplicate call_ended emissions on the same call instance.
func callEndedLockKey(channelID, startedAt string) string {
	return fmt.Sprintf("call_ended:%s:%s", channelID, startedAt)
}

// ClaimCallStarted atomically reserves the right to emit `call_started` for a
// voice channel. Two concurrent /call/start requests would otherwise both see
// an empty room and double-emit; this collapses them by piggy-backing on the
// existing started_at key (the same one Join SET NX's on first joiner). The
// caller that wins the SET NX should emit; everyone else skips.
//
// Returns true iff the caller should emit `call_started`.
func (s *Service) ClaimCallStarted(ctx context.Context, channelID string, startedAtMs int64) (bool, error) {
	if s.rdb == nil {
		// Without Redis we have no way to dedup; emit and accept the duplicate.
		return true, nil
	}
	return s.rdb.SetNX(ctx, startedAtKey(channelID), strconv.FormatInt(startedAtMs, 10), 6*time.Hour).Result()
}

// Join records a participant joining a voice channel. The first joiner
// atomically stamps voice:{channelID}:started_at with the current ms time
// (with a 6h fallback TTL in case Leave never fires). Returns whether this
// was a NEW presence row (true) or an idempotent re-join (false) — handlers
// use that signal to suppress duplicate VOICE_STATE_UPDATE broadcasts.
// NOTE: clients must periodically call RefreshHeartbeat to keep the key alive.
func (s *Service) Join(ctx context.Context, channelID, userID, username, avatarURL string) (bool, error) {
	if s.rdb == nil {
		return false, nil
	}
	p := Participant{UserID: userID, Username: username, AvatarURL: avatarURL}
	b, _ := json.Marshal(p)
	added, err := s.rdb.HSet(ctx, presenceKey(channelID), userID, string(b)).Result()
	if err != nil {
		return false, err
	}
	if err := s.rdb.Set(ctx, heartbeatKey(channelID, userID), "1", voiceHeartbeatTTL).Err(); err != nil {
		return false, err
	}
	// First-joiner wins the SET NX. Subsequent joiners no-op.
	startedAtMs := time.Now().UnixMilli()
	s.rdb.SetNX(ctx, startedAtKey(channelID), strconv.FormatInt(startedAtMs, 10), 6*time.Hour)
	return added > 0, nil
}

// EndIfEmpty atomically checks whether the room is empty and, if so, removes
// the started_at key and acquires a 60s NX lock to single-emit call_ended.
// Returns (startedAtMs, true, nil) iff the caller should emit call_ended.
// Returns (0, false, nil) when the room is non-empty or another emitter has
// already claimed the lock.
func (s *Service) EndIfEmpty(ctx context.Context, channelID string) (int64, bool, error) {
	if s.rdb == nil {
		return 0, false, nil
	}
	remaining, err := s.rdb.HLen(ctx, presenceKey(channelID)).Result()
	if err != nil {
		return 0, false, err
	}
	if remaining > 0 {
		return 0, false, nil
	}
	startedAtStr, err := s.rdb.GetDel(ctx, startedAtKey(channelID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil // already cleaned up
	}
	if err != nil {
		return 0, false, err // real failure, surface to caller
	}
	got, err := s.rdb.SetNX(ctx, callEndedLockKey(channelID, startedAtStr), "1", 60*time.Second).Result()
	if err != nil || !got {
		return 0, false, err
	}
	startedAtMs, parseErr := strconv.ParseInt(startedAtStr, 10, 64)
	if parseErr != nil {
		return 0, false, parseErr
	}
	s.rdb.Del(ctx, activityKey(channelID))
	return startedAtMs, true, nil
}

// Leave removes a participant from a voice channel.
func (s *Service) Leave(ctx context.Context, channelID, userID string) error {
	if s.rdb == nil {
		return nil
	}
	s.rdb.Del(ctx, heartbeatKey(channelID, userID))
	return s.rdb.HDel(ctx, presenceKey(channelID), userID).Err()
}

// RefreshHeartbeat resets the TTL on a user's voice heartbeat key.
// Clients should call this periodically (e.g. every 15s) to signal liveness.
func (s *Service) RefreshHeartbeat(ctx context.Context, channelID, userID string) error {
	if s.rdb == nil {
		return nil
	}
	return s.rdb.Set(ctx, heartbeatKey(channelID, userID), "1", voiceHeartbeatTTL).Err()
}

// Participants returns all current participants in a voice channel. Stale
// entries (presence hash members whose heartbeat key has expired) are evicted
// inline before returning.
func (s *Service) Participants(ctx context.Context, channelID string) ([]Participant, error) {
	if s.rdb == nil {
		return []Participant{}, nil
	}
	res, err := s.rdb.HGetAll(ctx, presenceKey(channelID)).Result()
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return []Participant{}, nil
	}
	// Pipeline the per-user heartbeat EXISTS into one round-trip instead of
	// O(users) sequential calls.
	uids := make([]string, 0, len(res))
	pipe := s.rdb.Pipeline()
	hbCmds := make(map[string]*redis.IntCmd, len(res))
	for uid := range res {
		uids = append(uids, uid)
		hbCmds[uid] = pipe.Exists(ctx, heartbeatKey(channelID, uid))
	}
	_, _ = pipe.Exec(ctx)
	out := make([]Participant, 0, len(res))
	var stale []string
	for _, uid := range uids {
		if hbCmds[uid].Val() == 0 {
			stale = append(stale, uid)
			continue
		}
		var p Participant
		if err := json.Unmarshal([]byte(res[uid]), &p); err != nil {
			continue
		}
		out = append(out, p)
	}
	if len(stale) > 0 {
		s.rdb.HDel(ctx, presenceKey(channelID), stale...)
	}
	return out, nil
}

// IsParticipant returns true if the user is currently in the voice channel.
// Lazy-evicts the presence entry if the heartbeat has expired.
func (s *Service) IsParticipant(ctx context.Context, channelID, userID string) (bool, error) {
	if s.rdb == nil {
		log.Printf("voice: IsParticipant called but Redis is not configured; returning false")
		return false, nil
	}
	exists, err := s.rdb.HExists(ctx, presenceKey(channelID), userID).Result()
	if err != nil || !exists {
		return exists, err
	}
	hbExists, _ := s.rdb.Exists(ctx, heartbeatKey(channelID, userID)).Result()
	if hbExists == 0 {
		s.rdb.HDel(ctx, presenceKey(channelID), userID)
		return false, nil
	}
	return true, nil
}

// EvictedParticipant identifies a presence entry that the sweeper removed.
// The Topic is left to the caller (sweep wiring in cmd/api) which has DB
// access for server-VC topic resolution.
type EvictedParticipant struct {
	ChannelID string
	UserID    string
}

// SweepStale scans every presence hash and removes members whose heartbeat
// keys have expired. Returns the (channelID, userID) pairs that were removed
// so the caller can broadcast leave events. Designed to be called on a
// timer (~15s) from a single goroutine; safe to call concurrently with
// Join/Leave/Refresh because each HDel is atomic.
func (s *Service) SweepStale(ctx context.Context) ([]EvictedParticipant, error) {
	if s.rdb == nil {
		return nil, nil
	}
	var evicted []EvictedParticipant
	var cursor uint64
	for {
		keys, c, err := s.rdb.Scan(ctx, cursor, "voice:*", 200).Result()
		if err != nil {
			return evicted, err
		}
		cursor = c

		// Batch HKeys across all presence keys in this scan page.
		type chanEntry struct {
			channelID string
			key       string
			cmd       *redis.StringSliceCmd
		}
		var entries []chanEntry
		hkPipe := s.rdb.Pipeline()
		for _, key := range keys {
			// Presence keys are exactly "voice:s:N" or "voice:dm:N" — two colons.
			// Heartbeat ("voice:heartbeat:..."), activity ("voice:s:N:activity"),
			// started_at, and call_ended:* keys all have more colons.
			if strings.Count(key, ":") != 2 {
				continue
			}
			channelID := strings.TrimPrefix(key, "voice:")
			entries = append(entries, chanEntry{
				channelID: channelID,
				key:       key,
				cmd:       hkPipe.HKeys(ctx, key),
			})
		}
		if len(entries) == 0 {
			if cursor == 0 {
				break
			}
			continue
		}
		_, _ = hkPipe.Exec(ctx)

		// Now batch EXISTS for every (channel, user) pair across all channels.
		type hbRef struct {
			channelID string
			key       string
			uid       string
			cmd       *redis.IntCmd
		}
		var refs []hbRef
		hbPipe := s.rdb.Pipeline()
		for _, e := range entries {
			users, err := e.cmd.Result()
			if err != nil {
				continue
			}
			for _, uid := range users {
				refs = append(refs, hbRef{
					channelID: e.channelID,
					key:       e.key,
					uid:       uid,
					cmd:       hbPipe.Exists(ctx, heartbeatKey(e.channelID, uid)),
				})
			}
		}
		if len(refs) > 0 {
			_, _ = hbPipe.Exec(ctx)
		}

		// Pipeline per-uid HDels so we can match each result back to its uid.
		// Per-uid HDel preserves the original race semantics: we only emit an
		// EvictedParticipant when *we* removed the field (removed > 0), which
		// means a concurrent Leave that won the race correctly does not produce
		// a duplicate broadcast.
		type delRef struct {
			channelID string
			uid       string
			cmd       *redis.IntCmd
		}
		var delRefs []delRef
		delPipe := s.rdb.Pipeline()
		for _, r := range refs {
			if r.cmd.Val() != 0 {
				continue
			}
			delRefs = append(delRefs, delRef{
				channelID: r.channelID,
				uid:       r.uid,
				cmd:       delPipe.HDel(ctx, r.key, r.uid),
			})
		}
		if len(delRefs) > 0 {
			_, _ = delPipe.Exec(ctx)
			for _, dr := range delRefs {
				if dr.cmd.Val() > 0 {
					evicted = append(evicted, EvictedParticipant{ChannelID: dr.channelID, UserID: dr.uid})
				}
			}
		}

		if cursor == 0 {
			break
		}
	}
	return evicted, nil
}

// Activity is the per-call active activity record stored in Redis.
type Activity struct {
	Type        string          `json:"type"`
	StartedBy   int64           `json:"started_by,string"`
	StartedAtMs int64           `json:"started_at_ms"`
	Params      json.RawMessage `json:"params,omitempty"`
}

func activityKey(channelID string) string {
	return fmt.Sprintf("voice:%s:activity", channelID)
}

// StartActivity records or replaces the active activity for a call.
func (s *Service) StartActivity(ctx context.Context, channelID, activityType string, startedBy int64, params json.RawMessage) error {
	if s.rdb == nil {
		return nil
	}
	a := Activity{
		Type:        activityType,
		StartedBy:   startedBy,
		StartedAtMs: time.Now().UnixMilli(),
		Params:      params,
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, activityKey(channelID), b, 6*time.Hour).Err()
}

// GetActivity returns the active activity for a call, or nil if none.
func (s *Service) GetActivity(ctx context.Context, channelID string) (*Activity, error) {
	if s.rdb == nil {
		return nil, nil
	}
	raw, err := s.rdb.Get(ctx, activityKey(channelID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a Activity
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// EndActivity removes the activity record. Idempotent.
func (s *Service) EndActivity(ctx context.Context, channelID string) error {
	if s.rdb == nil {
		return nil
	}
	return s.rdb.Del(ctx, activityKey(channelID)).Err()
}
