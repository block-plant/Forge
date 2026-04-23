// Package config provides configuration management for the Forge platform.
// It supports loading from a JSON config file and environment variable overrides.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the top-level configuration for the Forge platform.
type Config struct {
	// Server configuration
	Server ServerConfig `json:"server"`
	// Auth service configuration
	Auth AuthConfig `json:"auth"`
	// Database configuration
	Database DatabaseConfig `json:"database"`
	// Storage configuration
	Storage StorageConfig `json:"storage"`
	// Functions configuration
	Functions FunctionsConfig `json:"functions"`
	// Hosting configuration
	Hosting HostingConfig `json:"hosting"`
	// Analytics configuration
	Analytics AnalyticsConfig `json:"analytics"`
	// Real-time configuration
	Realtime RealtimeConfig `json:"realtime"`
	// Email/SMTP configuration
	Email EmailConfig `json:"email"`
	// Logging configuration
	Log LogConfig `json:"log"`
	// Data directory (root for all persistent data)
	DataDir string `json:"data_dir"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Host to bind the server to (default: "0.0.0.0")
	Host string `json:"host"`
	// Port to listen on (default: 8080)
	Port int `json:"port"`
	// ReadTimeout is the maximum duration for reading the entire request (default: "30s")
	ReadTimeout string `json:"read_timeout"`
	// WriteTimeout is the maximum duration before timing out writes of the response (default: "30s")
	WriteTimeout string `json:"write_timeout"`
	// MaxHeaderSize is the maximum size of request headers in bytes (default: 8192)
	MaxHeaderSize int `json:"max_header_size"`
	// MaxBodySize is the maximum size of request body in bytes (default: 10485760 = 10MB)
	MaxBodySize int64 `json:"max_body_size"`
	// EnableCORS enables Cross-Origin Resource Sharing (default: true)
	EnableCORS bool `json:"enable_cors"`
	// CORSOrigins is a list of allowed CORS origins (default: ["*"])
	CORSOrigins []string `json:"cors_origins"`
}

// AuthConfig holds authentication service settings.
type AuthConfig struct {
	// Enabled toggles the auth service (default: true)
	Enabled bool `json:"enabled"`
	// TokenExpiry is the access token lifetime (default: "1h")
	TokenExpiry string `json:"token_expiry"`
	// RefreshExpiry is the refresh token lifetime (default: "720h" = 30 days)
	RefreshExpiry string `json:"refresh_expiry"`
	// BcryptCost is the bcrypt hashing cost (default: 12)
	BcryptCost int `json:"bcrypt_cost"`
	// KeySize is the RSA key size in bits (default: 4096)
	KeySize int `json:"key_size"`
	// OAuth providers
	GoogleClientID     string `json:"google_client_id"`
	GoogleClientSecret string `json:"google_client_secret"`
	GitHubClientID     string `json:"github_client_id"`
	GitHubClientSecret string `json:"github_client_secret"`
}

// DatabaseConfig holds database engine settings.
type DatabaseConfig struct {
	// Enabled toggles the database service (default: true)
	Enabled            bool   `json:"enabled"`
	WALDir             string `json:"wal_dir"`
	SnapshotDir        string `json:"snapshot_dir"`
	SnapshotInterval   string `json:"snapshot_interval"`
	MaxTransactionSize int    `json:"max_transaction_size"`
	MaxConnections     int    `json:"max_connections"`
	CacheSizeMB        int    `json:"cache_size_mb"`
}

// StorageConfig holds file storage settings.
type StorageConfig struct {
	// Enabled toggles the storage service (default: true)
	Enabled     bool  `json:"enabled"`
	MaxFileSize int64 `json:"max_file_size"`
	ChunkSize   int   `json:"chunk_size"`
	AllowedTypes string `json:"allowed_types"`
}

// FunctionsConfig holds serverless functions settings.
type FunctionsConfig struct {
	// Enabled toggles the functions service (default: true)
	Enabled bool `json:"enabled"`
	// Timeout is the max execution time per function in seconds (default: 60)
	Timeout int `json:"timeout"`
	// MemoryLimit is the max memory per function in MB (default: 256)
	MemoryLimit int `json:"memory_limit"`
	// MaxConcurrent is the max number of concurrent function executions (default: 10)
	MaxConcurrent int `json:"max_concurrent"`
	// Runtime is the execution runtime: "node" or "script" (default: "script")
	Runtime string `json:"runtime"`
	// Env holds environment variables for function execution
	Env map[string]string `json:"env"`
}

// HostingConfig holds static hosting settings.
type HostingConfig struct {
	// Enabled toggles the hosting service (default: true)
	Enabled bool `json:"enabled"`
	// CacheSize is the max number of files in the in-memory cache (default: 1000)
	CacheSize int `json:"cache_size"`
	// CacheMaxFileSize is the max file size to cache in bytes (default: 1MB)
	CacheMaxFileSize int64 `json:"cache_max_file_size"`
	// EnableCompression enables Gzip compression (default: true)
	EnableCompression bool `json:"enable_compression"`
	// SPAMode enables single-page app fallback to index.html (default: true)
	SPAMode bool `json:"spa_mode"`
	// Headers allows setting custom HTTP response headers
	Headers map[string]string `json:"headers"`
}

// AnalyticsConfig holds analytics engine settings.
type AnalyticsConfig struct {
	// Enabled toggles the analytics service (default: true)
	Enabled bool `json:"enabled"`
	// BufferSize is the size of the event ingestion channel (default: 10000)
	BufferSize int `json:"buffer_size"`
	// FlushInterval is how often to flush events to disk (default: "5s")
	FlushInterval string `json:"flush_interval"`
	// RetentionDays is how long to keep analytics logs (0 = forever, default: 90)
	RetentionDays int `json:"retention_days"`
}

// RealtimeConfig holds real-time WebSocket service settings.
type RealtimeConfig struct {
	// Enabled toggles the real-time service (default: true)
	Enabled bool `json:"enabled"`
	// MaxClients is the max number of concurrent WebSocket connections
	MaxClients int `json:"max_clients"`
}

// EmailConfig holds SMTP settings for OTP and notifications.
type EmailConfig struct {
	// Enabled toggles email delivery (default: false)
	Enabled bool `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Password string `json:"password"`
	From    string `json:"from"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	// Level is the minimum log level: debug, info, warn, error (default: "info")
	Level string `json:"level"`
	// Pretty enables color-coded human-readable output (default: true)
	Pretty bool `json:"pretty"`
	// Output is the log output destination: "stdout" or a file path (default: "stdout")
	Output string `json:"output"`
}

// DefaultConfig returns a Config with sensible defaults for all services.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:          "0.0.0.0",
			Port:          8080,
			ReadTimeout:   "30s",
			WriteTimeout:  "30s",
			MaxHeaderSize: 8192,
			MaxBodySize:   10 * 1024 * 1024, // 10MB
			EnableCORS:    true,
			CORSOrigins:   []string{"*"},
		},
		Auth: AuthConfig{
			Enabled:       true,
			TokenExpiry:   "1h",
			RefreshExpiry: "720h",
			BcryptCost:    12,
			KeySize:       4096,
		},
		Database: DatabaseConfig{
			Enabled:            true,
			WALDir:             "database/wal",
			SnapshotDir:        "database/snapshots",
			SnapshotInterval:   "5m",
			MaxTransactionSize: 500,
			MaxConnections:     100,
			CacheSizeMB:        128,
		},
		Storage: StorageConfig{
			Enabled:      true,
			MaxFileSize:  100 * 1024 * 1024, // 100MB
			ChunkSize:    256 * 1024,         // 256KB
			AllowedTypes: "*/*",
		},
		Functions: FunctionsConfig{
			Enabled:       true,
			Timeout:       60,
			MemoryLimit:   256,
			MaxConcurrent: 10,
			Runtime:       "script",
			Env:           make(map[string]string),
		},
		Hosting: HostingConfig{
			Enabled:           true,
			CacheSize:         1000,
			CacheMaxFileSize:  1024 * 1024, // 1MB
			EnableCompression: true,
			SPAMode:           true,
			Headers:           make(map[string]string),
		},
		Analytics: AnalyticsConfig{
			Enabled:       true,
			BufferSize:    10000,
			FlushInterval: "5s",
			RetentionDays: 90,
		},
		Realtime: RealtimeConfig{
			Enabled:    true,
			MaxClients: 1000,
		},
		Email: EmailConfig{
			Enabled: false,
			Port:    587,
		},
		Log: LogConfig{
			Level:  "info",
			Pretty: true,
			Output: "stdout",
		},
		DataDir: "forge-data",
	}
}

// Load reads configuration from a JSON file and merges with defaults.
// If the file doesn't exist, returns defaults.
// Environment variables override file values (see env.go).
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Try to load from file
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				// File doesn't exist — use defaults + env overrides
				ApplyEnvOverrides(cfg)
				return cfg, nil
			}
			return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
		}

		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
		}
	}

	// Environment overrides take precedence
	ApplyEnvOverrides(cfg)

	return cfg, nil
}

// Address returns the full TCP listen address.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// ValidateEmailConfig checks if SMTP settings are provided if email is enabled.
func (c *Config) ValidateEmailConfig() error {
	if !c.Email.Enabled {
		return nil
	}
	if c.Email.Host == "" {
		return fmt.Errorf("SMTP host is required when email is enabled")
	}
	if c.Email.User == "" {
		return fmt.Errorf("SMTP user is required when email is enabled")
	}
	if c.Email.Password == "" {
		return fmt.Errorf("SMTP password is required when email is enabled")
	}
	if c.Email.From == "" {
		return fmt.Errorf("SMTP from address is required when email is enabled")
	}
	return nil
}

// ResolveDataPath resolves a relative path against the data directory.
func (c *Config) ResolveDataPath(parts ...string) string {
	allParts := append([]string{c.DataDir}, parts...)
	return filepath.Join(allParts...)
}

// EnsureDataDirs creates the data directory structure.
func (c *Config) EnsureDataDirs() error {
	dirs := []string{
		c.DataDir,
		c.ResolveDataPath("auth"),
		c.ResolveDataPath("auth", "tokens"),
		c.ResolveDataPath("dynamicdb"),
		c.ResolveDataPath("storage", "objects"),
		c.ResolveDataPath("storage", "metadata"),
		c.ResolveDataPath("functions", "bundles"),
		c.ResolveDataPath("hosting", "projects"),
		c.ResolveDataPath("analytics", "events"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
	}

	return nil
}
