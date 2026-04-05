package message

import (
	"context"
	"errors"
	"testing"
)

func TestSendMessage_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	tests := []struct {
		name          string
		channelID     string
		authorID      string
		content       string
		attachmentURL string
		wantErr       string
	}{
		{
			name:    "empty channel ID",
			authorID: "1",
			content: "hello",
			wantErr: "channel ID is required",
		},
		{
			name:      "empty author ID",
			channelID: "1",
			content:   "hello",
			wantErr:   "author ID is required",
		},
		{
			name:      "empty content and no attachment",
			channelID: "1",
			authorID:  "1",
			wantErr:   "content or attachment is required",
		},
		{
			name:      "non-numeric channel ID",
			channelID: "abc",
			authorID:  "1",
			content:   "hello",
			wantErr:   "invalid channel ID",
		},
		{
			name:      "non-numeric author ID",
			channelID: "1",
			authorID:  "abc",
			content:   "hello",
			wantErr:   "invalid author ID",
		},
		{
			name:      "spoofed link in content",
			channelID: "1",
			authorID:  "1",
			content:   "[http://goodguy.com](http://evil.com)",
			wantErr:   "message contains a spoofed link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SendMessage(context.Background(), tt.channelID, tt.authorID, tt.content, "", tt.attachmentURL, "", "", "")
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEditMessage_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	tests := []struct {
		name    string
		id      string
		content string
		wantErr string
	}{
		{
			name:    "empty content",
			id:      "1",
			wantErr: "content is required",
		},
		{
			name:    "invalid message ID",
			id:      "abc",
			content: "hello",
			wantErr: "invalid message ID",
		},
		{
			name:    "spoofed link in content",
			id:      "1",
			content: "[http://goodguy.com](http://evil.com)",
			wantErr: "message contains a spoofed link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.EditMessage(context.Background(), tt.id, tt.content)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDeleteMessage_InvalidID(t *testing.T) {
	svc := NewMessageService(nil)

	err := svc.DeleteMessage(context.Background(), "abc")
	if err == nil || err.Error() != "invalid message ID" {
		t.Errorf("got %v, want 'invalid message ID'", err)
	}
}

func TestGetMessage_InvalidID(t *testing.T) {
	svc := NewMessageService(nil)

	_, err := svc.GetMessage(context.Background(), "abc")
	if err == nil || err.Error() != "invalid message ID" {
		t.Errorf("got %v, want 'invalid message ID'", err)
	}
}

func TestGetChannelMessages_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	tests := []struct {
		name    string
		chID    string
		userID  string
		wantErr string
	}{
		{
			name:    "empty channel ID",
			userID:  "1",
			wantErr: "channel ID is required",
		},
		{
			name:    "empty user ID is forbidden",
			chID:    "1",
			wantErr: "forbidden",
		},
		{
			name:    "invalid channel ID",
			chID:    "abc",
			userID:  "1",
			wantErr: "invalid channel ID",
		},
		{
			name:    "invalid user ID",
			chID:    "1",
			userID:  "abc",
			wantErr: "invalid user ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetChannelMessages(context.Background(), tt.chID, tt.userID, 50, 0)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestToggleReaction_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	tests := []struct {
		name    string
		msgID   string
		userID  string
		emoji   string
		wantErr string
	}{
		{
			name:    "invalid message ID",
			msgID:   "abc",
			userID:  "1",
			emoji:   "👍",
			wantErr: "invalid message ID",
		},
		{
			name:    "invalid user ID",
			msgID:   "1",
			userID:  "abc",
			emoji:   "👍",
			wantErr: "invalid user ID",
		},
		{
			name:    "empty emoji",
			msgID:   "1",
			userID:  "1",
			emoji:   "",
			wantErr: "emoji is required",
		},
		{
			name:    "emoji too long",
			msgID:   "1",
			userID:  "1",
			emoji:   string(make([]byte, 65)),
			wantErr: "invalid emoji",
		},
		{
			name:    "emoji with control character",
			msgID:   "1",
			userID:  "1",
			emoji:   "\x01",
			wantErr: "invalid emoji",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ToggleReaction(context.Background(), tt.msgID, tt.userID, tt.emoji)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGetMessageVersions_InvalidID(t *testing.T) {
	svc := NewMessageService(nil)
	_, err := svc.GetMessageVersions(context.Background(), "abc")
	if err == nil || err.Error() != "invalid message ID" {
		t.Errorf("got %v, want 'invalid message ID'", err)
	}
}

func TestSearchMessages_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	tests := []struct {
		name     string
		serverID string
		userID   string
		wantErr  string
	}{
		{
			name:     "invalid server ID",
			serverID: "abc",
			userID:   "1",
			wantErr:  "invalid server ID",
		},
		{
			name:     "invalid user ID",
			serverID: "1",
			userID:   "abc",
			wantErr:  "invalid user ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SearchMessages(context.Background(), tt.serverID, tt.userID, "q", "", "", 25, 0)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestPinUnpin_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	tests := []struct {
		name      string
		channelID string
		messageID string
		userID    string
		wantErr   string
	}{
		{name: "invalid channel", channelID: "abc", messageID: "1", userID: "1", wantErr: "invalid channel ID"},
		{name: "invalid message", channelID: "1", messageID: "abc", userID: "1", wantErr: "invalid message ID"},
		{name: "invalid user", channelID: "1", messageID: "1", userID: "abc", wantErr: "invalid user ID"},
	}

	for _, tt := range tests {
		t.Run("Pin_"+tt.name, func(t *testing.T) {
			err := svc.PinMessage(context.Background(), tt.channelID, tt.messageID, tt.userID)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("PinMessage: got %v, want %q", err, tt.wantErr)
			}
		})
		t.Run("Unpin_"+tt.name, func(t *testing.T) {
			err := svc.UnpinMessage(context.Background(), tt.channelID, tt.messageID, tt.userID)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("UnpinMessage: got %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestGetChannelPins_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	_, err := svc.GetChannelPins(context.Background(), "abc", "1")
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("got %v, want 'invalid channel ID'", err)
	}
	_, err = svc.GetChannelPins(context.Background(), "1", "abc")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
}

func TestCanManageMessage_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	_, err := svc.CanManageMessage(context.Background(), "abc", "1")
	if err == nil || err.Error() != "invalid message ID" {
		t.Errorf("got %v, want 'invalid message ID'", err)
	}
	_, err = svc.CanManageMessage(context.Background(), "1", "abc")
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
}

func TestForwardToChannel_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	_, err := svc.ForwardToChannel(context.Background(), "abc", "1", nil)
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("got %v, want 'invalid channel ID'", err)
	}
	_, err = svc.ForwardToChannel(context.Background(), "1", "abc", nil)
	if err == nil || err.Error() != "invalid author ID" {
		t.Errorf("got %v, want 'invalid author ID'", err)
	}
}

func TestGetChannelMessagesAround_Validation(t *testing.T) {
	svc := NewMessageService(nil)

	_, err := svc.GetChannelMessagesAround(context.Background(), "abc", "1", 100)
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("got %v, want 'invalid channel ID'", err)
	}
	_, err = svc.GetChannelMessagesAround(context.Background(), "1", "", 100)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("got %v, want ErrForbidden", err)
	}
	_, err = svc.GetChannelMessagesAround(context.Background(), "1", "abc", 100)
	if err == nil || err.Error() != "invalid user ID" {
		t.Errorf("got %v, want 'invalid user ID'", err)
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrForbidden.Error() != "forbidden" {
		t.Errorf("ErrForbidden = %q", ErrForbidden)
	}
	if ErrChannelNotFound.Error() != "channel not found" {
		t.Errorf("ErrChannelNotFound = %q", ErrChannelNotFound)
	}
	if ErrMessageNotFound.Error() != "message not found" {
		t.Errorf("ErrMessageNotFound = %q", ErrMessageNotFound)
	}
	if ErrServerNotFound.Error() != "server not found" {
		t.Errorf("ErrServerNotFound = %q", ErrServerNotFound)
	}
}
