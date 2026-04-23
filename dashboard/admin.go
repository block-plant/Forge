package dashboard

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ayushkunwarsingh/forge/server"
)

type CreateProjectRequest struct {
	Name            string `json:"name"`
	EnableAuth      bool   `json:"enable_auth"`
	EnableDB        bool   `json:"enable_db"`
	EnableStorage   bool   `json:"enable_storage"`
	EnableFunctions bool   `json:"enable_functions"`
	EnableHosting   bool   `json:"enable_hosting"`
	EnableAnalytics bool   `json:"enable_analytics"`
	EnableRealtime  bool   `json:"enable_realtime"`
}

type ProjectInfo struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Port int    `json:"port"`
}

type serviceConfig struct {
	Enabled bool `json:"enabled"`
}

type projectConfig struct {
	Server struct {
		Port int `json:"port"`
	} `json:"server"`
	Auth      serviceConfig `json:"auth"`
	Database  serviceConfig `json:"database"`
	Storage   serviceConfig `json:"storage"`
	Functions serviceConfig `json:"functions"`
	Hosting   serviceConfig `json:"hosting"`
	Analytics serviceConfig `json:"analytics"`
	Realtime  serviceConfig `json:"realtime"`
	DataDir   string        `json:"data_dir"`
}

type projectSummary struct {
	Name      string                    `json:"name"`
	Slug      string                    `json:"slug"`
	Port      int                       `json:"port"`
	Health    string                    `json:"health"`
	Services  map[string]map[string]any `json:"services"`
	LastError string                    `json:"last_error,omitempty"`
}

type purgeRequest struct {
	Confirm string   `json:"confirm"`
	Scopes  []string `json:"scopes"`
}

var allowedPurgeScopes = map[string]bool{
	"auth":      true,
	"database":  true,
	"storage":   true,
	"hosting":   true,
	"analytics": true,
	"functions": true,
	"realtime":  true,
	"all":       true,
}

func projectDataDir(slug string, cfg projectConfig) string {
	if strings.TrimSpace(cfg.DataDir) != "" {
		return cfg.DataDir
	}
	return filepath.Join("/var/lib/forge-data", slug)
}

func wipeDirContents(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("empty directory path")
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func normalizeScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return []string{"all"}, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		s := strings.ToLower(strings.TrimSpace(scope))
		if s == "" {
			continue
		}
		if !allowedPurgeScopes[s] {
			return nil, fmt.Errorf("unsupported scope: %s", s)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []string{"all"}
	}
	return out, nil
}

func restartProjectService(slug string) {
	if strings.TrimSpace(slug) == "" {
		return
	}
	_ = exec.Command("sudo", "systemctl", "restart", "forge-"+slug).Run()
}

func loadProjectConfig(slug string) (projectConfig, error) {
	cfgPath := filepath.Join("/opt/forge/projects", slug, "forge.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return projectConfig{}, err
	}
	var cfg projectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return projectConfig{}, err
	}
	return cfg, nil
}

func purgeProjectScopes(slug string, cfg projectConfig, scopes []string) ([]string, error) {
	dataDir := projectDataDir(slug, cfg)
	expanded := scopes
	if len(scopes) == 1 && scopes[0] == "all" {
		expanded = []string{"auth", "database", "storage", "hosting", "analytics", "functions", "realtime"}
	}

	performed := make([]string, 0, len(expanded))
	for _, scope := range expanded {
		switch scope {
		case "auth":
			if err := wipeDirContents(filepath.Join(dataDir, "auth", "users")); err != nil {
				return performed, err
			}
			if err := wipeDirContents(filepath.Join(dataDir, "auth", "tokens")); err != nil {
				return performed, err
			}
		case "database":
			if err := wipeDirContents(filepath.Join(dataDir, "database")); err != nil {
				return performed, err
			}
		case "storage":
			if err := wipeDirContents(filepath.Join(dataDir, "storage", "objects")); err != nil {
				return performed, err
			}
			if err := wipeDirContents(filepath.Join(dataDir, "storage", "uploads")); err != nil {
				return performed, err
			}
		case "hosting":
			if err := wipeDirContents(filepath.Join(dataDir, "hosting", "projects")); err != nil {
				return performed, err
			}
		case "analytics":
			if err := wipeDirContents(filepath.Join(dataDir, "analytics")); err != nil {
				return performed, err
			}
		case "functions":
			if err := wipeDirContents(filepath.Join(dataDir, "functions")); err != nil {
				return performed, err
			}
		case "realtime":
			if err := wipeDirContents(filepath.Join(dataDir, "realtime")); err != nil {
				return performed, err
			}
		default:
			return performed, fmt.Errorf("unsupported scope: %s", scope)
		}
		performed = append(performed, scope)
	}
	restartProjectService(slug)
	return performed, nil
}

