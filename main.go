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

	"github.com/ayushkunwarsingh/forge/auth"
	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/database"
	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/server"
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

	// ── Health & Info Endpoints ──────────────────────────────────────

	router.GET("/health", func(ctx *server.Context) {
		services := map[string]string{}
		if authService != nil {
			services["auth"] = "ok"
		}
		if dbEngine != nil {
			services["database"] = "ok"
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
