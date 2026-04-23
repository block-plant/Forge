// Forge — A Firebase replacement built from scratch in Go.
//
// Entry point: boots all services from a single binary.
// Usage: ./forge [--config path/to/forge.json]
package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ayushkunwarsingh/forge/analytics"
	"github.com/ayushkunwarsingh/forge/auth"
	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/dashboard"
	"github.com/ayushkunwarsingh/forge/database"
	"github.com/ayushkunwarsingh/forge/functions"
	"github.com/ayushkunwarsingh/forge/hosting"
	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/realtime"
	"github.com/ayushkunwarsingh/forge/rules"
	"github.com/ayushkunwarsingh/forge/server"
	"github.com/ayushkunwarsingh/forge/storage"
	"bytes"
	"io"
	"net/http"
)

const banner = `
╔═══════════════════════════════════════════════╗
║                                               ║
║   ███████╗ ██████╗ ██████╗  ██████╗ ███████╗  ║
║   ██╔════╝██╔═══██╗██╔══██╗██╔════╝ ██╔════╝  ║
║   █████╗  ██║   ██║██████╔╝██║  ███╗█████╗    ║
║   ██╔══╝  ██║   ██║██╔══██╗██║   ██║██╔══╝    ║
║   ██║     ╚██████╔╝██║  ██║╚██████╔╝███████╗  ║
║   ╚═╝      ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚══════╝  ║
║                                               ║
║   Built from scratch. Zero dependencies.      ║
║   Every byte understood.                      ║
║                                               ║
╚═══════════════════════════════════════════════╝
`

