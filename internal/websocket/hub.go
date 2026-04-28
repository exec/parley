package websocket

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
)

// Lock ordering: h.mu must always be acquired before sendMu (Client.sendMu).
// Never acquire h.mu while holding sendMu — this would invert the ordering
// and risk deadlock.

// presenceSnapshotMax caps the number of online user IDs sent in the
// PRESENCE_SNAPSHOT event on connect. The frontend fills in remaining users
// incrementally via USER_ONLINE events. Sending all 25k IDs per connection
// is ~150KB of JSON with no practical benefit.
const presenceSnapshotMax = 500

// maxConnectionsPerUser is the maximum number of concurrent WebSocket connections
// allowed per user. Connections beyond this limit are rejected immediately.
const maxConnectionsPerUser = 10

// maxSubscriptionsPerClient is the maximum number of channel subscriptions a
// single WebSocket client may hold. Prevents a misbehaving client from consuming
// unbounded memory in the channelSubs/clientChannels maps.
const maxSubscriptionsPerClient = 500

// Publisher is implemented by RedisHub to cross-publish events to other nodes.
// Hub holds an optional reference to it.
type Publisher interface {
	PublishToChannel(channelID, event string, data []byte)
	PublishToUser(userID, event string, data []byte)
	// Global presence
	PublishGlobal(event string, data []byte)
	MarkOnline(userID string)
	MarkOffline(userID string)
	GetOnlineUserIDs() []string
}

// StatusWriter is implemented by *db.Repository. Hub uses it to persist
// online/offline status to the database on WS connect and disconnect.
type StatusWriter interface {
	SetUserStatusType(ctx context.Context, userID int64, statusType string) error
	SetUserStatusTypeIfNotInvisible(ctx context.Context, userID int64, statusType string) error
}

// safeSend attempts a non-blocking send to the client's send channel.
// Returns false if the client is already closed or the channel buffer is full.
//
// Concurrency safety: closeSend holds client.sendMu.Lock() while closing the
// channel. safeSend holds client.sendMu.RLock() while sending. This prevents
// close(send) and send<-msg from ever executing concurrently, eliminating the
// data race that the Go race detector would otherwise flag.
func safeSend(client *Client, msg []byte) bool {
	client.sendMu.RLock()
	defer client.sendMu.RUnlock()

	if client.closed {
		return false
	}

	select {
	case client.send <- msg:
		return true
	default:
		return false
	}
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	mu sync.RWMutex

	// Registered clients
	clients map[*Client]bool

	// User ID to client mapping (one user can have multiple connections)
	userToClient map[string]map[*Client]bool

	// Channel subscribers
	channelSubs map[string]map[*Client]bool

	// clientChannels is the inverse index of channelSubs: for O(k) unregister cleanup
	// where k = number of channels this client is subscribed to.
	clientChannels map[*Client]map[string]bool

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast messages to clients
	broadcast chan *Message

	// publisher is optional; if set, events are also published cross-node via Redis
	publisher Publisher

	// statusWriter persists online/offline status to the DB on connect/disconnect.
	statusWriter StatusWriter

	// channelAccessChecker is an optional function to verify whether a user is
	// allowed to subscribe to a channel. If nil, access is denied (fail closed).
	channelAccessChecker func(userID, channelID string) bool

	// channelServerResolver maps a subscription channel ID (as stored in
	// channelSubs / clientChannels) to the server ID it belongs to. Returns
	// ("", false) for DM channels or unknown IDs. Used by
	// UnsubscribeUserFromServer. If nil, only "server:{id}" virtual channels
	// can be matched.
	channelServerResolver func(channelID string) (serverID string, ok bool)
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[*Client]bool),
		userToClient:   make(map[string]map[*Client]bool),
		channelSubs:    make(map[string]map[*Client]bool),
		clientChannels: make(map[*Client]map[string]bool),
		register:       make(chan *Client, 64),
		// unregister buffer is sized to absorb a thundering-herd disconnect
		// (e.g. node restart). Without a wide buffer, ReadPump's defer falls
		// back to spawning a goroutine per disconnect — 5k disconnects = 5k
		// goroutines fighting to push into the channel.
		unregister: make(chan *Client, 8192),
		broadcast:  make(chan *Message, 1024),
	}
}

