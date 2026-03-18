package client

import (
	"context"
	"fmt"
)

// VirtualUser owns an HTTP client and optionally a WebSocket connection.
type VirtualUser struct {
	ID       int64
	Username string
	Token    string
	HTTP     *HTTPClient
	WS       *WSClient
}

// NewVirtualUser creates a VirtualUser from a provisioned account.
func NewVirtualUser(id int64, username, token, host, benchSecret string) *VirtualUser {
	return &VirtualUser{
		ID:       id,
		Username: username,
		Token:    token,
		HTTP:     NewHTTPClient(host, token, benchSecret),
	}
}

// ConnectWS obtains a WS ticket and connects to the WebSocket endpoint.
func (u *VirtualUser) ConnectWS(ctx context.Context, host string) error {
	ticket, err := u.HTTP.GetWSTicket(ctx)
	if err != nil {
		return fmt.Errorf("get ws ticket for user %s: %w", u.Username, err)
	}
	ws, err := NewWSClient(ctx, host, ticket)
	if err != nil {
		return fmt.Errorf("connect ws for user %s: %w", u.Username, err)
	}
	u.WS = ws
	return nil
}

// Subscribe subscribes the WS connection to a channel's events.
// Must call ConnectWS first.
func (u *VirtualUser) Subscribe(channelID int64) error {
	if u.WS == nil {
		return fmt.Errorf("user %s: WS not connected", u.Username)
	}
	return u.WS.Subscribe(channelID)
}

// Disconnect closes the WS connection if open.
func (u *VirtualUser) Disconnect() {
	if u.WS != nil {
		u.WS.Close()
	}
}
