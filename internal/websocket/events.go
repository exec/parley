package websocket

// Typed event payload structs. Used in place of inline map literals to skip the
// reflection-heavy json.Marshal map path and to give callers a stable shape.

// PresenceSnapshotPayload is the body of the PRESENCE_SNAPSHOT event sent to a
// new client at connect time.
type PresenceSnapshotPayload struct {
	UserIDs []string `json:"user_ids"`
}

// UserPresencePayload is the body of USER_ONLINE / USER_OFFLINE events.
type UserPresencePayload struct {
	UserID string `json:"user_id"`
}

// UserStatusUpdatePayload is the body of USER_STATUS_UPDATE events.
type UserStatusUpdatePayload struct {
	UserID     string `json:"user_id"`
	StatusType string `json:"status_type"`
	StatusText string `json:"status_text"`
}

// Event types for WebSocket messages
const (
	EventMessageCreate    = "MESSAGE_CREATE"
	EventMessageUpdate    = "MESSAGE_UPDATE"
	EventMessageDelete    = "MESSAGE_DELETE"
	EventUserJoin         = "USER_JOIN"
	EventUserLeave        = "USER_LEAVE"
	EventUserTyping       = "USER_TYPING"
	EventUserOnline       = "USER_ONLINE"
	EventUserOffline      = "USER_OFFLINE"
	EventPresenceSnapshot = "PRESENCE_SNAPSHOT"
	EventReactionAdd      = "REACTION_ADD"
	EventReactionRemove   = "REACTION_REMOVE"

	// Channel events
	EventChannelCreate = "CHANNEL_CREATE"
	EventChannelUpdate = "CHANNEL_UPDATE"
	EventChannelDelete = "CHANNEL_DELETE"

	// Server events
	EventServerUpdate       = "SERVER_UPDATE"
	EventServerDelete       = "SERVER_DELETE"
	EventUserServersReorder = "USER_SERVERS_REORDER"

	// Member events
	EventMemberJoin  = "SERVER_MEMBER_JOIN"
	EventMemberLeave = "SERVER_MEMBER_LEAVE"
	EventMemberKick  = "SERVER_MEMBER_KICK"
	EventMemberBan   = "SERVER_MEMBER_BAN"

	// Role events
	EventMemberRoleUpdate = "MEMBER_ROLE_UPDATE"
	EventRoleUpdate       = "ROLE_UPDATE"
	EventRoleDelete       = "ROLE_DELETE"

	// Channel permission events
	EventChannelOverwriteUpdate = "CHANNEL_OVERWRITE_UPDATE"

	// User update
	EventUserUpdate = "USER_UPDATE"

	// Voice events
	EventVoiceStateUpdate     = "VOICE_STATE_UPDATE"
	EventVoiceForceMute       = "VOICE_FORCE_MUTE"
	EventVoiceForceDisconnect = "VOICE_FORCE_DISCONNECT"

	// Bot events
	EventBotStatusUpdate = "BOT_STATUS_UPDATE"

	// Status events
	EventUserStatusUpdate = "USER_STATUS_UPDATE"

	// Friend events
	EventFriendRequest = "FRIEND_REQUEST"
	EventFriendAccept  = "FRIEND_ACCEPT"
	EventFriendRemove  = "FRIEND_REMOVE"

	// Bin events
	EventBinPostCreate        = "BIN_POST_CREATE"
	EventBinPostUpdate        = "BIN_POST_UPDATE"
	EventBinPostDelete        = "BIN_POST_DELETE"
	EventBinLineCommentCreate = "BIN_LINE_COMMENT_CREATE"
	EventBinLineCommentUpdate = "BIN_LINE_COMMENT_UPDATE"
	EventBinLineCommentDelete = "BIN_LINE_COMMENT_DELETE"

	// Project events (Phase A.A1) — broadcast to "server:{id}" topic.
	EventProjectCreate = "PROJECT_CREATE"
	EventProjectUpdate = "PROJECT_UPDATE"
	EventProjectDelete = "PROJECT_DELETE"

	// Soundboard events
	EventSoundboardPlay = "SOUNDBOARD_PLAY"

	// Notification events
	EventNotificationCreate = "NOTIFICATION_CREATE"

	// DM events
	EventDmChannelCreate  = "DM_CHANNEL_CREATE"
	EventDmChannelUpdate  = "DM_CHANNEL_UPDATE"
	EventDmMessageCreate  = "dm_message" // wire-compatible with existing literal in SendDmMessage broadcast
	EventDmMessageDelete  = "dm_message_delete"
	EventDmReactionAdd    = "dm_reaction_add"
	EventDmReactionRemove = "dm_reaction_remove"
	EventDmMemberAdd      = "DM_MEMBER_ADD"
	EventDmMemberRemove   = "DM_MEMBER_REMOVE"

	// Cross-cutting per-channel state events (server channels + DMs)
	EventChannelReadStateUpdate    = "CHANNEL_READ_STATE_UPDATE"
	EventChannelNotificationUpdate = "CHANNEL_NOTIFICATION_UPDATE"

	// Slash command / interaction events
	EventInteractionCreate = "INTERACTION_CREATE"

	// Call events (1:1 ringing)
	EventCallRing    = "CALL_RING"
	EventCallAccept  = "CALL_ACCEPT"
	EventCallDecline = "CALL_DECLINE"
	EventCallCancel  = "CALL_CANCEL"
	EventCallTimeout = "CALL_TIMEOUT"

	// VC activity events (stub harness; events fire even though the registry is empty in this iteration)
	EventActivityStart = "ACTIVITY_START"
	EventActivityEnd   = "ACTIVITY_END"
)
