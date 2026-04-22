package config

import (
	"os"
	"strconv"
	"strings"
)

// ApplyEnvOverrides reads FORGE_* environment variables and applies them
// to the config, overriding any file-based values.
//
// Environment variable naming convention:
//
//	FORGE_PORT=8080           → Server.Port
//	FORGE_HOST=0.0.0.0        → Server.Host
//	FORGE_DATA_DIR=./data     → DataDir
//	FORGE_LOG_LEVEL=debug     → Log.Level
//	FORGE_LOG_PRETTY=true     → Log.Pretty
//	FORGE_AUTH_ENABLED=false   → Auth.Enabled
//	FORGE_CORS_ORIGINS=*      → Server.CORSOrigins (comma-separated)
//
// OAuth secrets should ALWAYS be set via environment variables:
//
//	FORGE_GOOGLE_CLIENT_ID
//	FORGE_GOOGLE_CLIENT_SECRET
//	FORGE_GITHUB_CLIENT_ID
//	FORGE_GITHUB_CLIENT_SECRET
func ApplyEnvOverrides(cfg *Config) {
	// Server
	if v := os.Getenv("FORGE_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("FORGE_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("FORGE_READ_TIMEOUT"); v != "" {
		cfg.Server.ReadTimeout = v
	}
	if v := os.Getenv("FORGE_WRITE_TIMEOUT"); v != "" {
		cfg.Server.WriteTimeout = v
	}
	if v := os.Getenv("FORGE_MAX_HEADER_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.Server.MaxHeaderSize = size
		}
	}
	if v := os.Getenv("FORGE_MAX_BODY_SIZE"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Server.MaxBodySize = size
		}
	}
	if v := os.Getenv("FORGE_ENABLE_CORS"); v != "" {
		cfg.Server.EnableCORS = parseBool(v)
	}
	if v := os.Getenv("FORGE_CORS_ORIGINS"); v != "" {
		cfg.Server.CORSOrigins = strings.Split(v, ",")
	}

	// Auth
	if v := os.Getenv("FORGE_AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = parseBool(v)
	}
	if v := os.Getenv("FORGE_TOKEN_EXPIRY"); v != "" {
		cfg.Auth.TokenExpiry = v
	}
	if v := os.Getenv("FORGE_REFRESH_EXPIRY"); v != "" {
		cfg.Auth.RefreshExpiry = v
	}
	if v := os.Getenv("FORGE_BCRYPT_COST"); v != "" {
		if cost, err := strconv.Atoi(v); err == nil {
			cfg.Auth.BcryptCost = cost
		}
	}
	if v := os.Getenv("FORGE_GOOGLE_CLIENT_ID"); v != "" {
		cfg.Auth.GoogleClientID = v
	}
	if v := os.Getenv("FORGE_GOOGLE_CLIENT_SECRET"); v != "" {
		cfg.Auth.GoogleClientSecret = v
	}
	if v := os.Getenv("FORGE_GITHUB_CLIENT_ID"); v != "" {
		cfg.Auth.GitHubClientID = v
	}
	if v := os.Getenv("FORGE_GITHUB_CLIENT_SECRET"); v != "" {
		cfg.Auth.GitHubClientSecret = v
	}

	// Database
	if v := os.Getenv("FORGE_DB_ENABLED"); v != "" {
		cfg.Database.Enabled = parseBool(v)
	}
	if v := os.Getenv("FORGE_DB_WAL_DIR"); v != "" {
		cfg.Database.WALDir = v
	}
	if v := os.Getenv("FORGE_DB_SNAPSHOT_DIR"); v != "" {
		cfg.Database.SnapshotDir = v
	}
	if v := os.Getenv("FORGE_DB_SNAPSHOT_INTERVAL"); v != "" {
		cfg.Database.SnapshotInterval = v
	}

	// Storage
	if v := os.Getenv("FORGE_STORAGE_ENABLED"); v != "" {
		cfg.Storage.Enabled = parseBool(v)
	}
	if v := os.Getenv("FORGE_STORAGE_MAX_FILE_SIZE"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Storage.MaxFileSize = size
		}
	}

	// Hosting
	if v := os.Getenv("FORGE_HOSTING_ENABLED"); v != "" {
		cfg.Hosting.Enabled = parseBool(v)
	}

	// Functions
	if v := os.Getenv("FORGE_FUNCTIONS_ENABLED"); v != "" {
		cfg.Functions.Enabled = parseBool(v)
	}

	// Analytics
	if v := os.Getenv("FORGE_ANALYTICS_ENABLED"); v != "" {
		cfg.Analytics.Enabled = parseBool(v)
	}

	// Real-time
	if v := os.Getenv("FORGE_REALTIME_ENABLED"); v != "" {
		cfg.Realtime.Enabled = parseBool(v)
	}

	// Email / SMTP
	if v := os.Getenv("FORGE_EMAIL_ENABLED"); v != "" {
		cfg.Email.Enabled = parseBool(v)
	}
	if v := os.Getenv("FORGE_SMTP_HOST"); v != "" {
		cfg.Email.Host = v
	}
	if v := os.Getenv("FORGE_SMTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Email.Port = port
		}
	}
	if v := os.Getenv("FORGE_SMTP_USER"); v != "" {
		cfg.Email.User = v
	}
	if v := os.Getenv("FORGE_SMTP_PASS"); v != "" {
		cfg.Email.Password = v
	}
	if v := os.Getenv("FORGE_SMTP_FROM"); v != "" {
		cfg.Email.From = v
	}

	// Log
	if v := os.Getenv("FORGE_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("FORGE_LOG_PRETTY"); v != "" {
		cfg.Log.Pretty = parseBool(v)
	}
	if v := os.Getenv("FORGE_LOG_OUTPUT"); v != "" {
		cfg.Log.Output = v
	}

	// Data dir
	if v := os.Getenv("FORGE_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
}

// parseBool parses a boolean string with common variations.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