func handlePurgeProject(ctx *server.Context) {
	slug := strings.TrimSpace(ctx.Param("slug"))
	if slug == "" {
		ctx.Error(400, "Missing project slug")
		return
	}

	if slug == "forge" || slug == "admin" || slug == "dashboard" {
		ctx.Error(403, "Protected system slug cannot be purged")
		return
	}

	cfg, err := loadProjectConfig(slug)
	if err != nil {
		ctx.Error(404, "Project configuration not found")
		return
	}

	var req purgeRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.Error(400, "Invalid purge request")
		return
	}
	if strings.TrimSpace(req.Confirm) != "DESTROY" {
		ctx.Error(400, "Confirmation failed; send confirm=DESTROY")
		return
	}

	scopes, err := normalizeScopes(req.Scopes)
	if err != nil {
		ctx.Error(400, err.Error())
		return
	}
	performed, err := purgeProjectScopes(slug, cfg, scopes)
	if err != nil {
		ctx.Error(500, fmt.Sprintf("Purge failed: %v", err))
		return
	}
	ctx.JSON(200, map[string]any{
		"status":           "purged",
		"slug":             slug,
		"performed_scopes": performed,
	})
}

func currentProjectSlugFromCWD() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(wd)
	needle := filepath.Clean("/opt/forge/projects")
	if !strings.HasPrefix(clean, needle+string(os.PathSeparator)) {
		return "", fmt.Errorf("instance is not running from a project directory")
	}
	rest := strings.TrimPrefix(clean, needle+string(os.PathSeparator))
	parts := strings.Split(rest, string(os.PathSeparator))
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("unable to infer project slug from working directory")
	}
	return parts[0], nil
}

func handlePurgeCurrentProject(ctx *server.Context) {
	slug, err := currentProjectSlugFromCWD()
	if err != nil {
		ctx.Error(400, err.Error())
		return
	}
	cfg, err := loadProjectConfig(slug)
	if err != nil {
		ctx.Error(404, "Project configuration not found")
		return
	}
	var req purgeRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.Error(400, "Invalid purge request")
		return
	}
	if strings.TrimSpace(req.Confirm) != "DESTROY" {
		ctx.Error(400, "Confirmation failed; send confirm=DESTROY")
		return
	}
	scopes, err := normalizeScopes(req.Scopes)
	if err != nil {
		ctx.Error(400, err.Error())
		return
	}
	performed, err := purgeProjectScopes(slug, cfg, scopes)
	if err != nil {
		ctx.Error(500, fmt.Sprintf("Purge failed: %v", err))
		return
	}
	ctx.JSON(200, map[string]any{
		"status":           "purged",
		"slug":             slug,
		"performed_scopes": performed,
	})
}

func fetchJSON(url string, out any) error {
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("http %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func countAuthUsers(dataDir string) (total int, signupsToday int, activeSessions int) {
	usersDir := filepath.Join(dataDir, "auth", "users")
	entries, err := os.ReadDir(usersDir)
	if err == nil {
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			total++
			data, readErr := os.ReadFile(filepath.Join(usersDir, entry.Name()))
			if readErr != nil {
				continue
			}
			var user struct {
				CreatedAt int64 `json:"created_at"`
			}
			if json.Unmarshal(data, &user) == nil && user.CreatedAt >= startOfDay {
				signupsToday++
			}
		}
	}

	tokensDir := filepath.Join(dataDir, "auth", "tokens")
	tokens, err := os.ReadDir(tokensDir)
	if err == nil {
		nowUnix := time.Now().Unix()
		for _, entry := range tokens {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(tokensDir, entry.Name()))
			if readErr != nil {
				continue
			}
			var token struct {
				ExpiresAt int64 `json:"expires_at"`
			}
			if json.Unmarshal(data, &token) == nil && token.ExpiresAt > nowUnix {
				activeSessions++
			}
		}
	}
	return
}

func dirFileStats(root string) (count int, size int64) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		count++
		size += info.Size()
		return nil
	})
	return
}

