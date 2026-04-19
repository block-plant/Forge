package realtime

import (
	"encoding/json"
	"sync"

	"github.com/ayushkunwarsingh/forge/logger"
)

// Hub is the central pub/sub hub that manages all WebSocket clients and channels.
type Hub struct {
	// Registered clients
	clients    map[string]*Client
	clientsMu  sync.RWMutex

	// Channel subscriptions: channel name → set of client IDs
	channels   map[string]map[string]*Client
	channelsMu sync.RWMutex

	// Register/unregister channels
	register   chan *Client
	unregister chan *Client

	log *logger.Logger
}

// NewHub creates a new pub/sub hub.
func NewHub(log *logger.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		channels:   make(map[string]map[string]*Client),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		log:        log.WithField("service", "realtime"),
	}
}

// Run starts the hub's main event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clientsMu.Lock()
			h.clients[client.ID] = client
			h.clientsMu.Unlock()

			h.log.Info("Client connected", logger.Fields{
				"client_id": client.ID,
				"remote":    client.Conn.RemoteAddr(),
			})

		case client := <-h.unregister:
			h.clientsMu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.send)
			}
			h.clientsMu.Unlock()

			// Remove from all channels
			h.channelsMu.Lock()
			for ch, subs := range h.channels {
				delete(subs, client.ID)
				if len(subs) == 0 {
					delete(h.channels, ch)
				}
			}
			h.channelsMu.Unlock()

			h.log.Info("Client disconnected", logger.Fields{
				"client_id": client.ID,
			})
		}
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Subscribe adds a client to a channel.
func (h *Hub) Subscribe(client *Client, channel string) {
	if channel == "" {
		return
	}

	h.channelsMu.Lock()
	defer h.channelsMu.Unlock()

	if _, ok := h.channels[channel]; !ok {
		h.channels[channel] = make(map[string]*Client)
	}
	h.channels[channel][client.ID] = client

	client.mu.Lock()
	client.Channels[channel] = true
	client.mu.Unlock()

	h.log.Debug("Client subscribed", logger.Fields{
		"client_id": client.ID,
		"channel":   channel,
	})
}

// Unsubscribe removes a client from a channel.
func (h *Hub) Unsubscribe(client *Client, channel string) {
	h.channelsMu.Lock()
	defer h.channelsMu.Unlock()

	if subs, ok := h.channels[channel]; ok {
		delete(subs, client.ID)
		if len(subs) == 0 {
			delete(h.channels, channel)
		}
	}

	client.mu.Lock()
	delete(client.Channels, channel)
	client.mu.Unlock()
}

// Publish sends a message to all clients subscribed to a channel.
// Optionally excludes the sender (by sender client ID).
func (h *Hub) Publish(channel, event string, data interface{}, excludeID string) {
	h.channelsMu.RLock()
	subs, ok := h.channels[channel]
	h.channelsMu.RUnlock()

	if !ok {
		return
	}

	msg := &Message{
		Type:    "message",
		Channel: channel,
		Event:   event,
		Data:    data,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for id, client := range subs {
		if id == excludeID {
			continue
		}
		select {
		case client.send <- msgBytes:
		default:
			// Client buffer full — skip
		}
	}
}

// PublishToAll sends a message to ALL connected clients (broadcast).
func (h *Hub) PublishToAll(event string, data interface{}) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	msg := &Message{
		Type:  "broadcast",
		Event: event,
		Data:  data,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, client := range h.clients {
		select {
		case client.send <- msgBytes:
		default:
		}
	}
}

// BroadcastPresence sends the current presence list for a channel to all subscribers.
func (h *Hub) BroadcastPresence(channel string) {
	h.channelsMu.RLock()
	subs, ok := h.channels[channel]
	h.channelsMu.RUnlock()

	if !ok {
		return
	}

	// Build presence list
	presence := make([]map[string]interface{}, 0, len(subs))
	for _, client := range subs {
		entry := map[string]interface{}{
			"client_id": client.ID,
			"user_id":   client.UserID,
		}
		if data, ok := client.GetMetadata("presence"); ok {
			entry["data"] = data
		}
		presence = append(presence, entry)
	}

	msg := &Message{
		Type:    "presence",
		Channel: channel,
		Data:    presence,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, client := range subs {
		select {
		case client.send <- msgBytes:
		default:
		}
	}
}

// GetChannelClients returns the number of clients in a channel.
func (h *Hub) GetChannelClients(channel string) int {
	h.channelsMu.RLock()
	defer h.channelsMu.RUnlock()
	return len(h.channels[channel])
}

// GetClientCount returns the total number of connected clients.
func (h *Hub) GetClientCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return len(h.clients)
}

// ListChannels returns all active channels and subscriber counts.
func (h *Hub) ListChannels() map[string]int {
	h.channelsMu.RLock()
	defer h.channelsMu.RUnlock()

	result := make(map[string]int, len(h.channels))
	for ch, subs := range h.channels {
		result[ch] = len(subs)
	}
	return result
}

// Stats returns hub statistics.
func (h *Hub) Stats() map[string]interface{} {
	return map[string]interface{}{
		"connected_clients": h.GetClientCount(),
		"active_channels":   len(h.ListChannels()),
		"channels":          h.ListChannels(),
	}
}
