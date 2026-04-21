package functions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Function represents a deployed serverless function.
type Function struct {
	// Name is the unique identifier for this function.
	Name string `json:"name"`
	// Description is a human-readable description.
	Description string `json:"description,omitempty"`
	// Runtime is the execution runtime ("node", "script").
	Runtime string `json:"runtime"`
	// EntryPoint is the main file relative to the bundle directory.
	EntryPoint string `json:"entry_point"`
	// BundleDir is the absolute path to the function's deployed code.
	BundleDir string `json:"bundle_dir"`
	// Version is the deployment version number.
	Version int `json:"version"`
	// Triggers defines how this function can be invoked.
	Triggers []TriggerConfig `json:"triggers"`
	// CreatedAt is when the function was first deployed.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last deployment timestamp.
	UpdatedAt time.Time `json:"updated_at"`
	// Status is "active", "inactive", or "error".
	Status string `json:"status"`
}

// TriggerConfig defines how a function is triggered.
type TriggerConfig struct {
	// Type is the trigger type: "http", "db", "auth", "schedule".
	Type string `json:"type"`
	// Path is the HTTP route for HTTP triggers (e.g., "/api/hello").
	Path string `json:"path,omitempty"`
	// Event is the event name for DB/Auth triggers (e.g., "create", "delete").
	Event string `json:"event,omitempty"`
	// Collection is the DB collection for DB triggers.
	Collection string `json:"collection,omitempty"`
	// Schedule is the cron expression for schedule triggers.
	Schedule string `json:"schedule,omitempty"`
}

// Deployer manages function deployment, versioning, and storage.
type Deployer struct {
	cfg       *config.Config
	log       *logger.Logger
	bundleDir string

	// functions stores all deployed functions: name → *Function.
	functions map[string]*Function
	mu        sync.RWMutex
}

// NewDeployer creates a new function deployer.
func NewDeployer(cfg *config.Config, log *logger.Logger) (*Deployer, error) {
	bundleDir := cfg.ResolveDataPath("functions", "bundles")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return nil, fmt.Errorf("functions: failed to create bundle directory: %w", err)
	}

	d := &Deployer{
		cfg:       cfg,
		log:       log,
		bundleDir: bundleDir,
		functions: make(map[string]*Function),
	}

	// Load existing deployments
	count := d.loadExisting()
	if count > 0 {
		log.Info("Functions loaded from disk", logger.Fields{"count": count})
	}

	return d, nil
}

// Deploy deploys a function from provided source code.
// If the function already exists, it creates a new version.
func (d *Deployer) Deploy(name string, source []byte, entryPoint string, runtime string, triggers []TriggerConfig) (*Function, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("functions: name is required")
	}
	if len(source) == 0 {
		return nil, fmt.Errorf("functions: source code is required")
	}

	// Default entry point
	if entryPoint == "" {
		entryPoint = "index.js"
	}
	if runtime == "" {
		runtime = d.cfg.Functions.Runtime
	}

	// Determine version
	version := 1
	if existing, ok := d.functions[name]; ok {
		version = existing.Version + 1
	}

	// Create function directory
	fnDir := filepath.Join(d.bundleDir, name)
	if err := os.MkdirAll(fnDir, 0755); err != nil {
		return nil, fmt.Errorf("functions: failed to create function directory: %w", err)
	}

	// Write the source file
	srcPath := filepath.Join(fnDir, entryPoint)
	srcDir := filepath.Dir(srcPath)
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return nil, fmt.Errorf("functions: failed to create source directory: %w", err)
	}

	if err := os.WriteFile(srcPath, source, 0644); err != nil {
		return nil, fmt.Errorf("functions: failed to write source: %w", err)
	}

	now := time.Now().UTC()
	fn := &Function{
		Name:       name,
		Runtime:    runtime,
		EntryPoint: entryPoint,
		BundleDir:  fnDir,
		Version:    version,
		Triggers:   triggers,
		CreatedAt:  now,
		UpdatedAt:  now,
		Status:     "active",
	}

	// Preserve original creation time
	if existing, ok := d.functions[name]; ok {
		fn.CreatedAt = existing.CreatedAt
	}

	// Save metadata
	if err := d.saveMetadata(fn); err != nil {
		return nil, fmt.Errorf("functions: failed to save metadata: %w", err)
	}

	d.functions[name] = fn

	d.log.Info("Function deployed", logger.Fields{
		"name":    name,
		"version": version,
		"runtime": runtime,
		"entry":   entryPoint,
	})

	return fn, nil
}

// Get retrieves a deployed function by name.
func (d *Deployer) Get(name string) (*Function, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	fn, ok := d.functions[name]
	return fn, ok
}

// List returns all deployed functions.
func (d *Deployer) List() []*Function {
	d.mu.RLock()
	defer d.mu.RUnlock()

	list := make([]*Function, 0, len(d.functions))
	for _, fn := range d.functions {
		list = append(list, fn)
	}
	return list
}

// Delete removes a deployed function.
func (d *Deployer) Delete(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	fn, ok := d.functions[name]
	if !ok {
		return fmt.Errorf("functions: function %q not found", name)
	}

	// Remove from disk
	if err := os.RemoveAll(fn.BundleDir); err != nil {
		d.log.Warn("Failed to remove function directory", logger.Fields{
			"name":  name,
			"error": err.Error(),
		})
	}

	delete(d.functions, name)

	d.log.Info("Function deleted", logger.Fields{"name": name})
	return nil
}

// GetByTrigger finds functions matching a specific trigger type and details.
func (d *Deployer) GetByTrigger(triggerType, path, event, collection string) []*Function {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var matches []*Function
	for _, fn := range d.functions {
		if fn.Status != "active" {
			continue
		}
		for _, t := range fn.Triggers {
			if t.Type != triggerType {
				continue
			}
			switch triggerType {
			case "http":
				if t.Path == path {
					matches = append(matches, fn)
				}
			case "db":
				if t.Event == event && t.Collection == collection {
					matches = append(matches, fn)
				}
			case "auth":
				if t.Event == event {
					matches = append(matches, fn)
				}
			case "schedule":
				matches = append(matches, fn)
			}
		}
	}
	return matches
}

// Count returns the number of deployed functions.
func (d *Deployer) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.functions)
}

// saveMetadata writes function metadata to disk.
func (d *Deployer) saveMetadata(fn *Function) error {
	metaPath := filepath.Join(fn.BundleDir, "forge-function.json")
	data, err := json.MarshalIndent(fn, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// loadExisting scans the bundle directory for deployed functions.
func (d *Deployer) loadExisting() int {
	entries, err := os.ReadDir(d.bundleDir)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metaPath := filepath.Join(d.bundleDir, entry.Name(), "forge-function.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var fn Function
		if err := json.Unmarshal(data, &fn); err != nil {
			continue
		}

		d.functions[fn.Name] = &fn
		count++
	}

	return count
}
