package channel

import (
	"database/sql"
	"testing"
	"time"

	"parley/internal/db"
)

func TestCreateChannel_Validation(t *testing.T) {
	svc := NewChannelService(nil)

	tests := []struct {
		name      string
		serverID  string
		chanName  string
		userID    string
		wantErr   string
	}{
		{
			name:     "empty name",
			serverID: "1",
			userID:   "1",
			wantErr:  "channel name is required",
		},
		{
			name:     "name too long",
			serverID: "1",
			chanName: string(make([]byte, 101)),
			userID:   "1",
			wantErr:  "channel name must be 100 characters or fewer",
		},
		{
			name:     "invalid server ID",
			serverID: "abc",
			chanName: "general",
			userID:   "1",
			wantErr:  "invalid server ID",
		},
		{
			name:     "invalid user ID",
			serverID: "1",
			chanName: "general",
			userID:   "abc",
			wantErr:  "invalid user ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateChannel(nil, tt.serverID, tt.chanName, 0, nil, "", tt.userID)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestUpdateChannel_Validation(t *testing.T) {
	svc := NewChannelService(nil)

	tests := []struct {
		name    string
		id      string
		chName  string
		userID  string
		wantErr string
	}{
		{name: "empty name", id: "1", userID: "1", wantErr: "channel name is required"},
		{name: "name too long", id: "1", chName: string(make([]byte, 101)), userID: "1", wantErr: "channel name must be 100 characters or fewer"},
		{name: "invalid channel ID", id: "abc", chName: "general", userID: "1", wantErr: "invalid channel ID"},
		{name: "invalid user ID", id: "1", chName: "general", userID: "abc", wantErr: "invalid user ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateChannel(nil, tt.id, tt.chName, "", tt.userID)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestGetChannel_InvalidID(t *testing.T) {
	svc := NewChannelService(nil)
	_, err := svc.GetChannel(nil, "abc")
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("got %v, want 'invalid channel ID'", err)
	}
}

func TestGetServerChannels_InvalidServerID(t *testing.T) {
	svc := NewChannelService(nil)
	_, err := svc.GetServerChannels(nil, "abc", "", "")
	if err == nil || err.Error() != "invalid server ID" {
		t.Errorf("got %v, want 'invalid server ID'", err)
	}
}

func TestDeleteChannel_InvalidID(t *testing.T) {
	svc := NewChannelService(nil)
	err := svc.DeleteChannel(nil, "abc", "1")
	if err == nil || err.Error() != "invalid channel ID" {
		t.Errorf("got %v, want 'invalid channel ID'", err)
	}
}

func TestReorderChannels_Validation(t *testing.T) {
	svc := NewChannelService(nil)

	tests := []struct {
		name     string
		serverID string
		userID   string
		orders   []ChannelOrder
		wantErr  string
	}{
		{name: "invalid server ID", serverID: "abc", userID: "1", wantErr: "invalid server ID"},
		{name: "invalid user ID", serverID: "1", userID: "abc", wantErr: "invalid user ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ReorderChannels(nil, tt.serverID, tt.orders, tt.userID)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDbChannelToChannel(t *testing.T) {
	now := time.Now()
	parentID := int64(42)

	tests := []struct {
		name     string
		input    *db.Channel
		wantID   string
		wantName string
		wantType ChannelType
		hasParent bool
		wantParent string
	}{
		{
			name: "basic conversion",
			input: &db.Channel{
				ID:          1,
				ServerID:    10,
				Name:        "general",
				ChannelType: db.ChannelType(0),
				Position:    0,
				Topic:       "A topic",
				Synced:      true,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			wantID:   "1",
			wantName: "general",
			wantType: ChannelTypeText,
		},
		{
			name: "with parent ID",
			input: &db.Channel{
				ID:       2,
				ServerID: 10,
				Name:     "voice",
				ChannelType: db.ChannelType(1),
				ParentID: sql.NullInt64{Int64: parentID, Valid: true},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantID:     "2",
			wantName:   "voice",
			wantType:   ChannelTypeVoice,
			hasParent:  true,
			wantParent: "42",
		},
		{
			name: "without parent ID",
			input: &db.Channel{
				ID:        3,
				ServerID:  10,
				Name:      "bin",
				ChannelType: db.ChannelType(2),
				ParentID: sql.NullInt64{Valid: false},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantID:   "3",
			wantName: "bin",
			wantType: ChannelTypeBin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := dbChannelToChannel(tt.input)
			if ch.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", ch.ID, tt.wantID)
			}
			if ch.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ch.Name, tt.wantName)
			}
			if ch.Type != tt.wantType {
				t.Errorf("Type = %d, want %d", ch.Type, tt.wantType)
			}
			if tt.hasParent {
				if ch.ParentID == nil {
					t.Fatal("ParentID is nil, want non-nil")
				}
				if *ch.ParentID != tt.wantParent {
					t.Errorf("ParentID = %q, want %q", *ch.ParentID, tt.wantParent)
				}
			} else {
				if ch.ParentID != nil {
					t.Errorf("ParentID = %q, want nil", *ch.ParentID)
				}
			}
		})
	}
}

func TestInt64ToNullInt64(t *testing.T) {
	// nil input
	result := int64ToNullInt64(nil)
	if result.Valid {
		t.Error("nil input should produce Valid=false")
	}

	// non-nil input
	val := int64(42)
	result = int64ToNullInt64(&val)
	if !result.Valid {
		t.Error("non-nil input should produce Valid=true")
	}
	if result.Int64 != 42 {
		t.Errorf("Int64 = %d, want 42", result.Int64)
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrForbidden.Error() != "forbidden" {
		t.Errorf("ErrForbidden = %q", ErrForbidden)
	}
	if ErrServerNotFound.Error() != "server not found" {
		t.Errorf("ErrServerNotFound = %q", ErrServerNotFound)
	}
	if ErrChannelNotFound.Error() != "channel not found" {
		t.Errorf("ErrChannelNotFound = %q", ErrChannelNotFound)
	}
}

func TestChannelTypeConstants(t *testing.T) {
	tests := []struct {
		ct   ChannelType
		want int
	}{
		{ChannelTypeText, 0},
		{ChannelTypeVoice, 1},
		{ChannelTypeBin, 2},
		{ChannelTypeCategory, 3},
	}
	for _, tt := range tests {
		if int(tt.ct) != tt.want {
			t.Errorf("ChannelType %d, want %d", tt.ct, tt.want)
		}
	}
}

func TestGetServerOwnerID_InvalidID(t *testing.T) {
	svc := NewChannelService(nil)
	// With nil repo, parsing "abc" returns "" before repo access
	result := svc.GetServerOwnerID(nil, "abc")
	if result != "" {
		t.Errorf("got %q, want empty string for invalid server ID", result)
	}
}
