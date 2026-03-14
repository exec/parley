package message

// Broadcaster is an interface for broadcasting messages to channels and users
type Broadcaster interface {
	BroadcastToChannel(channelID string, event string, data interface{})
	BroadcastToUser(userID string, event string, data interface{})
}