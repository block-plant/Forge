package server

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/utils"
)

// CORSConfig holds configuration for the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is a list of origins that may access the resource.
	// Use ["*"] to allow all origins.
	AllowOrigins []string
	// AllowMethods is a list of allowed HTTP methods.
	AllowMethods []string
	// AllowHeaders is a list of allowed request headers.
	AllowHeaders []string
	// ExposeHeaders is a list of headers that can be exposed to the browser.
	ExposeHeaders []string
	// AllowCredentials indicates whether the response to the request can include credentials.
	AllowCredentials bool
	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached.
	MaxAge int
}

// DefaultCORSConfig returns a permissive CORS configuration for development.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Requested-With", "X-Request-ID",
		},
		ExposeHeaders:    []string{"Content-Length", "Content-Type", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing.
func CORSMiddleware(cfg CORSConfig) HandlerFunc {
	allowOrigins := strings.Join(cfg.AllowOrigins, ", ")
	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := fmt.Sprintf("%d", cfg.MaxAge)

	return func(ctx *Context) {
		origin := ctx.Header("origin")

		// Check if origin is allowed
		allowed := false
		for _, o := range cfg.AllowOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed && origin != "" {
			if cfg.AllowOrigins[0] == "*" && !cfg.AllowCredentials {
				ctx.SetResponseHeader("Access-Control-Allow-Origin", "*")
			} else {
				ctx.SetResponseHeader("Access-Control-Allow-Origin", origin)
				ctx.SetResponseHeader("Vary", "Origin")
			}
			ctx.SetResponseHeader("Access-Control-Allow-Methods", allowMethods)
			ctx.SetResponseHeader("Access-Control-Allow-Headers", allowHeaders)
			ctx.SetResponseHeader("Access-Control-Expose-Headers", exposeHeaders)
			ctx.SetResponseHeader("Access-Control-Max-Age", maxAge)

			if cfg.AllowCredentials {
				ctx.SetResponseHeader("Access-Control-Allow-Credentials", "true")
			}
		} else if origin != "" {
			// Origin not allowed — set allow-origin to empty to deny
			_ = allowOrigins // used in error messages if needed
		}

		// Handle preflight requests
		if ctx.Method() == "OPTIONS" {
			ctx.Status(204).NoContent()
			ctx.Abort()
			return
		}

		ctx.Next()
	}
}

// LoggerMiddleware logs every request with method, path, status, and duration.
func LoggerMiddleware(log *logger.Logger) HandlerFunc {
	return func(ctx *Context) {
		start := time.Now()
		path := ctx.Path()
		method := ctx.Method()

		// Process request
		ctx.Next()

		// Log after request completes
		duration := time.Since(start)
		status := ctx.Response.StatusCode

		fields := logger.Fields{
			"method":   method,
			"path":     path,
			"status":   status,
			"duration": utils.FormatDuration(duration),
			"remote":   ctx.RemoteAddr(),
		}

		if reqID := ctx.GetString("request_id"); reqID != "" {
			fields["request_id"] = reqID
		}

		if status >= 500 {
			log.Error("Request error", fields)
		} else if status >= 400 {
			log.Warn("Request warning", fields)
		} else {
			log.Info("Request", fields)
		}
	}
}

// RecoveryMiddleware catches panics and returns a 500 Internal Server Error.
// It prevents the entire server from crashing due to an unhandled panic in a handler.
func RecoveryMiddleware(log *logger.Logger) HandlerFunc {
	return func(ctx *Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				log.Error("Panic recovered", logger.Fields{
					"error": fmt.Sprintf("%v", r),
					"stack": stack,
					"path":  ctx.Path(),
				})

				ctx.JSON(500, map[string]interface{}{
					"error":   "Internal Server Error",
					"message": "An unexpected error occurred",
					"status":  500,
				})
			}
		}()
		ctx.Next()
	}
}

// RequestIDMiddleware injects a unique request ID into every request.
// The ID is available via ctx.GetString("request_id") and is also
// sent in the X-Request-ID response header.
func RequestIDMiddleware() HandlerFunc {
	return func(ctx *Context) {
		// Check if the client sent a request ID
		requestID := ctx.Header("x-request-id")
		if requestID == "" {
			requestID = utils.MustGenerateUUID()
		}

		ctx.Set("request_id", requestID)
		ctx.SetResponseHeader("X-Request-ID", requestID)

		ctx.Next()
	}
}

// RateLimitConfig holds configuration for the rate limiter.
type RateLimitConfig struct {
	// RequestsPerSecond is the maximum number of requests per second per IP.
	RequestsPerSecond float64
	// BurstSize is the maximum burst of requests allowed.
	BurstSize int
}

// tokenBucket implements a per-IP token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per nanosecond
	lastRefill time.Time
}

// RateLimitMiddleware limits requests per IP using a token bucket algorithm.
func RateLimitMiddleware(cfg RateLimitConfig) HandlerFunc {
	var mu sync.Mutex
	buckets := make(map[string]*tokenBucket)

	// Cleanup goroutine: remove stale buckets every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			for ip, bucket := range buckets {
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(ctx *Context) {
		ip := extractIP(ctx.RemoteAddr())

		mu.Lock()

		bucket, ok := buckets[ip]
		if !ok {
			bucket = &tokenBucket{
				tokens:     float64(cfg.BurstSize),
				maxTokens:  float64(cfg.BurstSize),
				refillRate: cfg.RequestsPerSecond / float64(time.Second),
				lastRefill: time.Now(),
			}
			buckets[ip] = bucket
		}

		// Refill tokens
		now := time.Now()
		elapsed := now.Sub(bucket.lastRefill)
		bucket.tokens += float64(elapsed) * bucket.refillRate
		if bucket.tokens > bucket.maxTokens {
			bucket.tokens = bucket.maxTokens
		}
		bucket.lastRefill = now

		// Check if we have a token
		if bucket.tokens < 1 {
			mu.Unlock()
			ctx.SetResponseHeader("Retry-After", "1")
			ctx.JSON(429, map[string]interface{}{
				"error":   "Too Many Requests",
				"message": "Rate limit exceeded. Please try again later.",
				"status":  429,
			})
			ctx.Abort()
			return
		}

		bucket.tokens--
		mu.Unlock()

		ctx.Next()
	}
}

// extractIP extracts the IP address from a remote address string (ip:port).
func extractIP(remoteAddr string) string {
	// Try to find the last colon for IPv4 (ip:port) and IPv6 ([ip]:port)
	if idx := strings.LastIndex(remoteAddr, ":"); idx >= 0 {
		ip := remoteAddr[:idx]
		// Remove brackets for IPv6
		ip = strings.TrimPrefix(ip, "[")
		ip = strings.TrimSuffix(ip, "]")
		return ip
	}
	return remoteAddr
}
