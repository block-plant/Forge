// Package hosting implements Forge static site hosting.
// It serves static files from deployed site bundles with an
// in-memory LRU cache, Gzip compression, SPA fallback, and
// clean URL support — all built from scratch, zero dependencies.
package hosting

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/storage"
)

// Server is the static file server that serves deployed site bundles.
type Server struct {
	cfg       *config.Config
	log       *logger.Logger
	projectsDir string

	// sites maps site ID → *Site
	sites map[string]*Site
	mu    sync.RWMutex

	// cache is the in-memory file cache
	cache *FileCache
}

// Site represents a deployed static site.
type Site struct {
	// ID is the site's unique identifier.
	ID string `json:"id"`
	// Name is a human-friendly name.
	Name string `json:"name"`
	// RootDir is the absolute path to the site's served directory.
	RootDir string `json:"root_dir"`
	// Version is the deployment version.
	Version int `json:"version"`
	// SPAMode enables single-page app fallback to index.html.
	SPAMode bool `json:"spa_mode"`
	// CustomHeaders are response headers applied to all responses.
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	// Redirects defines URL redirect rules.
	Redirects []RedirectRule `json:"redirects,omitempty"`
	// CleanURLs enables extensionless URLs (/about → /about.html).
	CleanURLs bool `json:"clean_urls"`
	// CreatedAt is when the site was first deployed.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last deployment timestamp.
	UpdatedAt time.Time `json:"updated_at"`
	// Status is "active" or "inactive".
	Status string `json:"status"`
}

// RedirectRule defines a URL redirect.
type RedirectRule struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	StatusCode  int    `json:"status_code"` // 301 or 302
}

// ServedFile contains a file's content and metadata for serving.
type ServedFile struct {
	Content     []byte
	ContentType string
	Size        int64
	ModTime     time.Time
	ETag        string
}

// NewServer creates a new hosting server.
func NewServer(cfg *config.Config, log *logger.Logger) (*Server, error) {
	projectsDir := cfg.ResolveDataPath("hosting", "projects")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		return nil, fmt.Errorf("hosting: failed to create projects directory: %w", err)
	}

	s := &Server{
		cfg:         cfg,
		log:         log,
		projectsDir: projectsDir,
		sites:       make(map[string]*Site),
		cache:       NewFileCache(cfg.Hosting.CacheSize, cfg.Hosting.CacheMaxFileSize),
	}

	// Load existing sites
	count := s.loadExisting()
	if count > 0 {
		log.Info("Hosting sites loaded", logger.Fields{"count": count})
	}

	return s, nil
}

// ServeFile resolves a request path to a file and returns its content.
// It handles SPA fallback, clean URLs, and caching.
func (s *Server) ServeFile(siteID, requestPath string) (*ServedFile, error) {
	s.mu.RLock()
	site, ok := s.sites[siteID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("hosting: site %q not found", siteID)
	}

	if site.Status != "active" {
		return nil, fmt.Errorf("hosting: site %q is inactive", siteID)
	}

	// Normalize the request path
	requestPath = normalizePath(requestPath)

	// Check redirects first
	if dest, code := matchRedirect(site.Redirects, requestPath); dest != "" {
		return &ServedFile{
			Content:     []byte(fmt.Sprintf("Redirecting to %s", dest)),
			ContentType: "text/html",
			Size:        0,
			ETag:        fmt.Sprintf("redirect:%d:%s", code, dest),
		}, nil
	}

	// Build the absolute file path
	filePath := filepath.Join(site.RootDir, requestPath)

	// Security: prevent directory traversal
	absRoot, _ := filepath.Abs(site.RootDir)
	absFile, _ := filepath.Abs(filePath)
	if !strings.HasPrefix(absFile, absRoot) {
		return nil, fmt.Errorf("hosting: path traversal attempt blocked")
	}

	// Try to serve the file (with fallback chain)
	candidates := s.buildCandidates(filePath, requestPath, site)

	for _, candidate := range candidates {
		// Check cache first
		cacheKey := siteID + ":" + candidate
		if cached := s.cache.Get(cacheKey); cached != nil {
			return cached, nil
		}

		// Read from disk
		served, err := s.readFile(candidate)
		if err == nil {
			// Cache it
			s.cache.Set(cacheKey, served)
			return served, nil
		}
	}

	return nil, fmt.Errorf("hosting: file not found: %s", requestPath)
}

// buildCandidates generates an ordered list of file paths to try.
func (s *Server) buildCandidates(filePath, requestPath string, site *Site) []string {
	var candidates []string

	// 1. Exact match
	candidates = append(candidates, filePath)

	// 2. Index file if directory
	candidates = append(candidates, filepath.Join(filePath, "index.html"))

	// 3. Clean URLs: /about → /about.html
	if site.CleanURLs && !strings.Contains(filepath.Base(filePath), ".") {
		candidates = append(candidates, filePath+".html")
	}

	// 4. SPA fallback: serve /index.html for any path
	if site.SPAMode {
		candidates = append(candidates, filepath.Join(site.RootDir, "index.html"))
	}

	return candidates
}

// readFile reads a file from disk and prepares it for serving.
func (s *Server) readFile(path string) (*ServedFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, fmt.Errorf("is a directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	contentType := storage.DetectMIME(path, data)

	return &ServedFile{
		Content:     data,
		ContentType: contentType,
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		ETag:        fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()),
	}, nil
}

// GetSite returns a site by ID.
func (s *Server) GetSite(id string) (*Site, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	site, ok := s.sites[id]
	return site, ok
}

// ListSites returns all registered sites.
func (s *Server) ListSites() []*Site {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Site, 0, len(s.sites))
	for _, site := range s.sites {
		list = append(list, site)
	}
	return list
}

// AddSite registers a new site.
func (s *Server) AddSite(site *Site) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sites[site.ID] = site
}

// RemoveSite removes a site.
func (s *Server) RemoveSite(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sites[id]; !ok {
		return false
	}
	delete(s.sites, id)
	s.cache.Invalidate(id)
	return true
}

// InvalidateCache clears the cache for a specific site.
func (s *Server) InvalidateCache(siteID string) {
	s.cache.Invalidate(siteID)
}

// SiteCount returns the number of sites.
func (s *Server) SiteCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sites)
}

// loadExisting scans the projects directory for deployed sites.
func (s *Server) loadExisting() int {
	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		siteDir := filepath.Join(s.projectsDir, entry.Name())
		indexPath := filepath.Join(siteDir, "index.html")

		// Only register if it has at least an index.html
		if _, err := os.Stat(indexPath); err != nil {
			continue
		}

		site := &Site{
			ID:      entry.Name(),
			Name:    entry.Name(),
			RootDir: siteDir,
			Version: 1,
			SPAMode: s.cfg.Hosting.SPAMode,
			Status:  "active",
		}

		s.sites[site.ID] = site
		count++
	}

	return count
}

// ── Helpers ──

// normalizePath cleans a request path.
func normalizePath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	// Remove leading slash for filepath.Join compatibility
	path = strings.TrimPrefix(path, "/")
	// Clean the path to prevent traversal
	path = filepath.Clean(path)
	return path
}

// matchRedirect checks if a path matches any redirect rules.
func matchRedirect(rules []RedirectRule, path string) (string, int) {
	for _, rule := range rules {
		if rule.Source == path {
			code := rule.StatusCode
			if code == 0 {
				code = 301
			}
			return rule.Destination, code
		}
	}
	return "", 0
}
