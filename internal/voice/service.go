package voice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// Participant holds info about a user currently in a voice channel.
type Participant struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
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
	jti := make([]byte, 8)
	rand.Read(jti)

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": s.apiKey,
		"sub": userID,
		"iat": now.Unix(),
		"exp": now.Add(6 * time.Hour).Unix(),
		"jti": hex.EncodeToString(jti),
		"name": username,
		"video": map[string]interface{}{
			"room":           channelID,
			"roomJoin":       true,
			"canPublish":     true,
			"canSubscribe":   true,
			"canPublishData": true,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.apiSecret))
}

func (s *Service) ServerURL() string { return s.serverURL }

// presenceKey returns the Redis key for a voice channel's participant set.
func presenceKey(channelID string) string {
	return fmt.Sprintf("voice:%s", channelID)
}

// Join records a participant joining a voice channel.
func (s *Service) Join(ctx context.Context, channelID, userID, username string) error {
	if s.rdb == nil {
		return nil
	}
	p := Participant{UserID: userID, Username: username}
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
