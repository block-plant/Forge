package hosting

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Deployer handles site deployment: accepts archives, extracts them,
// and registers sites with the hosting server.
type Deployer struct {
	cfg    *config.Config
	log    *logger.Logger
	server *Server
}

// DeployRequest contains deployment parameters.
type DeployRequest struct {
	// SiteID is the site identifier (used as URL path prefix).
	SiteID string `json:"site_id"`
	// SiteName is a human-friendly name.
	SiteName string `json:"site_name,omitempty"`
	// SPAMode enables single-page app fallback.
	SPAMode *bool `json:"spa_mode,omitempty"`
	// CleanURLs enables extensionless URL resolution.
	CleanURLs bool `json:"clean_urls,omitempty"`
	// CustomHeaders are response headers for all files.
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	// Redirects defines URL redirect rules.
	Redirects []RedirectRule `json:"redirects,omitempty"`
}

// DeployResult is the outcome of a deployment.
type DeployResult struct {
	SiteID    string `json:"site_id"`
	Version   int    `json:"version"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
	URL       string `json:"url"`
}

// NewDeployer creates a new hosting deployer.
func NewDeployer(cfg *config.Config, log *logger.Logger, server *Server) *Deployer {
	return &Deployer{
		cfg:    cfg,
		log:    log,
		server: server,
	}
}

// DeployArchive deploys a site from a tar.gz archive.
func (d *Deployer) DeployArchive(req DeployRequest, archiveData []byte) (*DeployResult, error) {
	if req.SiteID == "" {
		return nil, fmt.Errorf("hosting: site_id is required")
	}

	// Sanitize site ID
	req.SiteID = sanitizeSiteID(req.SiteID)

	// Create site directory
	siteDir := d.cfg.ResolveDataPath("hosting", "projects", req.SiteID)
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		return nil, fmt.Errorf("hosting: failed to create site directory: %w", err)
	}

	// Extract archive
	fileCount, totalSize, err := d.extractTarGz(archiveData, siteDir)
	if err != nil {
		return nil, fmt.Errorf("hosting: extraction failed: %w", err)
	}

	// Determine version
	version := 1
	if existing, ok := d.server.GetSite(req.SiteID); ok {
		version = existing.Version + 1
	}

	// Default SPA mode from config
	spaMode := d.cfg.Hosting.SPAMode
	if req.SPAMode != nil {
		spaMode = *req.SPAMode
	}

	now := time.Now().UTC()
	site := &Site{
		ID:            req.SiteID,
		Name:          req.SiteName,
		RootDir:       siteDir,
		Version:       version,
		SPAMode:       spaMode,
		CleanURLs:     req.CleanURLs,
		CustomHeaders: req.CustomHeaders,
		Redirects:     req.Redirects,
		CreatedAt:     now,
		UpdatedAt:     now,
		Status:        "active",
	}

	if site.Name == "" {
		site.Name = req.SiteID
	}

	// Preserve original creation time
	if existing, ok := d.server.GetSite(req.SiteID); ok {
		site.CreatedAt = existing.CreatedAt
	}

	// Save site metadata
	d.saveMetadata(site)

	// Register with server
	d.server.AddSite(site)

	// Invalidate cache for this site
	d.server.InvalidateCache(req.SiteID)

	d.log.Info("Site deployed", logger.Fields{
		"site_id":    req.SiteID,
		"version":    version,
		"files":      fileCount,
		"total_size": totalSize,
	})

	return &DeployResult{
		SiteID:    req.SiteID,
		Version:   version,
		FileCount: fileCount,
		TotalSize: totalSize,
		URL:       fmt.Sprintf("/sites/%s/", req.SiteID),
	}, nil
}

// DeployFiles deploys a site from individual files (map of path → content).
func (d *Deployer) DeployFiles(req DeployRequest, files map[string][]byte) (*DeployResult, error) {
	if req.SiteID == "" {
		return nil, fmt.Errorf("hosting: site_id is required")
	}

	req.SiteID = sanitizeSiteID(req.SiteID)

	siteDir := d.cfg.ResolveDataPath("hosting", "projects", req.SiteID)
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		return nil, fmt.Errorf("hosting: failed to create site directory: %w", err)
	}

	var totalSize int64
	fileCount := 0

	for path, content := range files {
		// Security: prevent directory traversal
		cleanPath := filepath.Clean(path)
		if strings.HasPrefix(cleanPath, "..") {
			continue
		}

		fullPath := filepath.Join(siteDir, cleanPath)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			continue
		}

		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			continue
		}

		totalSize += int64(len(content))
		fileCount++
	}

	version := 1
	if existing, ok := d.server.GetSite(req.SiteID); ok {
		version = existing.Version + 1
	}

	spaMode := d.cfg.Hosting.SPAMode
	if req.SPAMode != nil {
		spaMode = *req.SPAMode
	}

	now := time.Now().UTC()
	site := &Site{
		ID:            req.SiteID,
		Name:          req.SiteName,
		RootDir:       siteDir,
		Version:       version,
		SPAMode:       spaMode,
		CleanURLs:     req.CleanURLs,
		CustomHeaders: req.CustomHeaders,
		Redirects:     req.Redirects,
		CreatedAt:     now,
		UpdatedAt:     now,
		Status:        "active",
	}

	if site.Name == "" {
		site.Name = req.SiteID
	}

	if existing, ok := d.server.GetSite(req.SiteID); ok {
		site.CreatedAt = existing.CreatedAt
	}

	d.saveMetadata(site)
	d.server.AddSite(site)
	d.server.InvalidateCache(req.SiteID)

	return &DeployResult{
		SiteID:    req.SiteID,
		Version:   version,
		FileCount: fileCount,
		TotalSize: totalSize,
		URL:       fmt.Sprintf("/sites/%s/", req.SiteID),
	}, nil
}

// Delete removes a deployed site.
func (d *Deployer) Delete(siteID string) error {
	site, ok := d.server.GetSite(siteID)
	if !ok {
		return fmt.Errorf("hosting: site %q not found", siteID)
	}

	// Remove from disk
	if err := os.RemoveAll(site.RootDir); err != nil {
		d.log.Warn("Failed to remove site directory", logger.Fields{
			"site_id": siteID,
			"error":   err.Error(),
		})
	}

	d.server.RemoveSite(siteID)

	d.log.Info("Site deleted", logger.Fields{"site_id": siteID})
	return nil
}

// extractTarGz extracts a tar.gz archive to the target directory.
func (d *Deployer) extractTarGz(data []byte, targetDir string) (int, int64, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		// Try as plain tar
		return d.extractTar(data, targetDir)
	}
	defer gzReader.Close()

	tarData, err := io.ReadAll(gzReader)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decompress gzip: %w", err)
	}

	return d.extractTar(tarData, targetDir)
}

// extractTar extracts a tar archive to the target directory.
func (d *Deployer) extractTar(data []byte, targetDir string) (int, int64, error) {
	tarReader := tar.NewReader(bytes.NewReader(data))

	fileCount := 0
	var totalSize int64

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fileCount, totalSize, fmt.Errorf("tar read error: %w", err)
		}

		// Security: prevent directory traversal
		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(cleanName, "/") {
			continue
		}

		targetPath := filepath.Join(targetDir, cleanName)
		absTarget, _ := filepath.Abs(targetPath)
		absRoot, _ := filepath.Abs(targetDir)
		if !strings.HasPrefix(absTarget, absRoot) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(targetPath, 0755)

		case tar.TypeReg:
			dir := filepath.Dir(targetPath)
			os.MkdirAll(dir, 0755)

			content, err := io.ReadAll(tarReader)
			if err != nil {
				continue
			}

			if err := os.WriteFile(targetPath, content, 0644); err != nil {
				continue
			}

			fileCount++
			totalSize += int64(len(content))
		}
	}

	return fileCount, totalSize, nil
}

// saveMetadata writes site metadata to disk.
func (d *Deployer) saveMetadata(site *Site) {
	metaPath := filepath.Join(site.RootDir, ".forge-site.json")
	data, err := json.MarshalIndent(site, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(metaPath, data, 0644)
}

// sanitizeSiteID removes dangerous characters from a site ID.
func sanitizeSiteID(id string) string {
	var sb strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	result := sb.String()
	if result == "" {
		result = "default"
	}
	return result
}
