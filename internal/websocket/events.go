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
	EventMemberRoleUpdate      = "MEMBER_ROLE_UPDATE"
	EventRoleUpdate            = "ROLE_UPDATE"
	EventRoleDelete            = "ROLE_DELETE"

	// Channel permission events
	EventChannelOverwriteUpdate = "CHANNEL_OVERWRITE_UPDATE"

	// User update
	EventUserUpdate = "USER_UPDATE"

	// Voice events
	EventVoiceStateUpdate     = "VOICE_STATE_UPDATE"
	EventVoiceForceMute       = "VOICE_FORCE_MUTE"
	EventVoiceForceDisconnect = "VOICE_FORCE_DISCONNECT"

	// Bin events
	EventBinPostCreate        = "BIN_POST_CREATE"
	EventBinPostUpdate        = "BIN_POST_UPDATE"
	EventBinPostDelete        = "BIN_POST_DELETE"
	EventBinLineCommentCreate = "BIN_LINE_COMMENT_CREATE"
	EventBinLineCommentUpdate = "BIN_LINE_COMMENT_UPDATE"
	EventBinLineCommentDelete = "BIN_LINE_COMMENT_DELETE"
)