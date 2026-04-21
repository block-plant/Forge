package functions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
	"github.com/ayushkunwarsingh/forge/server"
)

// Service is the top-level functions service that owns the deployer,
// runtime, trigger manager, and scheduler.
type Service struct {
	Deployer  *Deployer
	Runtime   *Runtime
	Triggers  *TriggerManager
	Scheduler *Scheduler
	log       *logger.Logger
}

// NewService creates the complete functions service.
func NewService(cfg *config.Config, log *logger.Logger) (*Service, error) {
	deployer, err := NewDeployer(cfg, log)
	if err != nil {
		return nil, err
	}

	runtime := NewRuntime(cfg, log)
	triggers := NewTriggerManager(deployer, runtime, log)
	scheduler := NewScheduler(deployer, runtime, log)

	return &Service{
		Deployer:  deployer,
		Runtime:   runtime,
		Triggers:  triggers,
		Scheduler: scheduler,
		log:       log,
	}, nil
}

// Start initializes background services (scheduler).
func (s *Service) Start() {
	s.Scheduler.Start()
}

// RegisterRoutes registers all function HTTP endpoints on the router.
func RegisterRoutes(router *server.Router, svc *Service) {
	g := router.Group("/functions")

	// Deploy a function
	g.POST("/deploy", handleDeploy(svc))

	// List all functions
	g.GET("/list", handleList(svc))

	// Get function details
	g.GET("/get/:name", handleGet(svc))

	// Delete a function
	g.DELETE("/:name", handleDelete(svc))

	// Invoke a function manually
	g.POST("/invoke/:name", handleInvoke(svc))

	// Get function logs (last execution results)
	g.GET("/logs/:name", handleLogs(svc))

	// List scheduled jobs
	g.GET("/schedules", handleSchedules(svc))

	// Stats
	g.GET("/stats", handleFuncStats(svc))
}

