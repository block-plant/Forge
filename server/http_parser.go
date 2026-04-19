package server

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// Request represents a parsed HTTP/1.1 request.
type Request struct {
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD).
	Method string
	// Path is the URL path (without query string).
	Path string
	// RawPath is the original path before decoding.
	RawPath string
	// Query contains parsed query string parameters.
	Query url.Values
	// RawQuery is the unparsed query string.
	RawQuery string
	// Proto is the HTTP protocol version (e.g., "HTTP/1.1").
	Proto string
	// Headers is a map of header names (lowercased) to values.
	Headers map[string]string
	// Body is the raw request body.
	Body []byte
	// ContentLength is the parsed Content-Length header value.
	ContentLength int64
	// RemoteAddr is the address of the client.
	RemoteAddr string
	// Host is the Host header value.
	Host string
}

// GetHeader returns a header value by name (case-insensitive).
func (r *Request) GetHeader(name string) string {
	return r.Headers[strings.ToLower(name)]
}

// ContentType returns the Content-Type header value.
func (r *Request) ContentType() string {
	return r.GetHeader("content-type")
}

// ParseHTTPRequest reads from a connection and parses a raw HTTP/1.1 request.
// It enforces maximum sizes for headers and body to prevent abuse.
func ParseHTTPRequest(reader io.Reader, maxHeaderSize int, maxBodySize int64) (*Request, error) {
	br := bufio.NewReaderSize(reader, 4096)

	// Parse the request line (e.g., "GET /path?query HTTP/1.1\r\n")
	requestLine, err := readLine(br)
	if err != nil {
		return nil, fmt.Errorf("failed to read request line: %w", err)
	}

	method, rawPath, proto, err := parseRequestLine(requestLine)
	if err != nil {
		return nil, err
	}

	// Parse headers
	headers, headerBytes, err := parseHeaders(br, maxHeaderSize)
	if err != nil {
		return nil, err
	}
	_ = headerBytes

	// Parse the URL path and query string
	path, rawQuery, query, err := parsePath(rawPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	req := &Request{
		Method:   method,
		Path:     path,
		RawPath:  rawPath,
		Query:    query,
		RawQuery: rawQuery,
		Proto:    proto,
		Headers:  headers,
		Host:     headers["host"],
	}

	// Parse Content-Length and read the body
	if cl, ok := headers["content-length"]; ok {
		contentLength, err := strconv.ParseInt(cl, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid Content-Length: %w", err)
		}
		if contentLength > maxBodySize {
			return nil, fmt.Errorf("request body too large: %d bytes (max: %d)", contentLength, maxBodySize)
		}
		req.ContentLength = contentLength

		if contentLength > 0 {
			body := make([]byte, contentLength)
			_, err := io.ReadFull(br, body)
			if err != nil {
				return nil, fmt.Errorf("failed to read request body: %w", err)
			}
			req.Body = body
		}
	}

	return req, nil
}

// readLine reads a single line from the buffered reader, terminated by \r\n or \n.
func readLine(br *bufio.Reader) (string, error) {
	var line strings.Builder
	for {
		b, err := br.ReadByte()
		if err != nil {
			if line.Len() > 0 {
				return line.String(), nil
			}
			return "", err
		}
		if b == '\r' {
			// Peek for \n
			next, err := br.ReadByte()
			if err == nil && next != '\n' {
				br.UnreadByte()
			}
			break
		}
		if b == '\n' {
			break
		}
		line.WriteByte(b)
	}
	return line.String(), nil
}

// parseRequestLine parses "METHOD /path HTTP/1.1" into its components.
func parseRequestLine(line string) (method, path, proto string, err error) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("malformed request line: %q", line)
	}

	method = strings.ToUpper(parts[0])
	path = parts[1]
	proto = parts[2]

	// Validate method
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD":
		// Valid
	default:
		return "", "", "", fmt.Errorf("unsupported HTTP method: %s", method)
	}

	// Validate protocol
	if !strings.HasPrefix(proto, "HTTP/") {
		return "", "", "", fmt.Errorf("unsupported protocol: %s", proto)
	}

	return method, path, proto, nil
}

// parseHeaders reads and parses HTTP headers until an empty line is found.
// Header names are normalized to lowercase.
func parseHeaders(br *bufio.Reader, maxHeaderSize int) (map[string]string, int, error) {
	headers := make(map[string]string)
	totalBytes := 0

	for {
		line, err := readLine(br)
		if err != nil {
			return nil, totalBytes, fmt.Errorf("failed to read header: %w", err)
		}

		totalBytes += len(line) + 2 // +2 for \r\n
		if totalBytes > maxHeaderSize {
			return nil, totalBytes, fmt.Errorf("headers too large: %d bytes (max: %d)", totalBytes, maxHeaderSize)
		}

		// Empty line marks end of headers
		if line == "" {
			break
		}

		// Split on first ":"
		colonIndex := strings.IndexByte(line, ':')
		if colonIndex < 1 {
			continue // Skip malformed headers
		}

		name := strings.ToLower(strings.TrimSpace(line[:colonIndex]))
		value := strings.TrimSpace(line[colonIndex+1:])
		headers[name] = value
	}

	return headers, totalBytes, nil
}

// parsePath splits a raw URL path into the path, query string, and parsed query values.
func parsePath(rawPath string) (string, string, url.Values, error) {
	// Split path and query
	path := rawPath
	rawQuery := ""

	if idx := strings.IndexByte(rawPath, '?'); idx >= 0 {
		path = rawPath[:idx]
		rawQuery = rawPath[idx+1:]
	}

	// URL-decode the path
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		decodedPath = path // Use raw path as fallback
	}

	// Normalize: ensure path starts with /
	if decodedPath == "" || decodedPath[0] != '/' {
		decodedPath = "/" + decodedPath
	}

	// Parse query string
	query, _ := url.ParseQuery(rawQuery)

	return decodedPath, rawQuery, query, nil
}
