package hosting

import (
	"compress/flate"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ayushkunwarsingh/forge/server"
)

// RegisterRoutes registers all hosting HTTP endpoints on the router.
func RegisterRoutes(router *server.Router, srv *Server, dep *Deployer) {
	g := router.Group("/hosting")

	// Deploy a site (tar.gz archive body)
	g.POST("/deploy", handleHostingDeploy(dep))

	// List all sites
	g.GET("/sites", handleListSites(srv))

	// Get site details
	g.GET("/sites/:id", handleGetSite(srv))

	// Delete a site
	g.DELETE("/sites/:id", handleDeleteSite(dep))

	// Invalidate cache
	g.POST("/cache/invalidate/:id", handleInvalidateCache(srv))

	// Cache stats
	g.GET("/cache/stats", handleCacheStats(srv))

	// Hosting stats
	g.GET("/stats", handleHostingStats(srv))

	// Static file serving: catch-all for /sites/:id/*path
	router.GET("/sites/:id/*path", handleServeFile(srv))
	router.GET("/sites/:id", handleServeFile(srv))
}

// handleHostingDeploy handles site deployment from an archive.
func handleHostingDeploy(dep *Deployer) server.HandlerFunc {
	return func(ctx *server.Context) {
		// Get site metadata from headers
		siteID := ctx.Header("X-Site-ID")
		if siteID == "" {
			siteID = ctx.QueryParam("site_id")
		}
		if siteID == "" {
			ctx.Error(400, "Site ID is required (X-Site-ID header or site_id query param)")
			return
		}

		body := ctx.BodyBytes()
		if len(body) == 0 {
			ctx.Error(400, "Request body is empty (expected tar.gz archive)")
			return
		}

		spaEnabled := true
		spaHeader := ctx.Header("X-SPA-Mode")
		if spaHeader == "false" || spaHeader == "0" {
			spaEnabled = false
		}

		req := DeployRequest{
			SiteID:    siteID,
			SiteName:  ctx.Header("X-Site-Name"),
			SPAMode:   &spaEnabled,
			CleanURLs: ctx.Header("X-Clean-URLs") == "true",
		}

		result, err := dep.DeployArchive(req, body)
		if err != nil {
			ctx.Error(500, err.Error())
			return
		}

		ctx.JSON(201, map[string]interface{}{
			"status":     "ok",
			"message":    "Site deployed successfully",
			"site_id":    result.SiteID,
			"version":    result.Version,
			"file_count": result.FileCount,
			"total_size": result.TotalSize,
			"url":        result.URL,
		})
	}
}

// handleListSites returns all deployed sites.
func handleListSites(srv *Server) server.HandlerFunc {
	return func(ctx *server.Context) {
		sites := srv.ListSites()

		items := make([]map[string]interface{}, 0, len(sites))
		for _, site := range sites {
			items = append(items, map[string]interface{}{
				"id":         site.ID,
				"name":       site.Name,
				"version":    site.Version,
				"spa_mode":   site.SPAMode,
				"status":     site.Status,
				"created_at": site.CreatedAt.Format(time.RFC3339),
				"updated_at": site.UpdatedAt.Format(time.RFC3339),
				"url":        fmt.Sprintf("/sites/%s/", site.ID),
			})
		}

		ctx.JSON(200, map[string]interface{}{
			"status": "ok",
			"count":  len(items),
			"sites":  items,
		})
	}
}

// handleGetSite returns details for a specific site.
func handleGetSite(srv *Server) server.HandlerFunc {
	return func(ctx *server.Context) {
		id := ctx.Param("id")
		site, ok := srv.GetSite(id)
		if !ok {
			ctx.Error(404, fmt.Sprintf("Site %q not found", id))
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":         "ok",
			"id":             site.ID,
			"name":           site.Name,
			"version":        site.Version,
			"spa_mode":       site.SPAMode,
			"clean_urls":     site.CleanURLs,
			"custom_headers": site.CustomHeaders,
			"redirects":      site.Redirects,
			"status_val":     site.Status,
			"created_at":     site.CreatedAt.Format(time.RFC3339),
			"updated_at":     site.UpdatedAt.Format(time.RFC3339),
		})
	}
}

