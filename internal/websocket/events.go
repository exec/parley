package websocket

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
	EventServerUpdate = "SERVER_UPDATE"
	EventServerDelete = "SERVER_DELETE"

	// Member events
	EventMemberJoin   = "SERVER_MEMBER_JOIN"
	EventMemberLeave  = "SERVER_MEMBER_LEAVE"
	EventMemberKick   = "SERVER_MEMBER_KICK"
	EventMemberBan    = "SERVER_MEMBER_BAN"

	// Role events
	EventMemberRoleUpdate = "MEMBER_ROLE_UPDATE"

	// User update
	EventUserUpdate = "USER_UPDATE"
)