// SetPublisher sets the cross-node publisher (e.g. RedisHub).
// Call this before starting the hub's Run loop.
func (h *Hub) SetPublisher(p Publisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.publisher = p
}

// SetStatusWriter sets the StatusWriter used to persist online/offline status.
// Call this before starting the hub's Run loop.
func (h *Hub) SetStatusWriter(sw StatusWriter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statusWriter = sw
}

// SetChannelAccessChecker sets the function used to decide whether a user may
// subscribe to a given channel. Call this before starting the hub's Run loop.
func (h *Hub) SetChannelAccessChecker(fn func(userID, channelID string) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.channelAccessChecker = fn
}

// SetChannelServerResolver sets the function used to look up which server a
// given subscription channel ID belongs to. Call this before starting the
// hub's Run loop. The resolver is called with the lock released.
func (h *Hub) SetChannelServerResolver(fn func(channelID string) (serverID string, ok bool)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.channelServerResolver = fn
}

// CheckChannelAccess returns true only if the channelAccessChecker confirms the
// user has access. Fails closed (returns false) when no checker is configured.
func (h *Hub) CheckChannelAccess(userID, channelID string) bool {
	h.mu.RLock()
	fn := h.channelAccessChecker
	h.mu.RUnlock()
	if fn == nil {
		return false
	}
	return fn(userID, channelID)
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.RegisterClient(client)

		case client := <-h.unregister:
			h.UnregisterClient(client)

		case message := <-h.broadcast:
			h.BroadcastToChannel(message.ChannelID, message.Type, message.Payload)
		}
	}
}

// RegisterClient adds a client to the hub, broadcasts USER_ONLINE globally,
// and sends a PRESENCE_SNAPSHOT of all online users to the new client.
func (h *Hub) RegisterClient(client *Client) {
	h.mu.Lock()

	if h.userToClient[client.userID] != nil && len(h.userToClient[client.userID]) >= maxConnectionsPerUser {
		h.mu.Unlock()
		client.closeSend()
		client.conn.Close()
		log.Printf("RegisterClient: user %s exceeded max connections (%d), rejecting", client.userID, maxConnectionsPerUser)
		return
	}

	h.clients[client] = true

	// Add to user map
	if h.userToClient[client.userID] == nil {
		h.userToClient[client.userID] = make(map[*Client]bool)
	}
	isFirstConnection := len(h.userToClient[client.userID]) == 0
	h.userToClient[client.userID][client] = true

	pub := h.publisher

	h.mu.Unlock()

	// Mark online in the cross-node presence store (first connection only)
	if isFirstConnection && pub != nil {
		pub.MarkOnline(client.userID)
	}

	// Build snapshot — use Redis for cross-node truth, fall back to local map.
	// Capped at presenceSnapshotMax to prevent ~150KB payloads at 25k users.
	var onlineUserIDs []string
	if pub != nil {
		onlineUserIDs = pub.GetOnlineUserIDs()
	} else {
		onlineUserIDs = make([]string, 0, presenceSnapshotMax)
		h.mu.RLock()
		for uid := range h.userToClient {
			onlineUserIDs = append(onlineUserIDs, uid)
			if len(onlineUserIDs) >= presenceSnapshotMax {
				break
			}
		}
		h.mu.RUnlock()
	}
	if len(onlineUserIDs) > presenceSnapshotMax {
		onlineUserIDs = onlineUserIDs[:presenceSnapshotMax]
	}

	// Send the new client a snapshot of everyone currently online
	if snapshotPayload, err := json.Marshal(PresenceSnapshotPayload{UserIDs: onlineUserIDs}); err == nil {
		client.Send(EventPresenceSnapshot, snapshotPayload)
	}

	// Announce arrival (only once per user, not once per tab/connection)
	if isFirstConnection {
		if onlinePayload, err := json.Marshal(UserPresencePayload{UserID: client.userID}); err == nil {
			// Deliver to clients on this node
			h.broadcastToAllLocal(EventUserOnline, onlinePayload)
			// Deliver to clients on other nodes via Redis
			if pub != nil {
				pub.PublishGlobal(EventUserOnline, onlinePayload)
			}
		}
	}

	// Persist 'online' status to DB on first connection (skip if invisible).
	if isFirstConnection {
		uid, parseErr := strconv.ParseInt(client.userID, 10, 64)
		h.mu.RLock()
		sw := h.statusWriter
		h.mu.RUnlock()
		if parseErr == nil && sw != nil {
			go func(id int64) {
				if err := sw.SetUserStatusTypeIfNotInvisible(context.Background(), id, "online"); err != nil {
					log.Printf("hub: set online for user %d: %v", id, err)
				}
			}(uid)
		}
	}

}

