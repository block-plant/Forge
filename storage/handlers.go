package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/server"
)

// RegisterRoutes registers all storage HTTP endpoints on the router.
func RegisterRoutes(router *server.Router, engine *Engine) {
	g := router.Group("/storage")

	// File upload (single file, body is file content)
	g.POST("/upload/*path", handleUpload(engine))

	// File download
	g.GET("/object/*path", handleDownload(engine))

	// File deletion
	g.DELETE("/object/*path", handleDelete(engine))

	// List files
	g.GET("/list/*path", handleList(engine))
	g.GET("/list", handleList(engine))

	// File metadata
	g.GET("/metadata/*path", handleGetMetadata(engine))
	g.PUT("/metadata/*path", handleUpdateMetadata(engine))

	// Signed URL generation
	g.POST("/signed-url", handleSignedURL(engine))

	// Chunked upload endpoints
	g.POST("/upload-chunk/init", handleChunkInit(engine))
	g.POST("/upload-chunk/add", handleChunkAdd(engine))
	g.POST("/upload-chunk/complete", handleChunkComplete(engine))
	g.DELETE("/upload-chunk/:uploadId", handleChunkCancel(engine))

	// Storage stats
	g.GET("/stats", handleStats(engine))
}

// handleUpload handles single-file uploads via POST body.
// The file path comes from the URL, content from the request body.
func handleUpload(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		filePath := ctx.Param("path")
		if filePath == "" {
			ctx.Error(400, "File path is required")
			return
		}

		body := ctx.BodyBytes()
		if len(body) == 0 {
			ctx.Error(400, "Request body is empty")
			return
		}

		// Check file size limit
		if int64(len(body)) > engine.cfg.Storage.MaxFileSize {
			ctx.Error(413, fmt.Sprintf("File size exceeds maximum of %d bytes", engine.cfg.Storage.MaxFileSize))
			return
		}

		// Get content type from header or auto-detect
		contentType := ctx.Header("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = DetectMIME(filePath, body)
		}

		// Get uploader UID from auth context
		uploaderUID := ctx.GetString("auth_uid")

		// Parse custom metadata from X-Custom-Metadata header (JSON)
		var customMeta map[string]string
		if metaHeader := ctx.Header("X-Custom-Metadata"); metaHeader != "" {
			json.Unmarshal([]byte(metaHeader), &customMeta)
		}

		info, err := engine.Upload(filePath, body, contentType, uploaderUID, customMeta)
		if err != nil {
			ctx.Error(500, err.Error())
			return
		}

		ctx.JSON(201, map[string]interface{}{
			"status":       "ok",
			"path":         info.Path,
			"hash":         info.Hash,
			"size":         info.Size,
			"content_type": info.ContentType,
			"created_at":   info.CreatedAt.Format(time.RFC3339),
		})
	}
}

// handleDownload handles file downloads with Range request support.
// Also supports signed URL access via ?token=&expires= query params.
func handleDownload(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		filePath := ctx.Param("path")
		if filePath == "" {
			ctx.Error(400, "File path is required")
			return
		}

		// Check for signed URL access
		token := ctx.QueryParam("token")
		if token != "" {
			expires := ctx.QueryParam("expires")
			method := ctx.QueryParamDefault("method", "GET")
			if err := engine.Access().VerifySignedURL(filePath, token, expires, method); err != nil {
				ctx.Error(403, err.Error())
				return
			}
		}

		// Check for Range header — use streaming if present
		rangeHeader := ctx.Header("Range")
		if rangeHeader != "" {
			info, err := engine.GetMetadata(filePath)
			if err != nil {
				ctx.Error(404, "File not found")
				return
			}

			rangeSpec, err := ParseRange(rangeHeader, info.Size)
			if err != nil {
				ctx.Error(416, err.Error())
				return
			}

			blobPath := engine.GetBlobPath(info.Hash)

			// Hijack the connection for direct streaming
			conn := ctx.Hijack()
			_, streamErr := StreamFile(blobPath, conn, rangeSpec, info.ContentType)
			if streamErr != nil {
				engine.log.Error("Stream error", logger.Fields{
					"path":  filePath,
					"error": streamErr.Error(),
				})
			}
			conn.Close()
			return
		}

		// Standard full download
		data, info, err := engine.Download(filePath)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				ctx.Error(404, "File not found")
			} else {
				ctx.Error(500, err.Error())
			}
			return
		}

		// Set response headers
		ctx.SetResponseHeader("Content-Type", info.ContentType)
		ctx.SetResponseHeader("Content-Length", fmt.Sprintf("%d", info.Size))
		ctx.SetResponseHeader("ETag", fmt.Sprintf(`"%s"`, GenerateETag(info.Hash)))
		ctx.SetResponseHeader("Accept-Ranges", "bytes")
		ctx.SetResponseHeader("Cache-Control", "public, max-age=31536000, immutable")

		// Check If-None-Match for caching
		if ifNoneMatch := ctx.Header("If-None-Match"); ifNoneMatch != "" {
			etag := fmt.Sprintf(`"%s"`, GenerateETag(info.Hash))
			if ifNoneMatch == etag {
				ctx.Status(304)
				return
			}
		}

		ctx.Response.SetStatus(200)
		ctx.Response.SetBody(data)
	}
}

