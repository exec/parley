package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPClient is a thin wrapper around net/http for Parley's API.
type HTTPClient struct {
	base   string
	token  string
	secret string // X-Bench-Secret for provision endpoint
	client *http.Client
}

func NewHTTPClient(base, token, secret string) *HTTPClient {
	return &HTTPClient{
		base:   base,
		token:  token,
		secret: secret,
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *HTTPClient) WithToken(token string) *HTTPClient {
	clone := *c
	clone.token = token
	return &clone
}

// do executes a request and returns the response body bytes and status code.
func (c *HTTPClient) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.secret != "" {
		req.Header.Set("X-Bench-Secret", c.secret)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}
	return buf.Bytes(), resp.StatusCode, nil
}

// --- Auth ---

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  struct {
		ID int64 `json:"id,string"`
	} `json:"user"`
}

func (c *HTTPClient) Register(ctx context.Context, username, email, password string) (*AuthResponse, int, error) {
	body, status, err := c.do(ctx, http.MethodPost, "/api/auth/register", RegisterRequest{
		Username: username, Email: email, Password: password,
	})
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusCreated {
		return nil, status, nil
	}
	var resp AuthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, status, err
	}
	return &resp, status, nil
}

func (c *HTTPClient) Login(ctx context.Context, email, password string) (*AuthResponse, int, error) {
	body, status, err := c.do(ctx, http.MethodPost, "/api/auth/login", map[string]string{
		"email": email, "password": password,
	})
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return nil, status, nil
	}
	var resp AuthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, status, err
	}
	return &resp, status, nil
}

func (c *HTTPClient) GetWSTicket(ctx context.Context) (string, error) {
	body, status, err := c.do(ctx, http.MethodPost, "/api/auth/ws-ticket", nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("ws-ticket: status %d", status)
	}
	var resp struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.Ticket, nil
}

// --- Messages ---

type SendMessageRequest struct {
	Content string `json:"content"`
	Nonce   string `json:"nonce,omitempty"`
}

func (c *HTTPClient) SendMessage(ctx context.Context, channelID int64, content string) (int, error) {
	_, status, err := c.do(ctx, http.MethodPost,
		fmt.Sprintf("/api/channels/%d/messages", channelID),
		SendMessageRequest{Content: content},
	)
	return status, err
}

func (c *HTTPClient) GetMessages(ctx context.Context, channelID int64) (int, error) {
	_, status, err := c.do(ctx, http.MethodGet,
		fmt.Sprintf("/api/channels/%d/messages", channelID),
		nil,
	)
	return status, err
}

// --- Provisioner ---

type ProvisionRequest struct {
	Count  int    `json:"count"`
	Prefix string `json:"prefix"`
}

type ProvisionedUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type ProvisionResponse struct {
	Users     []ProvisionedUser `json:"users"`
	ServerID  int64             `json:"server_id"`
	ChannelID int64             `json:"channel_id"`
}

func (c *HTTPClient) Provision(ctx context.Context, count int, prefix string) (*ProvisionResponse, error) {
	body, status, err := c.do(ctx, http.MethodPost, "/internal/bench/provision", ProvisionRequest{
		Count: count, Prefix: prefix,
	})
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrProvisionNotFound
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("provision: status %d: %s", status, body)
	}
	var resp ProvisionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *HTTPClient) Cleanup(ctx context.Context, prefix string) (int64, error) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	body, status, err := c.do(cleanupCtx, http.MethodDelete, "/internal/bench/cleanup",
		map[string]string{"prefix": prefix},
	)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("cleanup: status %d: %s", status, body)
	}
	var resp struct {
		Deleted int64 `json:"deleted"`
	}
	json.Unmarshal(body, &resp)
	return resp.Deleted, nil
}

// ErrProvisionNotFound is returned when the provision endpoint returns 404
// (i.e., the server is a normal build, not a stresstest build).
var ErrProvisionNotFound = fmt.Errorf("provision endpoint not found — server may not be a stresstest build")
