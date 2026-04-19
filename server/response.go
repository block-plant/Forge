package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Response represents an HTTP response being constructed.
type Response struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Headers contains response headers.
	Headers map[string]string
	// Body is the response body bytes.
	Body []byte
	// written tracks whether the response has been finalized.
	written bool
}

// statusTexts maps HTTP status codes to their reason phrases.
var statusTexts = map[int]string{
	100: "Continue",
	101: "Switching Protocols",
	200: "OK",
	201: "Created",
	204: "No Content",
	301: "Moved Permanently",
	302: "Found",
	304: "Not Modified",
	307: "Temporary Redirect",
	308: "Permanent Redirect",
	400: "Bad Request",
	401: "Unauthorized",
	403: "Forbidden",
	404: "Not Found",
	405: "Method Not Allowed",
	408: "Request Timeout",
	409: "Conflict",
	413: "Payload Too Large",
	415: "Unsupported Media Type",
	422: "Unprocessable Entity",
	429: "Too Many Requests",
	500: "Internal Server Error",
	502: "Bad Gateway",
	503: "Service Unavailable",
	504: "Gateway Timeout",
}

// StatusText returns the text for an HTTP status code.
func StatusText(code int) string {
	text, ok := statusTexts[code]
	if !ok {
		return "Unknown"
	}
	return text
}

// NewResponse creates a new Response with the given status code and default headers.
func NewResponse(statusCode int) *Response {
	return &Response{
		StatusCode: statusCode,
		Headers: map[string]string{
			"server":     "Forge/1.0",
			"date":       time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"),
			"connection": "close",
		},
	}
}

// SetStatus sets the HTTP status code.
func (r *Response) SetStatus(code int) {
	r.StatusCode = code
}

// SetHeader sets a response header (overwrites if exists).
func (r *Response) SetHeader(name, value string) {
	r.Headers[strings.ToLower(name)] = value
}

// GetHeader returns a response header value.
func (r *Response) GetHeader(name string) string {
	return r.Headers[strings.ToLower(name)]
}

// SetBody sets the response body and updates Content-Length.
func (r *Response) SetBody(body []byte) {
	r.Body = body
	r.Headers["content-length"] = fmt.Sprintf("%d", len(body))
}

// Build serializes the response into raw HTTP/1.1 bytes ready to write to the connection.
// Format:
//
//	HTTP/1.1 200 OK\r\n
//	Header-Name: value\r\n
//	\r\n
//	body bytes
func (r *Response) Build() []byte {
	var b strings.Builder

	// Status line
	statusText := StatusText(r.StatusCode)
	b.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.StatusCode, statusText))

	// Headers
	for name, value := range r.Headers {
		// Capitalize header name for standards compliance
		canonicalName := canonicalHeaderName(name)
		b.WriteString(fmt.Sprintf("%s: %s\r\n", canonicalName, value))
	}

	// End of headers
	b.WriteString("\r\n")

	// Convert header part to bytes
	headerBytes := []byte(b.String())

	// Append body if present
	if len(r.Body) > 0 {
		result := make([]byte, len(headerBytes)+len(r.Body))
		copy(result, headerBytes)
		copy(result[len(headerBytes):], r.Body)
		return result
	}

	return headerBytes
}

// canonicalHeaderName converts a lowercased header name to canonical HTTP format.
// e.g., "content-type" → "Content-Type"
func canonicalHeaderName(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "-")
}

// Predefined response helpers

// JSONResponse creates a response with JSON body and appropriate Content-Type.
func JSONResponse(statusCode int, data interface{}) *Response {
	resp := NewResponse(statusCode)
	resp.SetHeader("Content-Type", "application/json; charset=utf-8")

	body, err := json.Marshal(data)
	if err != nil {
		// Fallback to error response
		resp.SetStatus(500)
		body = []byte(`{"error":"internal server error","message":"failed to marshal response"}`)
	}

	resp.SetBody(body)
	return resp
}

// TextResponse creates a response with plain text body.
func TextResponse(statusCode int, text string) *Response {
	resp := NewResponse(statusCode)
	resp.SetHeader("Content-Type", "text/plain; charset=utf-8")
	resp.SetBody([]byte(text))
	return resp
}

// ErrorResponse creates a standardized JSON error response.
func ErrorResponse(statusCode int, message string) *Response {
	return JSONResponse(statusCode, map[string]interface{}{
		"error":   StatusText(statusCode),
		"message": message,
		"status":  statusCode,
	})
}

// RedirectResponse creates a 302 redirect response.
func RedirectResponse(location string) *Response {
	resp := NewResponse(302)
	resp.SetHeader("Location", location)
	resp.SetBody([]byte("Redirecting to " + location))
	return resp
}
