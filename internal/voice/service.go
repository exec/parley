package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
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

// Join records a participant joining a voice channel. The first joiner
// atomically stamps voice:{channelID}:started_at with the current ms time
// (with a 6h fallback TTL in case Leave never fires).
// NOTE: clients must periodically call RefreshHeartbeat to keep the key alive.
func (s *Service) Join(ctx context.Context, channelID, userID, username, avatarURL string) error {
	if s.rdb == nil {
		return nil
	}
	p := Participant{UserID: userID, Username: username, AvatarURL: avatarURL}
	b, _ := json.Marshal(p)
	if err := s.rdb.HSet(ctx, presenceKey(channelID), userID, string(b)).Err(); err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, heartbeatKey(channelID, userID), "1", voiceHeartbeatTTL).Err(); err != nil {
		return err
	}
	// First-joiner wins the SET NX. Subsequent joiners no-op.
	startedAtMs := time.Now().UnixMilli()
	s.rdb.SetNX(ctx, startedAtKey(channelID), strconv.FormatInt(startedAtMs, 10), 6*time.Hour)
	return nil
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
	if err != nil || startedAtStr == "" {
		// already cleaned up by another caller
		return 0, false, nil
	}
	got, err := s.rdb.SetNX(ctx, callEndedLockKey(channelID, startedAtStr), "1", 60*time.Second).Result()
	if err != nil || !got {
		return 0, false, err
	}
	startedAtMs, parseErr := strconv.ParseInt(startedAtStr, 10, 64)
	if parseErr != nil {
		return 0, false, parseErr
	}
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

// Participants returns all current participants in a voice channel.
func (s *Service) Participants(ctx context.Context, channelID string) ([]Participant, error) {
	if s.rdb == nil {
		return []Participant{}, nil
	}
	res, err := s.rdb.HGetAll(ctx, presenceKey(channelID)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Participant, 0, len(res))
	for _, v := range res {
		var p Participant
		if err := json.Unmarshal([]byte(v), &p); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

// IsParticipant returns true if the user is currently in the voice channel.
func (s *Service) IsParticipant(ctx context.Context, channelID, userID string) (bool, error) {
	if s.rdb == nil {
		log.Printf("voice: IsParticipant called but Redis is not configured; returning false")
		return false, nil
	}
	exists, err := s.rdb.HExists(ctx, presenceKey(channelID), userID).Result()
	return exists, err
}
