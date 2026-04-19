package realtime

import (
	"strings"
	"time"

	"github.com/ayushkunwarsingh/forge/server"
)

// RegisterRoutes registers the real-time WebSocket and REST endpoints.
func RegisterRoutes(router *server.Router, hub *Hub) {
	// WebSocket upgrade endpoint
	router.GET("/realtime/ws", handleWebSocket(hub))

	// REST API for realtime status
	rt := router.Group("/realtime")
	rt.GET("/stats", handleStats(hub))
	rt.GET("/channels", handleChannels(hub))
	rt.POST("/publish", handlePublish(hub))
}

// handleWebSocket handles the WebSocket upgrade and client setup.
func handleWebSocket(hub *Hub) server.HandlerFunc {
	return func(ctx *server.Context) {
		// Validate the WebSocket upgrade headers
		upgrade := ctx.Header("Upgrade")
		connection := ctx.Header("Connection")
		wsKey := ctx.Header("Sec-WebSocket-Key")

		if !strings.EqualFold(upgrade, "websocket") ||
			!containsToken(connection, "upgrade") ||
			wsKey == "" {
			ctx.Error(400, "Not a WebSocket upgrade request")
			return
		}

		// Hijack the TCP connection — from here on, we own it
		rawConn := ctx.Hijack()

		// Remove read/write deadlines for the persistent connection
		rawConn.SetReadDeadline(zeroTime)
		rawConn.SetWriteDeadline(zeroTime)

		// Send the HTTP 101 Switching Protocols response
		upgradeResp := UpgradeHTTP(wsKey)
		if _, err := rawConn.Write(upgradeResp); err != nil {
			rawConn.Close()
			return
		}

		// Wrap in WebSocket connection
		wsConn := NewConn(rawConn, 1024*1024) // 1MB max message

		// Extract user ID from auth context if available
		userID := ctx.GetString("user_id")

		// Create client and register with hub
		client := NewClient(wsConn, hub, userID)
		hub.Register(client)

		// Start read/write pumps
		go client.WritePump()
		client.ReadPump() // blocking — runs until disconnect
	}
}

// handleStats handles GET /realtime/stats
func handleStats(hub *Hub) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.JSON(200, hub.Stats())
	}
}

// handleChannels handles GET /realtime/channels
func handleChannels(hub *Hub) server.HandlerFunc {
	return func(ctx *server.Context) {
		channels := hub.ListChannels()
		result := make([]map[string]interface{}, 0, len(channels))
		for name, count := range channels {
			result = append(result, map[string]interface{}{
				"channel":     name,
				"subscribers": count,
			})
		}
		ctx.JSON(200, map[string]interface{}{
			"channels": result,
			"total":    len(result),
		})
	}
}

// handlePublish handles POST /realtime/publish (server-side publish)
func handlePublish(hub *Hub) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Channel string      `json:"channel"`
			Event   string      `json:"event"`
			Data    interface{} `json:"data"`
		}

		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if body.Channel == "" || body.Event == "" {
			ctx.Error(400, "Channel and event are required")
			return
		}

		hub.Publish(body.Channel, body.Event, body.Data, "")

		ctx.JSON(200, map[string]interface{}{
			"message":     "Published",
			"channel":     body.Channel,
			"event":       body.Event,
			"subscribers": hub.GetChannelClients(body.Channel),
		})
	}
}

// containsToken checks if a comma-separated header value contains a specific token.
func containsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// zeroTime is used to clear deadlines on hijacked connections.
var zeroTime time.Time
