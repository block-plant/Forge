package storage

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// StreamWriter handles streaming file downloads with support for
// HTTP Range requests (partial content), ETag headers, and
// Cache-Control directives. This enables video streaming and
// resumable downloads without buffering entire files in memory.
type StreamWriter struct {
	conn net.Conn
}

// NewStreamWriter creates a stream writer for the given connection.
func NewStreamWriter(conn net.Conn) *StreamWriter {
	return &StreamWriter{conn: conn}
}

// RangeSpec represents a parsed HTTP Range request.
type RangeSpec struct {
	// Start is the first byte position (inclusive).
	Start int64
	// End is the last byte position (inclusive).
	End int64
}

// ParseRange parses an HTTP Range header value.
// Supports single range specs like "bytes=0-499" or "bytes=500-" or "bytes=-500".
func ParseRange(rangeHeader string, fileSize int64) (*RangeSpec, error) {
	if rangeHeader == "" {
		return nil, nil // No range requested
	}

	// Must start with "bytes="
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, fmt.Errorf("stream: unsupported range unit")
	}

	rangeSpec := rangeHeader[6:] // Remove "bytes="

	// We only support single ranges (not multipart ranges)
	if strings.Contains(rangeSpec, ",") {
		return nil, fmt.Errorf("stream: multiple ranges not supported")
	}

	dashIdx := strings.IndexByte(rangeSpec, '-')
	if dashIdx < 0 {
		return nil, fmt.Errorf("stream: invalid range format")
	}

	startStr := rangeSpec[:dashIdx]
	endStr := rangeSpec[dashIdx+1:]

	var start, end int64

	if startStr == "" {
		// Suffix range: "-500" means last 500 bytes
		suffix, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || suffix <= 0 {
			return nil, fmt.Errorf("stream: invalid suffix range")
		}
		start = fileSize - suffix
		if start < 0 {
			start = 0
		}
		end = fileSize - 1
	} else if endStr == "" {
		// Open-ended range: "500-" means from byte 500 to end
		var err error
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil || start < 0 {
			return nil, fmt.Errorf("stream: invalid range start")
		}
		end = fileSize - 1
	} else {
		// Explicit range: "500-999"
		var err error
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil || start < 0 {
			return nil, fmt.Errorf("stream: invalid range start")
		}
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil || end < 0 {
			return nil, fmt.Errorf("stream: invalid range end")
		}
	}

	// Validate bounds
	if start > end {
		return nil, fmt.Errorf("stream: range start exceeds end")
	}
	if start >= fileSize {
		return nil, fmt.Errorf("stream: range start exceeds file size")
	}
	if end >= fileSize {
		end = fileSize - 1
	}

	return &RangeSpec{Start: start, End: end}, nil
}

// StreamFile streams a file to the connection, supporting range requests.
// If rangeSpec is nil, the entire file is streamed.
// Returns the number of bytes written.
func StreamFile(filePath string, conn net.Conn, rangeSpec *RangeSpec, contentType string) (int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("stream: failed to open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stream: failed to stat file: %w", err)
	}

	fileSize := stat.Size()

	if rangeSpec != nil {
		// Partial content (206)
		return streamPartial(f, conn, rangeSpec, fileSize, contentType)
	}

	// Full content (200)
	return streamFull(f, conn, fileSize, contentType)
}

// streamFull writes a complete file response (200 OK).
func streamFull(f *os.File, conn net.Conn, fileSize int64, contentType string) (int64, error) {
	// Build HTTP 200 response headers
	headers := buildResponseHeaders(200, contentType, fileSize, nil, "")

	if _, err := conn.Write([]byte(headers)); err != nil {
		return 0, fmt.Errorf("stream: failed to write headers: %w", err)
	}

	// Stream file content
	written, err := io.Copy(conn, f)
	if err != nil {
		return written, fmt.Errorf("stream: failed to stream: %w", err)
	}

	return written, nil
}

// streamPartial writes a partial content response (206 Partial Content).
func streamPartial(f *os.File, conn net.Conn, spec *RangeSpec, fileSize int64, contentType string) (int64, error) {
	// Seek to start position
	if _, err := f.Seek(spec.Start, io.SeekStart); err != nil {
		return 0, fmt.Errorf("stream: failed to seek: %w", err)
	}

	contentLength := spec.End - spec.Start + 1
	contentRange := fmt.Sprintf("bytes %d-%d/%d", spec.Start, spec.End, fileSize)

	// Build HTTP 206 response headers
	headers := buildResponseHeaders(206, contentType, contentLength, &contentRange, "")

	if _, err := conn.Write([]byte(headers)); err != nil {
		return 0, fmt.Errorf("stream: failed to write headers: %w", err)
	}

	// Stream only the requested range
	written, err := io.CopyN(conn, f, contentLength)
	if err != nil && err != io.EOF {
		return written, fmt.Errorf("stream: failed to stream range: %w", err)
	}

	return written, nil
}

// buildResponseHeaders builds raw HTTP response headers.
func buildResponseHeaders(status int, contentType string, contentLength int64, contentRange *string, etag string) string {
	statusText := "OK"
	if status == 206 {
		statusText = "Partial Content"
	} else if status == 416 {
		statusText = "Range Not Satisfiable"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", status, statusText))
	b.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	b.WriteString(fmt.Sprintf("Content-Length: %d\r\n", contentLength))
	b.WriteString("Accept-Ranges: bytes\r\n")

	if contentRange != nil {
		b.WriteString(fmt.Sprintf("Content-Range: %s\r\n", *contentRange))
	}

	if etag != "" {
		b.WriteString(fmt.Sprintf("ETag: \"%s\"\r\n", etag))
	}

	// Cache control: cache immutable content-addressed blobs for 1 year
	b.WriteString("Cache-Control: public, max-age=31536000, immutable\r\n")
	b.WriteString("Connection: close\r\n")
	b.WriteString("\r\n")

	return b.String()
}

// GenerateETag creates an ETag from the content hash.
func GenerateETag(hash string) string {
	if len(hash) > 16 {
		return hash[:16]
	}
	return hash
}
