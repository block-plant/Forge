package analytics

import (
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/logger"
)

// Collector handles automatic event collection from all Forge services.
// It provides a unified ingestion point that enriches events with
// server-side context before sending them to the Engine for persistence.
type Collector struct {
	engine *Engine
	log    *logger.Logger

	// counters for system-level metrics
	counters   map[string]*Counter
	countersMu sync.RWMutex
}

// Counter is an atomic counter for a specific metric.
type Counter struct {
	Name    string `json:"name"`
	Value   int64  `json:"value"`
	mu      sync.Mutex
}

// NewCollector creates a new event collector wired to the analytics engine.
func NewCollector(engine *Engine, log *logger.Logger) *Collector {
	c := &Collector{
		engine:   engine,
		log:      log.WithField("component", "collector"),
		counters: make(map[string]*Counter),
	}

	// Initialize built-in counters
	builtins := []string{
		"auth.signups", "auth.logins", "auth.failures",
		"db.reads", "db.writes", "db.deletes",
		"storage.uploads", "storage.downloads",
		"functions.invocations", "functions.errors",
		"http.requests", "http.errors",
	}
	for _, name := range builtins {
		c.counters[name] = &Counter{Name: name}
	}

	// Start periodic flush of counters to events
	go c.flushCountersLoop()

	return c
}

// TrackAuth records an authentication event.
func (c *Collector) TrackAuth(action, userID, provider string) {
	c.engine.Track(Event{
		Name:      "auth." + action,
		Timestamp: time.Now(),
		UserID:    userID,
		Properties: map[string]interface{}{
			"provider": provider,
		},
		Context: EventContext{},
	})

	switch action {
	case "signup":
		c.increment("auth.signups")
	case "login":
		c.increment("auth.logins")
	case "failure":
		c.increment("auth.failures")
	}
}

// TrackDB records a database operation event.
func (c *Collector) TrackDB(operation, collection, docID string) {
	c.engine.Track(Event{
		Name:      "db." + operation,
		Timestamp: time.Now(),
		Properties: map[string]interface{}{
			"collection": collection,
			"doc_id":     docID,
		},
		Context: EventContext{},
	})

	switch operation {
	case "read", "get", "list", "query":
		c.increment("db.reads")
	case "set", "create", "update":
		c.increment("db.writes")
	case "delete":
		c.increment("db.deletes")
	}
}

// TrackStorage records a storage operation event.
func (c *Collector) TrackStorage(operation, path string, sizeBytes int64) {
	c.engine.Track(Event{
		Name:      "storage." + operation,
		Timestamp: time.Now(),
		Properties: map[string]interface{}{
			"path":       path,
			"size_bytes": sizeBytes,
		},
		Context: EventContext{},
	})

	switch operation {
	case "upload":
		c.increment("storage.uploads")
	case "download":
		c.increment("storage.downloads")
	}
}

// TrackFunction records a function invocation event.
func (c *Collector) TrackFunction(funcName string, durationMs int64, errOccurred bool) {
	props := map[string]interface{}{
		"function":    funcName,
		"duration_ms": durationMs,
		"error":       errOccurred,
	}

	c.engine.Track(Event{
		Name:       "function.invocation",
		Timestamp:  time.Now(),
		Properties: props,
		Context:    EventContext{},
	})

	c.increment("functions.invocations")
	if errOccurred {
		c.increment("functions.errors")
	}
}

// TrackHTTP records an HTTP request event.
func (c *Collector) TrackHTTP(method, path string, statusCode int, durationMs int64) {
	c.engine.Track(Event{
		Name:      "http.request",
		Timestamp: time.Now(),
		Properties: map[string]interface{}{
			"method":      method,
			"path":        path,
			"status_code": statusCode,
			"duration_ms": durationMs,
		},
		Context: EventContext{},
	})

	c.increment("http.requests")
	if statusCode >= 400 {
		c.increment("http.errors")
	}
}

// GetCounters returns a snapshot of all counters.
func (c *Collector) GetCounters() map[string]int64 {
	c.countersMu.RLock()
	defer c.countersMu.RUnlock()

	result := make(map[string]int64, len(c.counters))
	for name, counter := range c.counters {
		counter.mu.Lock()
		result[name] = counter.Value
		counter.mu.Unlock()
	}
	return result
}

// increment atomically increments a named counter.
func (c *Collector) increment(name string) {
	c.countersMu.RLock()
	counter, ok := c.counters[name]
	c.countersMu.RUnlock()
	if !ok {
		return
	}
	counter.mu.Lock()
	counter.Value++
	counter.mu.Unlock()
}

// flushCountersLoop periodically emits counter summaries as events.
func (c *Collector) flushCountersLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		counters := c.GetCounters()
		c.engine.Track(Event{
			Name:       "system.counters",
			Timestamp:  time.Now(),
			Properties: toInterfaceMap(counters),
			Context:    EventContext{},
		})
	}
}

// toInterfaceMap converts map[string]int64 to map[string]interface{}.
func toInterfaceMap(m map[string]int64) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