// handleDeploy deploys a new function.
func handleDeploy(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		var body struct {
			Name        string          `json:"name"`
			Source      string          `json:"source"`
			EntryPoint  string          `json:"entry_point"`
			Runtime     string          `json:"runtime"`
			Description string          `json:"description"`
			Triggers    []TriggerConfig `json:"triggers"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.Error(400, "Invalid JSON body")
			return
		}

		if body.Name == "" {
			ctx.Error(400, "Function name is required")
			return
		}
		if body.Source == "" {
			ctx.Error(400, "Function source code is required")
			return
		}

		fn, err := svc.Deployer.Deploy(
			body.Name,
			[]byte(body.Source),
			body.EntryPoint,
			body.Runtime,
			body.Triggers,
		)
		if err != nil {
			ctx.Error(500, err.Error())
			return
		}

		if body.Description != "" {
			fn.Description = body.Description
		}

		ctx.JSON(201, map[string]interface{}{
			"status":   "ok",
			"message":  "Function deployed successfully",
			"name":     fn.Name,
			"version":  fn.Version,
			"runtime":  fn.Runtime,
			"triggers": fn.Triggers,
		})
	}
}

// handleList returns all deployed functions.
func handleList(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		functions := svc.Deployer.List()

		items := make([]map[string]interface{}, 0, len(functions))
		for _, fn := range functions {
			items = append(items, map[string]interface{}{
				"name":        fn.Name,
				"runtime":     fn.Runtime,
				"version":     fn.Version,
				"status":      fn.Status,
				"triggers":    fn.Triggers,
				"created_at":  fn.CreatedAt.Format(time.RFC3339),
				"updated_at":  fn.UpdatedAt.Format(time.RFC3339),
			})
		}

		ctx.JSON(200, map[string]interface{}{
			"status":    "ok",
			"count":     len(items),
			"functions": items,
		})
	}
}

// handleGet returns details for a specific function.
func handleGet(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		name := ctx.Param("name")
		fn, ok := svc.Deployer.Get(name)
		if !ok {
			ctx.Error(404, fmt.Sprintf("Function %q not found", name))
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":      "ok",
			"name":        fn.Name,
			"description": fn.Description,
			"runtime":     fn.Runtime,
			"entry_point": fn.EntryPoint,
			"version":     fn.Version,
			"triggers":    fn.Triggers,
			"status_val":  fn.Status,
			"created_at":  fn.CreatedAt.Format(time.RFC3339),
			"updated_at":  fn.UpdatedAt.Format(time.RFC3339),
		})
	}
}

// handleDelete removes a deployed function.
func handleDelete(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		name := ctx.Param("name")
		if err := svc.Deployer.Delete(name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				ctx.Error(404, err.Error())
			} else {
				ctx.Error(500, err.Error())
			}
			return
		}

		ctx.JSON(200, map[string]interface{}{
			"status":  "ok",
			"message": fmt.Sprintf("Function %q deleted", name),
		})
	}
}

// handleInvoke manually invokes a deployed function.
func handleInvoke(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		name := ctx.Param("name")
		fn, ok := svc.Deployer.Get(name)
		if !ok {
			ctx.Error(404, fmt.Sprintf("Function %q not found", name))
			return
		}

		// Parse payload
		var payload map[string]interface{}
		if body := ctx.BodyBytes(); len(body) > 0 {
			json.Unmarshal(body, &payload)
		}
		if payload == nil {
			payload = make(map[string]interface{})
		}

		req := &ExecRequest{
			FunctionName: fn.Name,
			Trigger:      "http",
			Payload:      payload,
		}

		result := svc.Runtime.Execute(fn, req)

		statusCode := 200
		if !result.Success {
			statusCode = 500
		}

		ctx.JSON(statusCode, map[string]interface{}{
			"status":    result.Success,
			"output":    result.Output,
			"error":     result.Error,
			"duration":  result.Duration.String(),
			"exit_code": result.ExitCode,
		})
	}
}

// handleLogs returns execution logs for a function from persistent storage.
func handleLogs(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		name := ctx.Param("name")
		fn, ok := svc.Deployer.Get(name)
		if !ok {
			ctx.Error(404, fmt.Sprintf("Function %q not found", name))
			return
		}

		// Read the logs.jsonl file from the function's bundle directory
		logPath := fn.BundleDir + "/logs.jsonl"
		data, err := readFileBytes(logPath)
		if err != nil {
			// No logs yet — return empty
			ctx.JSON(200, map[string]interface{}{
				"status":   "ok",
				"function": name,
				"count":    0,
				"logs":     []interface{}{},
			})
			return
		}

		// Parse limit from query string (default 100)
		limit := 100
		if l := ctx.QueryParam("limit"); l != "" {
			if parsed := parsePositiveInt(l); parsed > 0 {
				limit = parsed
			}
		}

		// Parse each JSONL line into a map
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		logs := make([]interface{}, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry map[string]interface{}
			if json.Unmarshal([]byte(line), &entry) == nil {
				logs = append(logs, entry)
			}
		}

		// Reverse so newest logs appear first
		for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
			logs[i], logs[j] = logs[j], logs[i]
		}

		// Apply limit
		if len(logs) > limit {
			logs = logs[:limit]
		}

		ctx.JSON(200, map[string]interface{}{
			"status":   "ok",
			"function": name,
			"count":    len(logs),
			"logs":     logs,
		})
	}
}

// handleSchedules returns all scheduled jobs.
func handleSchedules(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		jobs := svc.Scheduler.Jobs()

		items := make([]map[string]interface{}, 0, len(jobs))
		for _, job := range jobs {
			item := map[string]interface{}{
				"function":    job.FunctionName,
				"schedule":    job.Schedule,
				"next_run":    job.NextRun.Format(time.RFC3339),
				"run_count":   job.RunCount,
				"last_result": job.LastResult,
			}
			if !job.LastRun.IsZero() {
				item["last_run"] = job.LastRun.Format(time.RFC3339)
			}
			items = append(items, item)
		}

		ctx.JSON(200, map[string]interface{}{
			"status": "ok",
			"count":  len(items),
			"jobs":   items,
		})
	}
}

// handleFuncStats returns functions service statistics.
func handleFuncStats(svc *Service) server.HandlerFunc {
	return func(ctx *server.Context) {
		ctx.JSON(200, map[string]interface{}{
			"status":          "ok",
			"deployed_count":  svc.Deployer.Count(),
			"scheduled_jobs":  len(svc.Scheduler.Jobs()),
		})
	}
}

// readFileBytes reads the entire file at the given path.
// Returns the bytes and any error (e.g., file not found).
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// parsePositiveInt parses a string into a positive integer.
// Returns 0 if the string is not a valid positive integer.
func parsePositiveInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