func main() {
	// Parse command-line arguments
	configPath := parseArgs()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Output:  os.Stdout,
		Level:   logger.ParseLevel(cfg.Log.Level),
		Pretty:  cfg.Log.Pretty,
		Service: "forge",
	})

	// Print banner
	fmt.Print(banner)
	log.Info("Forge is starting up", logger.Fields{
		"version":  "0.1.0",
		"go":       runtime.Version(),
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"cpus":     runtime.NumCPU(),
	})

	// Validate SMTP config
	if err := cfg.ValidateEmailConfig(); err != nil {
		log.Warn("Email service is enabled but SMTP configuration is incomplete. OTP delivery will fail.", logger.Fields{
			"error": err.Error(),
			"tip":   "Set FORGE_SMTP_HOST, FORGE_SMTP_USER, FORGE_SMTP_PASS, etc.",
		})
	}

	// Create data directories
	if err := cfg.EnsureDataDirs(); err != nil {
		log.Fatal("Failed to create data directories", logger.Fields{
			"error": err.Error(),
		})
	}

	// Create the HTTP server
	srv, err := server.New(cfg, log)
	if err != nil {
		log.Fatal("Failed to create server", logger.Fields{
			"error": err.Error(),
		})
	}

	// Set PID for logging
	server.SetPID(os.Getpid())

	// Register global middleware
	router := srv.Router()
	router.Use(server.RecoveryMiddleware(log))
	router.Use(server.RequestIDMiddleware())
	router.Use(server.LoggerMiddleware(log))

	if cfg.Server.EnableCORS {
		corsConfig := server.DefaultCORSConfig()
		if len(cfg.Server.CORSOrigins) > 0 {
			corsConfig.AllowOrigins = cfg.Server.CORSOrigins
		}
		router.Use(server.CORSMiddleware(corsConfig))
	}

	// ── Auth Service ────────────────────────────────────────────────

	var authService *auth.Service
	if cfg.Auth.Enabled {
		authService, err = auth.NewService(cfg, log)
		if err != nil {
			log.Fatal("Failed to initialize auth service", logger.Fields{
				"error": err.Error(),
			})
		}

		// Add JWT verification middleware (runs on all routes)
		router.Use(auth.Middleware(authService.JWTManager()))

		// Register auth routes
		auth.RegisterRoutes(router, authService)

		// Register OAuth routes if configured
		baseURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
		auth.RegisterOAuthRoutes(router, authService, baseURL)

		log.Info("Auth service registered", logger.Fields{
			"endpoints": "signup, signin, refresh, me, jwks, admin",
		})
	}

	// ── Database Service ────────────────────────────────────────────

	var dbEngine *database.Engine
	if cfg.Database.Enabled {
		dbEngine, err = database.NewEngine(cfg, log)
		if err != nil {
			log.Fatal("Failed to initialize database engine", logger.Fields{
				"error": err.Error(),
			})
		}

		// Register database routes
		database.RegisterRoutes(router, dbEngine)

		log.Info("Database service registered", logger.Fields{
			"endpoints": "CRUD, query, batch, transaction, indexes, snapshot",
		})
	}

	// ── Real-Time Service ──────────────────────────────────────────

	var hub *realtime.Hub
	if cfg.Realtime.Enabled {
		hub = realtime.NewHub(log)
		go hub.Run()

		// Register real-time routes
		realtime.RegisterRoutes(router, hub)

		// Connect document change streams if database is enabled
		if dbEngine != nil {
			realtime.NewStreams(hub, dbEngine, log)
		}

		log.Info("Real-time service registered", logger.Fields{
			"endpoints": "ws, stats, channels, publish",
		})
	}

	// ── Storage Service ───────────────────────────────────────────

	var storageEngine *storage.Engine
	if cfg.Storage.Enabled {
		storageEngine, err = storage.NewEngine(cfg, log)
		if err != nil {
			log.Fatal("Failed to initialize storage engine", logger.Fields{
				"error": err.Error(),
			})
		}

		// Register storage routes
		storage.RegisterRoutes(router, storageEngine)

		log.Info("Storage service registered", logger.Fields{
			"endpoints": "upload, download, delete, list, metadata, signed-url, chunks",
		})
	}

	// ── Rules Engine ──────────────────────────────────────────────

	// Load security rules if a rules file exists
	rulesFile := "forge.rules"
	if rulesData, readErr := os.ReadFile(rulesFile); readErr == nil {
		ruleSet, parseErrs := rules.ParseRules(string(rulesData))
		if len(parseErrs) > 0 {
			for _, e := range parseErrs {
				log.Warn("Rules parse error", logger.Fields{"error": e.Error()})
			}
		} else {
			// Validate rules
			valErrors := rules.Validate(ruleSet)
			for _, ve := range valErrors {
				if ve.Severity == "error" {
					log.Error("Rules validation error", logger.Fields{"error": ve.Error()})
				} else {
					log.Warn("Rules validation warning", logger.Fields{"warning": ve.Error()})
				}
			}

			log.Info("Security rules loaded", logger.Fields{
				"file":     rulesFile,
				"services": len(ruleSet.Services),
			})
		}
	} else {
		log.Info("No rules file found, running without security rules", logger.Fields{
			"expected": rulesFile,
		})
	}

	// ── Functions Service ─────────────────────────────────────────

	var funcService *functions.Service
	if cfg.Functions.Enabled {
		funcService, err = functions.NewService(cfg, log)
		if err != nil {
			log.Fatal("Failed to initialize functions service", logger.Fields{
				"error": err.Error(),
			})
		}

		functions.RegisterRoutes(router, funcService)
		funcService.Start()

		log.Info("Functions service registered", logger.Fields{
			"endpoints": "deploy, list, get, delete, invoke, schedules",
		})
	}

	// ── Hosting Service ──────────────────────────────────────────

	var hostingServer *hosting.Server
	if cfg.Hosting.Enabled {
		hostingServer, err = hosting.NewServer(cfg, log)
		if err != nil {
			log.Fatal("Failed to initialize hosting service", logger.Fields{
				"error": err.Error(),
			})
		}

		hostingDeployer := hosting.NewDeployer(cfg, log, hostingServer)
		hosting.RegisterRoutes(router, hostingServer, hostingDeployer)

		log.Info("Hosting service registered", logger.Fields{
			"endpoints": "deploy, sites, serve, cache",
		})
	}

	// ── Analytics Service ────────────────────────────────────────

	var analyticsEngine *analytics.Engine
	if cfg.Analytics.Enabled {
		analyticsEngine, err = analytics.NewEngine(cfg, log)
		if err != nil {
			log.Fatal("Failed to initialize analytics service", logger.Fields{
				"error": err.Error(),
			})
		}

		analytics.RegisterRoutes(router, analyticsEngine)

		log.Info("Analytics service registered", logger.Fields{
			"endpoints": "track, batch, stats",
		})
	}

	// ── Admin Dashboard ──────────────────────────────────────────

	if err := dashboard.RegisterRoutes(router); err != nil {
		log.Fatal("Failed to initialize dashboard", logger.Fields{
			"error": err.Error(),
		})
	}

	log.Info("Admin dashboard registered", logger.Fields{
		"path": "/dashboard/",
	})

	// ── Heimdall API Proxy ──────────────────────────────────────────
	// Tunnel /api/* requests to the Python worker on port 8000
	proxyHandler := func(ctx *server.Context) {
		path := ctx.Param("path")
		if path == "" {
			path = "/"
		} else if path[0] != '/' {
			path = "/" + path
		}
		targetURL := fmt.Sprintf("http://127.0.0.1:8000%s", path)
		if ctx.Request.RawQuery != "" {
			targetURL += "?" + ctx.Request.RawQuery
		}

		req, err := http.NewRequest(ctx.Request.Method, targetURL, bytes.NewReader(ctx.Request.Body))
		if err != nil {
			ctx.Error(500, "Proxy creation error: "+err.Error())
			return
		}

		// Copy request headers
		for k, v := range ctx.Request.Headers {
			req.Header.Set(k, v)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			ctx.Error(502, "Proxy connection error: "+err.Error())
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		// Copy response headers
		for k, v := range resp.Header {
			if len(v) > 0 {
				ctx.Response.SetHeader(k, v[0])
			}
		}
		ctx.Response.SetStatus(resp.StatusCode)
		ctx.Response.SetBody(body)
	}

	router.Handle("GET", "/api/*path", proxyHandler)
	router.Handle("POST", "/api/*path", proxyHandler)
	router.Handle("PUT", "/api/*path", proxyHandler)
	router.Handle("PATCH", "/api/*path", proxyHandler)
	router.Handle("DELETE", "/api/*path", proxyHandler)
	router.Handle("OPTIONS", "/api/*path", proxyHandler)

	log.Info("Heimdall API proxy registered", logger.Fields{
		"prefix": "/api/",
		"target": "http://127.0.0.1:8000",
	})

	// ── Health & Info Endpoints ──────────────────────────────────────

	router.GET("/health", func(ctx *server.Context) {
		services := map[string]string{}
		if authService != nil {
			services["auth"] = "ok"
		}
		if dbEngine != nil {
			services["database"] = "ok"
		}
		if hub != nil {
			services["realtime"] = "ok"
		}
		if storageEngine != nil {
			services["storage"] = "ok"
		}
		if funcService != nil {
			services["functions"] = "ok"
		}
		if hostingServer != nil {
			services["hosting"] = "ok"
		}
		if analyticsEngine != nil {
			services["analytics"] = "ok"
		}

		ctx.JSON(200, map[string]interface{}{
			"status":    "ok",
			"service":   "forge",
			"version":   "0.1.0",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"services":  services,
		})
	})

	router.GET("/", func(ctx *server.Context) {
		ctx.JSON(200, map[string]interface{}{
			"name":     "Forge",
			"version":  "0.1.0",
			"tagline":  "A Firebase replacement built from scratch",
			"status":   "running",
			"docs":     "/docs",
			"health":   "/health",
			"services": []string{"auth", "database", "storage", "realtime", "functions", "hosting", "analytics"},
		})
	})

	// ── Graceful Shutdown ────────────────────────────────────────────

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info("Received shutdown signal")
		
		if analyticsEngine != nil {
			analyticsEngine.Shutdown()
		}

		if err := srv.Shutdown(30 * time.Second); err != nil {
			log.Error("Shutdown error", logger.Fields{"error": err.Error()})
		}
		os.Exit(0)
	}()

	// ── Start the Server ─────────────────────────────────────────────

	log.Info("Starting TCP listener", logger.Fields{
		"address": cfg.Address(),
	})

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal("Server error", logger.Fields{
			"error": err.Error(),
		})
	}
}

// parseArgs reads command-line arguments for the config file path.
func parseArgs() string {
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 < len(args) {
				return args[i+1]
			}
			fmt.Fprintln(os.Stderr, "Error: --config requires a file path")
			os.Exit(1)
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		case "--version", "-v":
			fmt.Println("Forge v0.1.0")
			os.Exit(0)
		}
	}

	// Default: try forge.json in current directory
	return "forge.json"
}

// printUsage prints the help message.
func printUsage() {
	fmt.Println(`Forge — A Firebase replacement built from scratch

Usage:
  forge [options]

Options:
  --config, -c <path>    Path to config file (default: forge.json)
  --help, -h             Show this help message
  --version, -v          Show version

Environment Variables:
  FORGE_PORT             Server port (default: 8080)
  FORGE_HOST             Server host (default: 0.0.0.0)
  FORGE_DATA_DIR         Data directory (default: forge-data)
  FORGE_LOG_LEVEL        Log level: debug, info, warn, error (default: info)
  FORGE_LOG_PRETTY       Pretty print logs: true/false (default: true)

Examples:
  forge                          Start with defaults
  forge --config prod.json       Start with custom config
  FORGE_PORT=3000 forge          Start on port 3000`)
}
