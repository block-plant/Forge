package realtime

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/utils"
)

// Client represents a connected WebSocket client.
type Client struct {
	ID         string
	UserID     string // empty if not authenticated
	Conn       *Conn
	Hub        *Hub
	Channels   map[string]bool // channels this client is subscribed to
	mu         sync.RWMutex
	send       chan []byte
	done       chan struct{}
	metadata   map[string]interface{} // arbitrary client metadata
}

// NewClient creates a new WebSocket client.
func NewClient(conn *Conn, hub *Hub, userID string) *Client {
	return &Client{
		ID:       utils.MustGenerateUUID(),
		UserID:   userID,
		Conn:     conn,
		Hub:      hub,
		Channels: make(map[string]bool),
		send:     make(chan []byte, 256),
		done:     make(chan struct{}),
		metadata: make(map[string]interface{}),
	}
}

// Message represents a real-time message sent over WebSocket.
type Message struct {
	Type    string                 `json:"type"`    // subscribe, unsubscribe, publish, presence, ack, error
	Channel string                `json:"channel,omitempty"`
	Event   string                `json:"event,omitempty"`
	Data    interface{}            `json:"data,omitempty"`
	ID      string                `json:"id,omitempty"` // message ID for ack
}

// ReadPump reads messages from the WebSocket and processes them.
// Runs in its own goroutine per client.
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close(CloseGoingAway, "")
		close(c.done)
	}()

	for {
		frame, err := c.Conn.ReadFrame()
		if err != nil {
			return
		}

		switch frame.Opcode {
		case OpText:
			c.handleMessage(frame.Payload)
		case OpPing:
			c.Conn.WritePong(frame.Payload)
		case OpClose:
			return
		}
	}
}

// WritePump writes messages to the WebSocket from the send channel.
// Runs in its own goroutine per client.
func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second) // ping interval
	defer func() {
		ticker.Stop()
		c.Conn.Close(CloseGoingAway, "")
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Hub closed the channel
				return
			}
			if err := c.Conn.WriteJSON(msg); err != nil {
				return
			}

		case <-ticker.C:
			if err := c.Conn.WriteFrame(OpPing, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}

// Send queues a message for sending to this client.
func (c *Client) Send(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
		// Client buffer full — drop message or disconnect
	}
}

// SetMetadata sets a metadata key-value pair.
func (c *Client) SetMetadata(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metadata[key] = value
}

// GetMetadata gets a metadata value.
func (c *Client) GetMetadata(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.metadata[key]
	return v, ok
}

// handleMessage processes an incoming message from the client.
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.Send(&Message{
			Type: "error",
			Data: "Invalid message format",
		})
		return
	}

	switch msg.Type {
	case "subscribe":
		c.Hub.Subscribe(c, msg.Channel)
		c.Send(&Message{
			Type:    "ack",
			ID:      msg.ID,
			Channel: msg.Channel,
			Event:   "subscribed",
		})

	case "unsubscribe":
		c.Hub.Unsubscribe(c, msg.Channel)
		c.Send(&Message{
			Type:    "ack",
			ID:      msg.ID,
			Channel: msg.Channel,
			Event:   "unsubscribed",
		})

	case "publish":
		c.Hub.Publish(msg.Channel, msg.Event, msg.Data, c.ID)
		c.Send(&Message{
			Type:  "ack",
			ID:    msg.ID,
			Event: "published",
		})

	case "presence":
		c.SetMetadata("presence", msg.Data)
		c.Hub.BroadcastPresence(msg.Channel)

	default:
		c.Send(&Message{
			Type: "error",
			Data: "Unknown message type: " + msg.Type,
		})
	}
}
