package dashboard

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