// UnregisterClient removes a client from the hub and broadcasts USER_OFFLINE
// globally when the user has no remaining connections.
func (h *Hub) UnregisterClient(client *Client) {
	h.mu.Lock()

	userFullyOffline := false

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		client.closeSend()

		// Remove from user map
		if h.userToClient[client.userID] != nil {
			delete(h.userToClient[client.userID], client)
			if len(h.userToClient[client.userID]) == 0 {
				delete(h.userToClient, client.userID)
				userFullyOffline = true
			}
		}

		// Remove from all channel subscriptions using O(k) inverse index
		// where k = channels this client is subscribed to.
		if channels := h.clientChannels[client]; channels != nil {
			for channelID := range channels {
				if h.channelSubs[channelID] != nil {
					delete(h.channelSubs[channelID], client)
					if len(h.channelSubs[channelID]) == 0 {
						delete(h.channelSubs, channelID)
					}
				}
			}
			delete(h.clientChannels, client)
		}

	}

	pub := h.publisher

	h.mu.Unlock()

	// Broadcast USER_OFFLINE only when the user has no remaining connections.
	if userFullyOffline {
		if offlinePayload, err := json.Marshal(UserPresencePayload{UserID: client.userID}); err == nil {
			// Deliver to clients on this node
			h.broadcastToAllLocal(EventUserOffline, offlinePayload)
			// Remove from cross-node presence store and notify other nodes
			if pub != nil {
				pub.MarkOffline(client.userID)
				pub.PublishGlobal(EventUserOffline, offlinePayload)
			}
		}
	}

	// Persist 'offline' status to DB unconditionally on last disconnect.
	if userFullyOffline {
		uid, parseErr := strconv.ParseInt(client.userID, 10, 64)
		h.mu.RLock()
		sw := h.statusWriter
		h.mu.RUnlock()
		if parseErr == nil && sw != nil {
			go func(id int64) {
				if err := sw.SetUserStatusType(context.Background(), id, "offline"); err != nil {
					log.Printf("hub: set offline for user %d: %v", id, err)
				}
			}(uid)
		}
	}
}

// SubscribeToChannel adds a client to a channel's subscriber list.
// Presence events are now handled globally (on connect/disconnect), not per-channel.
func (h *Hub) SubscribeToChannel(channelID string, client *Client) {
	h.mu.Lock()

	if len(h.clientChannels[client]) >= maxSubscriptionsPerClient {
		h.mu.Unlock()
		log.Printf("SubscribeToChannel: client (user %s) at subscription limit (%d), ignoring channel %s", client.userID, maxSubscriptionsPerClient, channelID)
		return
	}

	if h.channelSubs[channelID] == nil {
		h.channelSubs[channelID] = make(map[*Client]bool)
	}
	h.channelSubs[channelID][client] = true

	// Maintain inverse index
	if h.clientChannels[client] == nil {
		h.clientChannels[client] = make(map[string]bool)
	}
	h.clientChannels[client][channelID] = true

	h.mu.Unlock()
}