func summarizeProject(name, slug string, cfg projectConfig) projectSummary {
	sum := projectSummary{
		Name:     name,
		Slug:     slug,
		Port:     cfg.Server.Port,
		Health:   "offline",
		Services: map[string]map[string]any{},
	}

	sum.Services["auth"] = map[string]any{"enabled": cfg.Auth.Enabled, "users": 0, "signups_today": 0, "active_sessions": 0}
	sum.Services["database"] = map[string]any{"enabled": cfg.Database.Enabled, "collections": 0, "documents": 0}
	sum.Services["storage"] = map[string]any{"enabled": cfg.Storage.Enabled, "files": 0, "bytes": int64(0)}
	sum.Services["hosting"] = map[string]any{"enabled": cfg.Hosting.Enabled, "sites": 0}
	sum.Services["analytics"] = map[string]any{"enabled": cfg.Analytics.Enabled, "buffer_used": 0, "buffer_capacity": 0}
	sum.Services["functions"] = map[string]any{"enabled": cfg.Functions.Enabled}
	sum.Services["realtime"] = map[string]any{"enabled": cfg.Realtime.Enabled}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
	var healthResp struct {
		Status   string            `json:"status"`
		Services map[string]string `json:"services"`
	}
	if err := fetchJSON(baseURL+"/health", &healthResp); err == nil {
		if strings.EqualFold(healthResp.Status, "ok") {
			sum.Health = "online"
		}
	}

	if cfg.Database.Enabled {
		var dbResp struct {
			Collections []struct {
				Count int `json:"count"`
			} `json:"collections"`
		}
		if err := fetchJSON(baseURL+"/db/collections", &dbResp); err == nil {
			totalDocs := 0
			for _, c := range dbResp.Collections {
				totalDocs += c.Count
			}
			sum.Services["database"]["collections"] = len(dbResp.Collections)
			sum.Services["database"]["documents"] = totalDocs
		}
	}

	if cfg.Storage.Enabled {
		var storageResp struct {
			TotalFiles int   `json:"total_files"`
			TotalSize  int64 `json:"total_size"`
		}
		if err := fetchJSON(baseURL+"/storage/stats", &storageResp); err == nil {
			sum.Services["storage"]["files"] = storageResp.TotalFiles
			sum.Services["storage"]["bytes"] = storageResp.TotalSize
		}
	}

	if cfg.Hosting.Enabled {
		var hostingResp struct {
			TotalSites int `json:"total_sites"`
		}
		if err := fetchJSON(baseURL+"/hosting/stats", &hostingResp); err == nil {
			sum.Services["hosting"]["sites"] = hostingResp.TotalSites
		}
	}

	if cfg.Analytics.Enabled {
		var analyticsResp struct {
			Buffer struct {
				Used     int `json:"used"`
				Capacity int `json:"capacity"`
			} `json:"buffer"`
		}
		if err := fetchJSON(baseURL+"/analytics/stats", &analyticsResp); err == nil {
			sum.Services["analytics"]["buffer_used"] = analyticsResp.Buffer.Used
			sum.Services["analytics"]["buffer_capacity"] = analyticsResp.Buffer.Capacity
		}
	}

	dataDir := projectDataDir(slug, cfg)
	if cfg.Auth.Enabled {
		users, today, sessions := countAuthUsers(dataDir)
		sum.Services["auth"]["users"] = users
		sum.Services["auth"]["signups_today"] = today
		sum.Services["auth"]["active_sessions"] = sessions
	}
	if cfg.Storage.Enabled {
		count, size := dirFileStats(filepath.Join(dataDir, "storage", "objects"))
		if files, ok := sum.Services["storage"]["files"].(int); ok && files == 0 {
			sum.Services["storage"]["files"] = count
		}
		if bytes, ok := sum.Services["storage"]["bytes"].(int64); ok && bytes == 0 {
			sum.Services["storage"]["bytes"] = size
		}
	}
	return sum
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

// generateSlug turns "Chat App" into "chat-app"
func generateSlug(name string) string {
	lower := strings.ToLower(name)
	slug := nonAlphanumericRegex.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}

// findFreePort checks ports between 8081 and 8100
func findFreePort() (int, error) {
	for port := 8081; port <= 8100; port++ {
		addr := fmt.Sprintf("0.0.0.0:%d", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports between 8081 and 8100")
}

// resolveSlugByPort scans /opt/forge/projects/*/forge.json to find
// which project slug is running on the given port.
func resolveSlugByPort(port int) (string, error) {
	projectsDir := "/opt/forge/projects"
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", fmt.Errorf("cannot read projects directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		configPath := filepath.Join(projectsDir, entry.Name(), "forge.json")
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var cfg struct {
			Server struct {
				Port int `json:"port"`
			} `json:"server"`
		}
		if json.Unmarshal(data, &cfg) == nil && cfg.Server.Port == port {
			return entry.Name(), nil
		}
	}
	return "", fmt.Errorf("no project found on port %d", port)
}

func handleCreateProject(ctx *server.Context) {
	var req CreateProjectRequest
	if err := ctx.BindJSON(&req); err != nil || req.Name == "" {
		ctx.Error(400, "Invalid or missing project name")
		return
	}

	slug := generateSlug(req.Name)
	if slug == "" {
		ctx.Error(400, "Project name must contain alphanumeric characters")
		return
	}

	port, err := findFreePort()
	if err != nil {
		ctx.Error(500, "Could not find an available port")
		return
	}

	appDir := filepath.Join("/opt/forge/projects", slug)
	dataDir := filepath.Join("/var/lib/forge-data", slug)
	serviceFile := fmt.Sprintf("/etc/systemd/system/forge-%s.service", slug)

	// Create directories (using sudo to be safe if /var/lib needs it)
	exec.Command("sudo", "mkdir", "-p", appDir, dataDir).Run()
	exec.Command("sudo", "chown", "-R", "ubuntu:ubuntu", appDir, dataDir).Run()

	// Write forge.json
	configJSON := fmt.Sprintf(`{
  "server": {
    "host": "0.0.0.0",
    "port": %d,
    "enable_cors": true,
    "cors_origins": ["*"]
  },
  "auth": { "enabled": %t },
  "database": { "enabled": %t },
  "storage": { "enabled": %t },
  "functions": { "enabled": %t },
  "hosting": { "enabled": %t },
  "analytics": { "enabled": %t },
  "realtime": { "enabled": %t },
  "data_dir": "%s"
}`, port, req.EnableAuth, req.EnableDB, req.EnableStorage, req.EnableFunctions, req.EnableHosting, req.EnableAnalytics, req.EnableRealtime, dataDir)

	configPath := filepath.Join(appDir, "forge.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		ctx.Error(500, fmt.Sprintf("Failed to write config: %v", err))
		return
	}

	// Copy the current binary into the project directory
	currentBinary := "/opt/forge/forge"
	targetBinary := filepath.Join(appDir, "forge")
	if srcData, err := os.ReadFile(currentBinary); err == nil {
		os.WriteFile(targetBinary, srcData, 0755)
	}

	// Write systemd service
	serviceContent := fmt.Sprintf(`[Unit]
Description=Forge BaaS - %s
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s/forge --config %s
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`, req.Name, appDir, appDir, configPath)

	tmpService := fmt.Sprintf("/tmp/forge-%s.service", slug)
	if err := os.WriteFile(tmpService, []byte(serviceContent), 0644); err != nil {
		ctx.Error(500, fmt.Sprintf("Failed to write temp systemd service: %v", err))
		return
	}
	// Move temp file to systemd dir using sudo
	if out, err := exec.Command("sudo", "mv", tmpService, serviceFile).CombinedOutput(); err != nil {
		ctx.Error(500, fmt.Sprintf("Failed to install systemd service: %v, out: %s", err, string(out)))
		return
	}

	// Reload systemd and start the service
	exec.Command("sudo", "systemctl", "daemon-reload").Run()
	exec.Command("sudo", "systemctl", "enable", fmt.Sprintf("forge-%s", slug)).Run()
	exec.Command("sudo", "systemctl", "restart", fmt.Sprintf("forge-%s", slug)).Run()

	// Update iptables (do best effort)
	exec.Command("sudo", "iptables", "-I", "INPUT", "1", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT").Run()
	exec.Command("sudo", "netfilter-persistent", "save").Run()

	ctx.JSON(200, ProjectInfo{
		Name: req.Name,
		Slug: slug,
		Port: port,
	})
}

func handleDeleteProject(ctx *server.Context) {
	slugOrPort := ctx.Param("slug")
	if slugOrPort == "" {
		ctx.Error(400, "Missing project identifier")
		return
	}

	// If the parameter looks like a port number, resolve it to a slug
	slug := slugOrPort
	if portNum, err := strconv.Atoi(slugOrPort); err == nil && portNum >= 8081 && portNum <= 8100 {
		resolved, err := resolveSlugByPort(portNum)
		if err != nil {
			ctx.Error(404, fmt.Sprintf("No project found on port %d", portNum))
			return
		}
		slug = resolved
	}

	// 🛡️ CRITICAL SAFETY: Never allow deleting the main forge service or directory
	if slug == "forge" || slug == "admin" || slug == "dashboard" {
		ctx.Error(403, "Protected system slug cannot be deleted")
		return
	}

	appDir := filepath.Join("/opt/forge/projects", slug)
	serviceFile := fmt.Sprintf("/etc/systemd/system/forge-%s.service", slug)

	// Check if the project actually exists
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		ctx.Error(404, fmt.Sprintf("Project '%s' not found", slug))
		return
	}

	// Recover port from forge.json if possible to clear iptables
	var port int
	configPath := filepath.Join(appDir, "forge.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg struct {
			Server struct {
				Port int `json:"port"`
			} `json:"server"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			port = cfg.Server.Port
		}
	}

	// Stop and disable service
	exec.Command("sudo", "systemctl", "stop", fmt.Sprintf("forge-%s", slug)).Run()
	exec.Command("sudo", "systemctl", "disable", fmt.Sprintf("forge-%s", slug)).Run()

	// Remove files
	exec.Command("sudo", "rm", "-rf", appDir).Run()
	exec.Command("sudo", "rm", "-f", serviceFile).Run()

	// Reload systemd
	exec.Command("sudo", "systemctl", "daemon-reload").Run()

	// Best-effort iptables cleanup
	if port > 0 {
		exec.Command("sudo", "iptables", "-D", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT").Run()
		exec.Command("sudo", "netfilter-persistent", "save").Run()
	}

	ctx.JSON(200, map[string]string{
		"status": "deleted",
		"slug":   slug,
	})
}

// handleGetProjects handles GET /admin/projects
func handleGetProjects(ctx *server.Context) {
	projectsDir := "/opt/forge/projects"

	// Ensure the directory exists
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		ctx.JSON(200, []ProjectInfo{})
		return
	}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		// Log error but return empty list instead of 500
		ctx.JSON(200, []ProjectInfo{})
		return
	}

	var projects []ProjectInfo
	for _, entry := range entries {
		// Skip non-directories or hidden files
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		configPath := filepath.Join(projectsDir, entry.Name(), "forge.json")

		// If file is missing (being deleted), just skip this project
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var cfg struct {
			Server struct {
				Port int `json:"port"`
			} `json:"server"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		projects = append(projects, ProjectInfo{
			Name: entry.Name(),
			Slug: entry.Name(),
			Port: cfg.Server.Port,
		})
	}

	ctx.JSON(200, projects)
}

// handleGetProjectSummaries handles GET /admin/projects/summary
func handleGetProjectSummaries(ctx *server.Context) {
	projectsDir := "/opt/forge/projects"
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		ctx.JSON(200, map[string]any{"projects": []projectSummary{}, "totals": map[string]any{"projects": 0, "online": 0, "offline": 0}})
		return
	}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		ctx.Error(500, "Failed to read projects")
		return
	}

	summaries := make([]projectSummary, 0)
	online := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		slug := entry.Name()
		configPath := filepath.Join(projectsDir, slug, "forge.json")
		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			continue
		}
		var cfg projectConfig
		if json.Unmarshal(data, &cfg) != nil || cfg.Server.Port == 0 {
			continue
		}
		sum := summarizeProject(slug, slug, cfg)
		if sum.Health == "online" {
			online++
		}
		summaries = append(summaries, sum)
	}

	ctx.JSON(200, map[string]any{
		"projects": summaries,
		"totals": map[string]any{
			"projects": len(summaries),
			"online":   online,
			"offline":  len(summaries) - online,
		},
	})
}