// handleDeleteSite removes a deployed site.
func handleDeleteSite(dep *Deployer) server.HandlerFunc {
	return func(ctx *server.Context) {
		id := ctx.Param("id")
		if err := dep.Delete(id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				ctx.Error(404, err.Error())
			} else {
				ctx.Error(500, err.Error())
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":  "ok",
			"message": fmt.Sprintf("Site %q deleted", id),
		})
	}
}

// handleServeFile serves a static file from a deployed site.
func handleServeFile(srv *Server) server.HandlerFunc {
	return func(ctx *server.Context) {
		siteID := ctx.Param("id")
		requestPath := ctx.Param("path")
		if requestPath == "" {
			requestPath = "/"
		}

		served, err := srv.ServeFile(siteID, requestPath)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				ctx.Error(404, "File not found")
			} else {
				ctx.Error(500, err.Error())
			}
			return
		}

		// Check for redirect ETag
		if strings.HasPrefix(served.ETag, "redirect:") {
			// Extract status code and destination
			parts := strings.SplitN(served.ETag, ":", 3)
			if len(parts) == 3 {
				ctx.SetResponseHeader("Location", parts[2])
				ctx.Status(301)
				return
			}
		}

		// Apply custom headers from site config
		site, _ := srv.GetSite(siteID)
		if site != nil && site.CustomHeaders != nil {
			for key, value := range site.CustomHeaders {
				ctx.SetResponseHeader(key, value)
			}
		}

		// Set standard caching headers
		ctx.SetResponseHeader("Content-Type", served.ContentType)
		ctx.SetResponseHeader("Content-Length", fmt.Sprintf("%d", served.Size))
		ctx.SetResponseHeader("ETag", served.ETag)
		ctx.SetResponseHeader("Cache-Control", "public, max-age=3600")

		// Check If-None-Match
		if ifNoneMatch := ctx.Header("If-None-Match"); ifNoneMatch == served.ETag {
			ctx.Status(304)
			return
		}

		// Check if compression is possible
		acceptEncoding := ctx.Header("Accept-Encoding")
		if strings.Contains(acceptEncoding, "deflate") && isCompressible(served.ContentType) && served.Size > 1024 {
			compressed := compressDeflate(served.Content)
			if compressed != nil && len(compressed) < len(served.Content) {
				ctx.SetResponseHeader("Content-Encoding", "deflate")
				ctx.SetResponseHeader("Content-Length", fmt.Sprintf("%d", len(compressed)))
				ctx.Response.SetStatus(200)
				ctx.Response.SetBody(compressed)
				return
			}
		}

		ctx.Response.SetStatus(200)
		ctx.Response.SetBody(served.Content)
	}
}

// handleInvalidateCache clears the cache for a specific site.
func handleInvalidateCache(srv *Server) server.HandlerFunc {
	return func(ctx *server.Context) {
		id := ctx.Param("id")
		srv.InvalidateCache(id)

		ctx.JSON(200, map[string]interface{}{
			"status":  "ok",
			"message": fmt.Sprintf("Cache invalidated for site %q", id),
		})
	}
}

// handleCacheStats returns cache statistics.
func handleCacheStats(srv *Server) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.JSON(200, map[string]interface{}{
			"status":        "ok",
			"cached_files":  srv.cache.Size(),
			"max_entries":   srv.cache.maxEntries,
			"max_file_size": srv.cache.maxFileSize,
		})
	}
}

// handleHostingStats returns overall hosting statistics.
func handleHostingStats(srv *Server) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.JSON(200, map[string]interface{}{
			"status":       "ok",
			"total_sites":  srv.SiteCount(),
			"cached_files": srv.cache.Size(),
		})
	}
}

// isCompressible returns true if the content type benefits from compression.
func isCompressible(contentType string) bool {
	compressible := []string{
		"text/", "application/json", "application/javascript",
		"application/xml", "application/xhtml", "image/svg",
	}
	for _, prefix := range compressible {
		if strings.Contains(contentType, prefix) {
			return true
		}
	}
	return false
}

// compressDeflate compresses data using DEFLATE (stdlib compress/flate).
func compressDeflate(data []byte) []byte {
	var buf strings.Builder
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil
	}
	if _, err := w.Write(data); err != nil {
		return nil
	}
	if err := w.Close(); err != nil {
		return nil
	}
	return []byte(buf.String())
}

// compressWriter adapts a strings.Builder to an io.Writer for flate.
// (strings.Builder implements io.Writer natively in Go stdlib)
var _ io.Writer = (*strings.Builder)(nil)