// UnsubscribeFromChannel removes a client from a channel's subscriber list.
// Presence events are now handled globally (on connect/disconnect), not per-channel.
func (h *Hub) UnsubscribeFromChannel(channelID string, client *Client) {
	h.mu.Lock()

	if h.channelSubs[channelID] != nil {
		delete(h.channelSubs[channelID], client)
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
	}

	// Maintain inverse index
	if h.clientChannels[client] != nil {
		delete(h.clientChannels[client], channelID)
		if len(h.clientChannels[client]) == 0 {
			delete(h.clientChannels, client)
		}
	}

	h.mu.Unlock()
}

// SendToUser sends a message to a specific user by their userID.
// It also publishes to Redis (if a publisher is set) so other nodes deliver it too.
func (h *Hub) SendToUser(userID string, messageType string, payload []byte) error {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return err
	}

	h.mu.RLock()
	pub := h.publisher
	userClients := h.userToClient[userID]
	clients := make([]*Client, 0, len(userClients))
	for c := range userClients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		if pub != nil {
			pub.PublishToUser(userID, messageType, payload)
		}
		return nil
	}

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			// Minimal eviction: close the send channel only. userToClient cleanup
			// is deferred to UnregisterClient — unlike channel evictions, there is no
			// stale subscriber list to prune here since we already skipped dead clients.
			client.closeSend()
		}
		h.mu.Unlock()
	}

	if pub != nil {
		pub.PublishToUser(userID, messageType, payload)
	}

	return nil
}

// DisconnectUser closes all WebSocket connections for the given user.
// The natural teardown chain (WritePump exit → conn close → ReadPump unregister)
// handles map cleanup, so we only need to close the send channels here.
func (h *Hub) DisconnectUser(userID string) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.userToClient[userID]))
	for c := range h.userToClient[userID] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		c.closeSend()
	}
}

// UnsubscribeUserFromServer drops all of the user's channel subscriptions
// that belong to the given server, leaving DM and other-server subscriptions
// intact. Called on kick/ban/leave so a removed user stops receiving
// real-time events from the server. Does NOT close the WS connection.
//
// The "server:{serverID}" virtual channel is always matched directly. For
// other channels, the hub's channelServerResolver (if set) is consulted
// outside the lock; missing resolvers mean only the virtual channel is
// dropped.
func (h *Hub) UnsubscribeUserFromServer(userID, serverID string) {
	if userID == "" || serverID == "" {
		return
	}

	// Snapshot clients and their current subscription sets under RLock.
	h.mu.RLock()
	resolver := h.channelServerResolver
	userClients := h.userToClient[userID]
	type clientChans struct {
		client   *Client
		channels []string
	}
	snapshots := make([]clientChans, 0, len(userClients))
	for c := range userClients {
		chs := h.clientChannels[c]
		if len(chs) == 0 {
			continue
		}
		list := make([]string, 0, len(chs))
		for chID := range chs {
			list = append(list, chID)
		}
		snapshots = append(snapshots, clientChans{client: c, channels: list})
	}
	h.mu.RUnlock()

	if len(snapshots) == 0 {
		return
	}

	// Decide which channels belong to serverID (outside the lock — resolver
	// may touch the DB or a cache).
	serverPrefix := "server:" + serverID
	toDrop := make(map[*Client][]string, len(snapshots))
	for _, snap := range snapshots {
		for _, chID := range snap.channels {
			if chID == serverPrefix {
				toDrop[snap.client] = append(toDrop[snap.client], chID)
				continue
			}
			// Skip other "server:" virtual channels and DM channels.
			if len(chID) >= 7 && chID[:7] == "server:" {
				continue
			}
			if len(chID) >= 3 && chID[:3] == "dm:" {
				continue
			}
			if resolver == nil {
				continue
			}
			if sID, ok := resolver(chID); ok && sID == serverID {
				toDrop[snap.client] = append(toDrop[snap.client], chID)
			}
		}
	}

	if len(toDrop) == 0 {
		return
	}

	// Apply removals under WLock.
	h.mu.Lock()
	for client, channels := range toDrop {
		clientChans := h.clientChannels[client]
		for _, chID := range channels {
			if subs := h.channelSubs[chID]; subs != nil {
				delete(subs, client)
				if len(subs) == 0 {
					delete(h.channelSubs, chID)
				}
			}
			if clientChans != nil {
				delete(clientChans, chID)
			}
		}
		if clientChans != nil && len(clientChans) == 0 {
			delete(h.clientChannels, client)
		}
	}
	h.mu.Unlock()
}

