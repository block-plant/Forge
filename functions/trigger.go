package functions

import (
	"sync"

	"github.com/ayushkunwarsingh/forge/logger"
)

// TriggerManager manages function triggers — HTTP, DB events, Auth events.
// It acts as a dispatcher: when an event occurs, it finds matching functions
// and executes them via the Runtime.
type TriggerManager struct {
	deployer *Deployer
	runtime  *Runtime
	log      *logger.Logger

	// listeners stores event listeners: eventKey → []callback
	listeners map[string][]TriggerCallback
	mu        sync.RWMutex
}

// TriggerCallback is called when a trigger fires.
type TriggerCallback func(payload map[string]interface{})

// TriggerEvent represents an event that can fire triggers.
type TriggerEvent struct {
	// Type is the event type: "db", "auth", "http".
	Type string `json:"type"`
	// Event is the specific event: "create", "update", "delete", "signup", "signin".
	Event string `json:"event"`
	// Collection is the DB collection (for db triggers).
	Collection string `json:"collection,omitempty"`
	// Payload is the event data passed to the function.
	Payload map[string]interface{} `json:"payload"`
}

// NewTriggerManager creates a new trigger manager.
func NewTriggerManager(deployer *Deployer, runtime *Runtime, log *logger.Logger) *TriggerManager {
	return &TriggerManager{
		deployer:  deployer,
		runtime:   runtime,
		log:       log,
		listeners: make(map[string][]TriggerCallback),
	}
}

// Fire dispatches an event to all matching function triggers.
// Functions are executed asynchronously.
func (tm *TriggerManager) Fire(event TriggerEvent) {
	// Find matching functions
	functions := tm.deployer.GetByTrigger(event.Type, "", event.Event, event.Collection)

	if len(functions) == 0 {
		return
	}

	tm.log.Info("Trigger fired", logger.Fields{
		"type":      event.Type,
		"event":     event.Event,
		"functions": len(functions),
	})

	// Execute each matching function asynchronously
	for _, fn := range functions {
		go tm.executeTriggered(fn, event)
	}

	// Also notify registered listeners
	tm.mu.RLock()
	key := triggerKey(event.Type, event.Event, event.Collection)
	if callbacks, ok := tm.listeners[key]; ok {
		for _, cb := range callbacks {
			go cb(event.Payload)
		}
	}
	tm.mu.RUnlock()
}

// FireDBEvent is a convenience method for database triggers.
func (tm *TriggerManager) FireDBEvent(collection, event string, data map[string]interface{}) {
	tm.Fire(TriggerEvent{
		Type:       "db",
		Event:      event,
		Collection: collection,
		Payload: map[string]interface{}{
			"collection": collection,
			"event":      event,
			"data":       data,
		},
	})
}

// FireAuthEvent is a convenience method for auth triggers.
func (tm *TriggerManager) FireAuthEvent(event string, userData map[string]interface{}) {
	tm.Fire(TriggerEvent{
		Type:  "auth",
		Event: event,
		Payload: map[string]interface{}{
			"event": event,
			"user":  userData,
		},
	})
}

// OnEvent registers a callback for a specific event.
func (tm *TriggerManager) OnEvent(eventType, event, collection string, cb TriggerCallback) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := triggerKey(eventType, event, collection)
	tm.listeners[key] = append(tm.listeners[key], cb)
}

// executeTriggered runs a function in response to a trigger event.
func (tm *TriggerManager) executeTriggered(fn *Function, event TriggerEvent) {
	req := &ExecRequest{
		FunctionName: fn.Name,
		Trigger:      event.Type,
		Payload:      event.Payload,
	}

	result := tm.runtime.Execute(fn, req)

	if !result.Success {
		tm.log.Error("Triggered function failed", logger.Fields{
			"function": fn.Name,
			"trigger":  event.Type,
			"event":    event.Event,
			"error":    result.Error,
		})
	}
}

// triggerKey creates a unique key for a trigger event.
func triggerKey(eventType, event, collection string) string {
	if collection != "" {
		return eventType + ":" + event + ":" + collection
	}
	return eventType + ":" + event
}