// handleDelete removes a file from storage.
func handleDelete(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		filePath := ctx.Param("path")
		if filePath == "" {
			ctx.Error(400, "File path is required")
			return
		}

		err := engine.Delete(filePath)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				ctx.Error(404, "File not found")
			} else {
				ctx.Error(500, err.Error())
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":  "ok",
			"message": "File deleted",
			"path":    filePath,
		})
	}
}

// handleList lists files under a given prefix/directory.
func handleList(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		prefix := ctx.Param("path")

		files := engine.List(prefix)

		items := make([]map[string]interface{}, 0, len(files))
		for _, info := range files {
			items = append(items, map[string]interface{}{
				"path":         info.Path,
				"size":         info.Size,
				"content_type": info.ContentType,
				"created_at":   info.CreatedAt.Format(time.RFC3339),
				"updated_at":   info.UpdatedAt.Format(time.RFC3339),
			})
		}

		ctx.JSON(200, map[string]interface{}{
			"status": "ok",
			"prefix": prefix,
			"count":  len(items),
			"files":  items,
		})
	}
}

// handleGetMetadata returns metadata for a file without its content.
func handleGetMetadata(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		filePath := ctx.Param("path")
		if filePath == "" {
			ctx.Error(400, "File path is required")
			return
		}

		info, err := engine.GetMetadata(filePath)
		if err != nil {
			ctx.Error(404, "File not found")
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":          "ok",
			"path":            info.Path,
			"hash":            info.Hash,
			"size":            info.Size,
			"content_type":    info.ContentType,
			"created_at":      info.CreatedAt.Format(time.RFC3339),
			"updated_at":      info.UpdatedAt.Format(time.RFC3339),
			"uploader_uid":    info.UploaderUID,
			"custom_metadata": info.CustomMetadata,
		})
	}
}