// BroadcastToAllLocal sends a message to every locally-connected client without
// republishing to Redis. Called internally for presence events and by the Redis
// subscriber when delivering "global" events from other nodes.
func (h *Hub) BroadcastToAllLocal(messageType string, payload []byte) {
	h.broadcastToAllLocal(messageType, payload)
}

// broadcastToAllLocal is the unexported implementation.
func (h *Hub) broadcastToAllLocal(messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			// Minimal eviction: close the send channel only.
			// Full cleanup (h.clients, h.userToClient, clientChannels) happens
			// when UnregisterClient fires naturally after closeSend drains.
			client.closeSend()
		}
		h.mu.Unlock()
	}
}

// BroadcastStatusUpdate broadcasts a USER_STATUS_UPDATE event to all clients.
func (h *Hub) BroadcastStatusUpdate(userID, statusType, statusText string) {
	payload, err := json.Marshal(UserStatusUpdatePayload{
		UserID:     userID,
		StatusType: statusType,
		StatusText: statusText,
	})
	if err != nil {
		return
	}
	h.broadcastToAllLocal(EventUserStatusUpdate, payload)
	h.mu.RLock()
	pub := h.publisher
	h.mu.RUnlock()
	if pub != nil {
		pub.PublishGlobal(EventUserStatusUpdate, payload)
	}
}

// BroadcastLocalToChannel sends to local clients subscribed to a channel ONLY.
// No Redis publish — use this when delivering events received from Redis to avoid
// the infinite re-broadcast loop that would occur if we published back to Redis.
func (h *Hub) BroadcastLocalToChannel(channelID string, messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("BroadcastLocalToChannel: marshal error: %v", err)
		return
	}

	h.mu.RLock()
	subs := h.channelSubs[channelID]
	if len(subs) == 0 {
		h.mu.RUnlock()
		return
	}
	clients := make([]*Client, 0, len(subs))
	for c := range subs {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
			delete(h.channelSubs[channelID], client)
			if h.clientChannels[client] != nil {
				delete(h.clientChannels[client], channelID)
			}
		}
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
		h.mu.Unlock()
	}
}

// SendLocalToUser delivers to local clients for a user ONLY — no Redis publish.
func (h *Hub) SendLocalToUser(userID string, messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		return
	}

	h.mu.RLock()
	userClients := h.userToClient[userID]
	clients := make([]*Client, 0, len(userClients))
	for c := range userClients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			// Minimal eviction: close the send channel only. userToClient cleanup
			// is deferred to UnregisterClient — unlike channel evictions, there is no
			// stale subscriber list to prune here since we already skipped dead clients.
			client.closeSend()
		}
		h.mu.Unlock()
	}
}

// BroadcastToChannel sends a message to all clients subscribed to a channel.
// It also publishes to Redis (if a publisher is set) so other nodes deliver it too.
//
// Performance: JSON marshaling happens once outside the lock. Subscriber snapshot
// is taken under RLock. Sends happen outside all locks. Evictions (slow/full
// send buffers) take a brief WLock at the end.
func (h *Hub) BroadcastToChannel(channelID string, messageType string, payload []byte) {
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("BroadcastToChannel: marshal error: %v", err)
		return
	}
	h.fanoutChannel(channelID, messageType, payload, msgBytes)
}

