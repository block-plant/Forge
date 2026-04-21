package analytics

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/ayushkunwarsingh/forge/server"
)

// RegisterRoutes registers all analytics HTTP endpoints on the router.
func RegisterRoutes(router *server.Router, engine *Engine) {
	g := router.Group("/analytics")

	// Track a single event
	g.POST("/track", handleTrack(engine))

	// Track multiple events (batch)
	g.POST("/batch", handleBatch(engine))

	// Stats endpoint
	g.GET("/stats", handleStats(engine))
}

// handleTrack parses and queues a single analytics event.
func handleTrack(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var payload struct {
			ID         string                 `json:"id"`
			Name       string                 `json:"name"`
			UserID     string                 `json:"user_id"`
			SessionID  string                 `json:"session_id"`
			Properties map[string]interface{} `json:"properties"`
		}

		if err := ctx.BindJSON(&payload); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if payload.Name == "" {
			ctx.Error(400, "Event name is required")
			return
		}

		// Auto-generate ID if missing
		if payload.ID == "" {
			payload.ID = generateEventID()
		}

		// Build network context
		eventCtx := EventContext{
			IP:        ctx.Request.RemoteAddr, // Might need real IP parsing via headers later
			UserAgent: ctx.Header("User-Agent"),
			Referrer:  ctx.Header("Referer"),
		}

		// Extract authenticated UserID if a valid token was parsed by global middleware
		if uid := ctx.GetString("auth_uid"); uid != "" {
			payload.UserID = uid
		}

		event := Event{
			ID:         payload.ID,
			Name:       payload.Name,
			Timestamp:  time.Now().UTC(),
			UserID:     payload.UserID,
			SessionID:  payload.SessionID,
			Properties: payload.Properties,
			Context:    eventCtx,
		}

		if err := engine.Track(event); err != nil {
			// 503 Service Unavailable if buffer is full
			ctx.Error(503, err.Error())
			return
		}

		ctx.JSON(202, map[string]interface{}{
			"status": "queued",
			"id":     event.ID,
		})
	}
}

// handleBatch tracks multiple events in a single payload.
func handleBatch(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var payload struct {
			Events []struct {
				ID         string                 `json:"id"`
				Name       string                 `json:"name"`
				Timestamp  string                 `json:"timestamp"`
				UserID     string                 `json:"user_id"`
				SessionID  string                 `json:"session_id"`
				Properties map[string]interface{} `json:"properties"`
			} `json:"events"`
		}

		if err := ctx.BindJSON(&payload); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if len(payload.Events) == 0 {
			ctx.Error(400, "No events provided")
			return
		}

		// Auth context for the whole batch
		authUserID := ctx.GetString("auth_uid")

		eventCtx := EventContext{
			IP:        ctx.Request.RemoteAddr,
			UserAgent: ctx.Header("User-Agent"),
			Referrer:  ctx.Header("Referer"),
		}

		accepted := 0
		for _, e := range payload.Events {
			if e.Name == "" {
				continue
			}

			id := e.ID
			if id == "" {
				id = generateEventID()
			}

			// Parse timestamp or default to now
			ts := time.Now().UTC()
			if e.Timestamp != "" {
				if parsed, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
					ts = parsed.UTC()
				}
			}

			userID := e.UserID
			if userID == "" && authUserID != "" {
				userID = authUserID
			}

			event := Event{
				ID:         id,
				Name:       e.Name,
				Timestamp:  ts,
				UserID:     userID,
				SessionID:  e.SessionID,
				Properties: e.Properties,
				Context:    eventCtx,
			}

			if err := engine.Track(event); err == nil {
				accepted++
			}
		}

		ctx.JSON(202, map[string]interface{}{
			"status":   "queued",
			"accepted": accepted,
			"rejected": len(payload.Events) - accepted,
		})
	}
}

// handleStats provides engine health and buffer metrics.
func handleStats(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.JSON(200, map[string]interface{}{
			"status": "ok",
			"buffer": map[string]interface{}{
				"capacity": cap(engine.buffer),
				"used":     len(engine.buffer),
			},
			"config": map[string]interface{}{
				"flush_interval": engine.flushInterval.String(),
			},
		})
	}
}

// generateEventID creates a random 16-byte hex UUID-like string.
func generateEventID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
