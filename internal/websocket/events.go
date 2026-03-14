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
)