// BroadcastToChannels marshals the WSMessage envelope once and fans out to
// every channel ID. Saves N-1 marshals when broadcasting the same payload to
// many channels (e.g. profile updates across every server a user belongs to).
func (h *Hub) BroadcastToChannels(channelIDs []string, messageType string, payload []byte) {
	if len(channelIDs) == 0 {
		return
	}
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("BroadcastToChannels: marshal error: %v", err)
		return
	}
	for _, channelID := range channelIDs {
		h.fanoutChannel(channelID, messageType, payload, msgBytes)
	}
}

// fanoutChannel is the shared 3-phase send path used by BroadcastToChannel and
// BroadcastToChannels. msgBytes is the pre-marshaled WSMessage envelope;
// messageType and payload are forwarded to the cross-node Publisher.
func (h *Hub) fanoutChannel(channelID, messageType string, payload, msgBytes []byte) {
	// Phase 1: Snapshot subscribers and publisher under RLock.
	h.mu.RLock()
	pub := h.publisher
	subs := h.channelSubs[channelID]
	if len(subs) == 0 {
		h.mu.RUnlock()
		if pub != nil {
			pub.PublishToChannel(channelID, messageType, payload)
		}
		return
	}
	clients := make([]*Client, 0, len(subs))
	for c := range subs {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	// Phase 2: Send outside all locks. safeSend handles closed channels (evicted
	// clients) and full buffers without panicking.
	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	// Phase 3: Minimal eviction under a brief WLock.
	// We remove from channelSubs so future broadcasts skip this dead client.
	// We do NOT delete from h.clients — that would bypass UnregisterClient's
	// guard and cause USER_OFFLINE to never fire. The natural teardown chain
	// (closeSend → WritePump exit → conn.Close → ReadPump unregister → UnregisterClient)
	// handles full map cleanup and presence broadcasting.
	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
			delete(h.channelSubs[channelID], client)
			if h.clientChannels[client] != nil {
				delete(h.clientChannels[client], channelID)
			}
		}
		if len(h.channelSubs[channelID]) == 0 {
			delete(h.channelSubs, channelID)
		}
		h.mu.Unlock()
	}

	if pub != nil {
		pub.PublishToChannel(channelID, messageType, payload)
	}
}

// SendBytesToUsers marshals the WSMessage envelope once and delivers it to all
// of the given users' active connections on this node, plus a Redis publish
// per user for cross-node delivery. Use when fanning out the same payload to
// many users (e.g. group DM channel-create) — the hot path is one marshal
// regardless of recipient count.
//
// User IDs are int64 to match the rest of the codebase; they're stringified
// internally to align with the userToClient map's string keys.
func (h *Hub) SendBytesToUsers(userIDs []int64, messageType string, payload []byte) {
	if len(userIDs) == 0 {
		return
	}
	wsMsg := WSMessage{Type: messageType, Payload: payload}
	msgBytes, err := json.Marshal(wsMsg)
	if err != nil {
		log.Printf("SendBytesToUsers: marshal error: %v", err)
		return
	}

	// Phase 1: snapshot all clients across all target users under a single RLock.
	h.mu.RLock()
	pub := h.publisher
	var clients []*Client
	for _, uid := range userIDs {
		userClients := h.userToClient[strconv.FormatInt(uid, 10)]
		for c := range userClients {
			clients = append(clients, c)
		}
	}
	h.mu.RUnlock()

	// Phase 2: send outside locks (same safeSend semantics as SendToUser).
	var toEvict []*Client
	for _, client := range clients {
		if !safeSend(client, msgBytes) {
			toEvict = append(toEvict, client)
		}
	}

	// Phase 3: minimal eviction — close the send channel only. userToClient
	// cleanup is deferred to UnregisterClient (matches SendToUser semantics).
	if len(toEvict) > 0 {
		h.mu.Lock()
		for _, client := range toEvict {
			client.closeSend()
		}
		h.mu.Unlock()
	}

	// Cross-node publish: one PublishToUser per user. Each user has its own
	// Redis topic, so this can't be batched at the publisher layer.
	if pub != nil {
		for _, uid := range userIDs {
			pub.PublishToUser(strconv.FormatInt(uid, 10), messageType, payload)
		}
	}
}
