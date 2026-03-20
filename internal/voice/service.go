package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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

// presenceKey returns the Redis key for a voice channel's participant set.
func presenceKey(channelID string) string {
	return fmt.Sprintf("voice:%s", channelID)
}

// Join records a participant joining a voice channel.
func (s *Service) Join(ctx context.Context, channelID, userID, username, avatarURL string) error {
	if s.rdb == nil {
		return nil
	}
	p := Participant{UserID: userID, Username: username, AvatarURL: avatarURL}
	b, _ := json.Marshal(p)
	return s.rdb.HSet(ctx, presenceKey(channelID), userID, string(b)).Err()
}

// Leave removes a participant from a voice channel.
func (s *Service) Leave(ctx context.Context, channelID, userID string) error {
	if s.rdb == nil {
		return nil
	}
	return s.rdb.HDel(ctx, presenceKey(channelID), userID).Err()
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
		log.Printf("soundboard: IsParticipant called but Redis is not configured; returning false")
		return false, nil
	}
	exists, err := s.rdb.HExists(ctx, presenceKey(channelID), userID).Result()
	return exists, err
}