// handleGetProjectConfig handles GET /admin/projects/:slug/config
func handleGetProjectConfig(ctx *server.Context) {
	slug := ctx.Param("slug")
	configPath := filepath.Join("/opt/forge/projects", slug, "forge.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		ctx.Error(404, "Project configuration not found")
		return
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		ctx.Error(500, "Failed to parse configuration")
		return
	}

	ctx.JSON(200, cfg)
}

// handleUpdateProjectConfig handles PUT /admin/projects/:slug/config
func handleUpdateProjectConfig(ctx *server.Context) {
	slug := ctx.Param("slug")
	appDir := filepath.Join("/opt/forge/projects", slug)
	configPath := filepath.Join(appDir, "forge.json")

	var newConfig map[string]interface{}
	if err := ctx.BindJSON(&newConfig); err != nil {
		ctx.Error(400, "Invalid configuration JSON")
		return
	}

	data, err := json.MarshalIndent(newConfig, "", "  ")
	if err != nil {
		ctx.Error(500, "Failed to encode configuration")
		return
	}

	// Save the config
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		ctx.Error(500, "Failed to save configuration")
		return
	}

	// Restart the service to apply changes
	serviceName := "forge-" + slug
	exec.Command("sudo", "systemctl", "restart", serviceName).Run()

	ctx.JSON(200, map[string]interface{}{
		"message": "Configuration updated and service restarted",
	})
}
