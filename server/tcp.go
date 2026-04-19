// Package server implements a raw TCP-based HTTP/1.1 server for the Forge platform.
// Every byte is parsed by hand — no net/http abstractions are used.
package server

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Server is the core TCP server that accepts connections and processes HTTP requests.
type Server struct {
	config   *config.Config
	logger   *logger.Logger
	router   *Router
	listener net.Listener

	// Connection tracking for graceful shutdown
	activeConns sync.WaitGroup
	shutdown    atomic.Bool

	// Timeouts parsed from config strings
	readTimeout  time.Duration
	writeTimeout time.Duration
}

// New creates a new Server with the given configuration and logger.
func New(cfg *config.Config, log *logger.Logger) (*Server, error) {
	readTimeout, err := time.ParseDuration(cfg.Server.ReadTimeout)
	if err != nil {
		readTimeout = 30 * time.Second
	}

	writeTimeout, err := time.ParseDuration(cfg.Server.WriteTimeout)
	if err != nil {
		writeTimeout = 30 * time.Second
	}

	s := &Server{
		config:       cfg,
		logger:       log,
		router:       NewRouter(),
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}

	return s, nil
}

// Router returns the server's router for registering routes.
func (s *Server) Router() *Router {
	return s.router
}

// ListenAndServe starts the TCP listener and begins accepting connections.
// This method blocks until the server is shut down.
func (s *Server) ListenAndServe() error {
	addr := s.config.Address()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	s.logger.Info("Forge server started", logger.Fields{
		"address": addr,
		"pid":     fmt.Sprintf("%d", getPID()),
	})

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.shutdown.Load() {
				s.logger.Info("Server shutting down, stopped accepting connections")
				return nil
			}
			s.logger.Error("Failed to accept connection", logger.Fields{
				"error": err.Error(),
			})
			continue
		}

		s.activeConns.Add(1)
		go s.handleConnection(conn)
	}
}

// Shutdown gracefully stops the server.
// It stops accepting new connections and waits for active connections to finish.
func (s *Server) Shutdown(timeout time.Duration) error {
	s.shutdown.Store(true)
	s.logger.Info("Initiating graceful shutdown...")

	// Close the listener to stop accepting new connections
	if s.listener != nil {
		s.listener.Close()
	}

	// Wait for active connections to complete (with timeout)
	done := make(chan struct{})
	go func() {
		s.activeConns.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All connections drained, shutdown complete")
		return nil
	case <-time.After(timeout):
		s.logger.Warn("Shutdown timeout reached, forcing close")
		return fmt.Errorf("shutdown timed out after %v", timeout)
	}
}

// handleConnection processes a single TCP connection.
// It reads the raw bytes, parses the HTTP request, routes it, and writes the response.
func (s *Server) handleConnection(conn net.Conn) {
	defer s.activeConns.Done()
	defer conn.Close()

	// Set connection timeouts
	conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))

	// Parse the HTTP request from raw TCP bytes
	req, err := ParseHTTPRequest(conn, s.config.Server.MaxHeaderSize, s.config.Server.MaxBodySize)
	if err != nil {
		s.logger.Debug("Failed to parse HTTP request", logger.Fields{
			"error":  err.Error(),
			"remote": conn.RemoteAddr().String(),
		})
		// Send a 400 Bad Request response
		resp := NewResponse(400)
		resp.SetHeader("Content-Type", "text/plain")
		resp.SetBody([]byte("Bad Request: " + err.Error()))
		conn.Write(resp.Build())
		return
	}

	// Set the remote address on the request
	req.RemoteAddr = conn.RemoteAddr().String()

	// Create the response writer
	resp := NewResponse(200)

	// Create the request context
	ctx := NewContext(req, resp, conn)

	// Route the request through the middleware chain and handler
	s.router.ServeHTTP(ctx)

	// Write the response back over the TCP connection
	responseBytes := resp.Build()
	_, writeErr := conn.Write(responseBytes)
	if writeErr != nil {
		s.logger.Debug("Failed to write response", logger.Fields{
			"error":  writeErr.Error(),
			"remote": conn.RemoteAddr().String(),
		})
	}
}

// getPID returns the current process ID as a safe alternative to os.Getpid.
func getPID() int {
	// Using a simple approach to avoid importing os just for PID
	// The main package will set this during startup
	return pid
}

var pid int

// SetPID stores the process ID for logging. Called from main.go.
func SetPID(p int) {
	pid = p
}
