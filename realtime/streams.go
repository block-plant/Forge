package realtime

import (
	"fmt"
	"strings"

	"github.com/ayushkunwarsingh/forge/database"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Streams bridges the database change events to the real-time WebSocket hub.
// It automatically broadcasts document changes to clients subscribed to
// the relevant collection or document channels.
//
// Channel naming convention:
//   - "documents:users"          → all changes in the "users" collection
//   - "documents:users:user123"  → changes to a specific document
type Streams struct {
	hub *Hub
	log *logger.Logger
}

// NewStreams creates a new document change stream bridge.
func NewStreams(hub *Hub, engine *database.Engine, log *logger.Logger) *Streams {
	s := &Streams{
		hub: hub,
		log: log.WithField("service", "streams"),
	}

	// Register as a database change listener
	engine.OnChange(s.onDatabaseChange)

	log.Info("Document change streams initialized")
	return s
}

// onDatabaseChange is called when any document in the database changes.
// It publishes the change to subscribers of both the collection channel
// and the specific document channel.
func (s *Streams) onDatabaseChange(event *database.ChangeEvent) {
	// Build the collection-level channel name
	collectionChannel := fmt.Sprintf("documents:%s", event.Collection)

	// Build the document-level channel name
	documentChannel := fmt.Sprintf("documents:%s:%s", event.Collection, event.DocumentID)

	// Payload to send
	payload := map[string]interface{}{
		"type":        event.Type,
		"collection":  event.Collection,
		"document_id": event.DocumentID,
		"timestamp":   event.Timestamp,
	}
	if event.Data != nil {
		payload["data"] = event.Data
	}

	// Publish to collection subscribers
	s.hub.Publish(collectionChannel, "change", payload, "")

	// Publish to document subscribers
	s.hub.Publish(documentChannel, "change", payload, "")
}

// IsDocumentChannel checks if a channel name follows the document stream pattern.
func IsDocumentChannel(channel string) bool {
	return strings.HasPrefix(channel, "documents:")
}

// ParseDocumentChannel extracts collection and optional document ID from a channel name.
func ParseDocumentChannel(channel string) (collection string, docID string, ok bool) {
	if !strings.HasPrefix(channel, "documents:") {
		return "", "", false
	}

	parts := strings.SplitN(channel[len("documents:"):], ":", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}

	collection = parts[0]
	if len(parts) > 1 {
		docID = parts[1]
	}
	return collection, docID, true
}
