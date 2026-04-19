// Package realtime implements a WebSocket-based real-time system for Forge.
// It provides hand-rolled RFC 6455 WebSocket framing, a pub/sub hub,
// document change streams, and presence tracking.
package realtime

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// WebSocket magic GUID (RFC 6455 §1.3)
const websocketGUID = "258EAFA5-E914-47DA-95CA-5AB5DC6E3B45"

// WebSocket opcodes (RFC 6455 §5.2)
const (
	OpText   byte = 0x1
	OpBinary byte = 0x2
	OpClose  byte = 0x8
	OpPing   byte = 0x9
	OpPong   byte = 0xA
)

// Close status codes (RFC 6455 §7.4.1)
const (
	CloseNormal       = 1000
	CloseGoingAway    = 1001
	CloseProtocolErr  = 1002
	CloseAbnormal     = 1006
)

// Frame represents a WebSocket frame.
type Frame struct {
	Fin     bool
	Opcode  byte
	Masked  bool
	Payload []byte
}

// WebSocket errors
var (
	ErrNotWebSocket     = errors.New("websocket: not a WebSocket upgrade request")
	ErrBadHandshake     = errors.New("websocket: bad handshake")
	ErrFrameTooLarge    = errors.New("websocket: frame exceeds maximum size")
	ErrConnectionClosed = errors.New("websocket: connection closed")
)

// Conn wraps a raw TCP connection with WebSocket frame reading and writing.
type Conn struct {
	conn       net.Conn
	mu         sync.Mutex // protects writes
	closed     bool
	closeMu    sync.Mutex
	maxMsgSize int64
}

// NewConn wraps a raw TCP connection as a WebSocket connection.
func NewConn(conn net.Conn, maxMsgSize int64) *Conn {
	if maxMsgSize <= 0 {
		maxMsgSize = 1024 * 1024 // 1MB default
	}
	return &Conn{
		conn:       conn,
		maxMsgSize: maxMsgSize,
	}
}

// UpgradeHTTP performs the WebSocket handshake on a raw TCP connection.
// Returns the response bytes to write (the caller writes them to the connection).
func UpgradeHTTP(key string) []byte {
	// Compute the accept key: SHA-1(key + magic GUID), base64 encoded
	h := sha1.New()
	h.Write([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Build the HTTP 101 response by hand
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n" +
		"\r\n"

	return []byte(resp)
}

// ReadFrame reads a single WebSocket frame from the connection.
func (c *Conn) ReadFrame() (*Frame, error) {
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Read the first 2 bytes (FIN, opcode, mask bit, payload length)
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, wrapErr(err)
	}

	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	payloadLen := uint64(header[1] & 0x7F)

	// Extended payload length
	switch {
	case payloadLen == 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(c.conn, ext); err != nil {
			return nil, wrapErr(err)
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext))
	case payloadLen == 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(c.conn, ext); err != nil {
			return nil, wrapErr(err)
		}
		payloadLen = binary.BigEndian.Uint64(ext)
	}

	if int64(payloadLen) > c.maxMsgSize {
		return nil, ErrFrameTooLarge
	}

	// Read masking key (4 bytes, only if masked)
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.conn, maskKey[:]); err != nil {
			return nil, wrapErr(err)
		}
	}

	// Read payload
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(c.conn, payload); err != nil {
			return nil, wrapErr(err)
		}
	}

	// Unmask payload (RFC 6455 §5.3)
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return &Frame{
		Fin:     fin,
		Opcode:  opcode,
		Masked:  masked,
		Payload: payload,
	}, nil
}

// WriteFrame writes a WebSocket frame to the connection.
// Server-to-client frames are NOT masked (RFC 6455 §5.1).
func (c *Conn) WriteFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	var frame []byte
	payloadLen := len(payload)

	// First byte: FIN bit + opcode
	firstByte := byte(0x80) | opcode // FIN=1

	// Second byte: payload length (no mask for server frames)
	if payloadLen < 126 {
		frame = make([]byte, 2+payloadLen)
		frame[0] = firstByte
		frame[1] = byte(payloadLen)
		copy(frame[2:], payload)
	} else if payloadLen < 65536 {
		frame = make([]byte, 4+payloadLen)
		frame[0] = firstByte
		frame[1] = 126
		binary.BigEndian.PutUint16(frame[2:4], uint16(payloadLen))
		copy(frame[4:], payload)
	} else {
		frame = make([]byte, 10+payloadLen)
		frame[0] = firstByte
		frame[1] = 127
		binary.BigEndian.PutUint64(frame[2:10], uint64(payloadLen))
		copy(frame[10:], payload)
	}

	_, err := c.conn.Write(frame)
	return err
}

// WriteText sends a text message.
func (c *Conn) WriteText(msg string) error {
	return c.WriteFrame(OpText, []byte(msg))
}

// WriteJSON sends a JSON message.
func (c *Conn) WriteJSON(data []byte) error {
	return c.WriteFrame(OpText, data)
}

// WritePong sends a pong response to a ping.
func (c *Conn) WritePong(payload []byte) error {
	return c.WriteFrame(OpPong, payload)
}

// Close sends a close frame and closes the underlying connection.
func (c *Conn) Close(code int, reason string) error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Send close frame with status code
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload[:2], uint16(code))
	copy(payload[2:], reason)

	c.WriteFrame(OpClose, payload)
	return c.conn.Close()
}

// IsClosed returns whether the connection is closed.
func (c *Conn) IsClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}

// RemoteAddr returns the remote address.
func (c *Conn) RemoteAddr() string {
	return c.conn.RemoteAddr().String()
}

// wrapErr wraps io errors into WebSocket errors.
func wrapErr(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrConnectionClosed
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return fmt.Errorf("websocket: connection timed out: %w", err)
	}
	return fmt.Errorf("websocket: %w", err)
}