// handleUpdateMetadata updates the custom metadata for a file.
func handleUpdateMetadata(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		filePath := ctx.Param("path")
		if filePath == "" {
			ctx.Error(400, "File path is required")
			return
		}

		var body struct {
			CustomMetadata map[string]string `json:"custom_metadata"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		info, err := engine.UpdateMetadata(filePath, body.CustomMetadata)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				ctx.Error(404, "File not found")
			} else {
				ctx.Error(500, err.Error())
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":          "ok",
			"path":            info.Path,
			"custom_metadata": info.CustomMetadata,
			"updated_at":      info.UpdatedAt.Format(time.RFC3339),
		})
	}
}

// handleSignedURL generates a time-limited signed URL for a file.
func handleSignedURL(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Path      string `json:"path"`
			ExpiresIn int    `json:"expires_in"` // seconds
			Method    string `json:"method"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if body.Path == "" {
			ctx.Error(400, "File path is required")
			return
		}

		// Verify file exists
		if _, err := engine.GetMetadata(body.Path); err != nil {
			ctx.Error(404, "File not found")
			return
		}

		expiry := time.Duration(body.ExpiresIn) * time.Second
		if expiry <= 0 {
			expiry = 1 * time.Hour
		}

		signed, err := engine.Access().GenerateSignedURL(SignedURLParams{
			Path:   body.Path,
			Expiry: expiry,
			Method: body.Method,
		})
		if err != nil {
			ctx.Error(500, err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":     "ok",
			"url":        signed.URL,
			"token":      signed.Token,
			"expires_at": signed.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// handleChunkInit initiates a chunked upload session.
func handleChunkInit(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Path        string `json:"path"`
			TotalSize   int64  `json:"total_size"`
			ContentType string `json:"content_type"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if body.Path == "" {
			ctx.Error(400, "File path is required")
			return
		}
		if body.TotalSize <= 0 {
			ctx.Error(400, "Total size must be positive")
			return
		}

		uploaderUID := ctx.GetString("auth_uid")

		upload, err := engine.Chunks().InitUpload(body.Path, body.TotalSize, body.ContentType, uploaderUID)
		if err != nil {
			ctx.Error(400, err.Error())
			return
		}

		ctx.JSON(201, map[string]interface{}{
			"status":       "ok",
			"upload_id":    upload.ID,
			"chunk_size":   upload.ChunkSize,
			"total_chunks": upload.TotalChunks,
			"expires_at":   upload.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// handleChunkAdd receives a single chunk for an upload session.
func handleChunkAdd(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		uploadID := ctx.QueryParam("upload_id")
		indexStr := ctx.QueryParam("index")

		if uploadID == "" {
			ctx.Error(400, "upload_id query parameter is required")
			return
		}
		if indexStr == "" {
			ctx.Error(400, "index query parameter is required")
			return
		}

		index := 0
		for _, c := range indexStr {
			if c < '0' || c > '9' {
				ctx.Error(400, "Invalid chunk index")
				return
			}
			index = index*10 + int(c-'0')
		}

		data := ctx.BodyBytes()
		if len(data) == 0 {
			ctx.Error(400, "Chunk data is empty")
			return
		}

		status, err := engine.Chunks().AddChunk(uploadID, index, data)
		if err != nil {
			ctx.Error(400, err.Error())
			return
		}

		ctx.JSON(200, status)
	}
}

// handleChunkComplete assembles a chunked upload and stores the file.
func handleChunkComplete(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			UploadID string `json:"upload_id"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if body.UploadID == "" {
			ctx.Error(400, "upload_id is required")
			return
		}

		data, upload, err := engine.Chunks().Assemble(body.UploadID)
		if err != nil {
			ctx.Error(400, err.Error())
			return
		}

		// Store the assembled file
		info, err := engine.Upload(upload.Path, data, upload.ContentType, upload.UploaderUID, nil)
		if err != nil {
			ctx.Error(500, err.Error())
			return
		}

		ctx.JSON(201, map[string]interface{}{
			"status":       "ok",
			"path":         info.Path,
			"hash":         info.Hash,
			"size":         info.Size,
			"content_type": info.ContentType,
			"created_at":   info.CreatedAt.Format(time.RFC3339),
		})
	}
}

// handleChunkCancel cancels an in-progress chunked upload.
func handleChunkCancel(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		uploadID := ctx.Param("uploadId")
		if uploadID == "" {
			ctx.Error(400, "Upload ID is required")
			return
		}

		if err := engine.Chunks().CancelUpload(uploadID); err != nil {
			ctx.Error(404, err.Error())
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":  "ok",
			"message": "Upload cancelled",
		})
	}
}

// handleStats returns storage engine statistics.
func handleStats(engine *Engine) server.HandlerFunc {
	return func(ctx *server.Context) {
		stats := engine.Stats()
		stats["status"] = "ok"
		ctx.JSON(200, stats)
	}
}

// readBody reads the full request body from context.
// This is a helper for multipart support in the future.
func readBody(r io.Reader, maxSize int64) ([]byte, error) {
	limited := io.LimitReader(r, maxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("body exceeds maximum size of %d bytes", maxSize)
	}
	return data, nil
}
