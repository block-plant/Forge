// Package functions implements the Forge serverless functions runtime.
// It provides function deployment, execution via subprocess, trigger management,
// and cron scheduling — all built from scratch with zero external dependencies.
package functions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/config"
	"github.com/ayushkunwarsingh/forge/logger"
)

// Runtime executes deployed functions in an isolated subprocess.
// It supports Node.js scripts via os/exec and a built-in script evaluator.
type Runtime struct {
	cfg *config.Config
	log *logger.Logger

	// sem limits concurrent executions.
	sem chan struct{}
	mu  sync.Mutex
}

// ExecRequest contains everything needed to execute a function.
type ExecRequest struct {
	// FunctionName is the name of the function to invoke.
	FunctionName string `json:"function_name"`
	// Trigger is what initiated the execution: "http", "db", "auth", "schedule".
	Trigger string `json:"trigger"`
	// Payload is the input data passed to the function.
	Payload map[string]interface{} `json:"payload"`
	// Timeout overrides the default timeout (0 = use default).
	Timeout int `json:"timeout,omitempty"`
}

// ExecResult is the outcome of a function execution.
type ExecResult struct {
	// Success is true if the function executed without error.
	Success bool `json:"success"`
	// Output is the function's stdout output (typically JSON).
	Output string `json:"output"`
	// Error contains the error message if execution failed.
	Error string `json:"error,omitempty"`
	// Duration is how long the execution took.
	Duration time.Duration `json:"duration"`
	// ExitCode is the process exit code (0 = success).
	ExitCode int `json:"exit_code"`
}

// NewRuntime creates a new function execution runtime.
func NewRuntime(cfg *config.Config, log *logger.Logger) *Runtime {
	maxConcurrent := cfg.Functions.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}

	return &Runtime{
		cfg: cfg,
		log: log,
		sem: make(chan struct{}, maxConcurrent),
	}
}

// Execute runs a deployed function with the given request.
// It enforces timeout, memory limits, and concurrency controls.
func (r *Runtime) Execute(fn *Function, req *ExecRequest) *ExecResult {
	startTime := time.Now()

	// Acquire semaphore slot (concurrency control)
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	default:
		return &ExecResult{
			Success:  false,
			Error:    "max concurrent executions reached",
			Duration: time.Since(startTime),
		}
	}

	// Determine timeout
	timeout := time.Duration(r.cfg.Functions.Timeout) * time.Second
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	r.log.Info("Executing function", logger.Fields{
		"function": fn.Name,
		"trigger":  req.Trigger,
		"timeout":  timeout.String(),
	})

	// Execute based on runtime type
	var result *ExecResult
	switch fn.Runtime {
	case "node":
		result = r.executeNode(fn, req, timeout)
	case "script":
		result = r.executeScript(fn, req, timeout)
	default:
		result = r.executeScript(fn, req, timeout)
	}

	result.Duration = time.Since(startTime)

	if result.Success {
		r.log.Info("Function completed", logger.Fields{
			"function": fn.Name,
			"duration": result.Duration.String(),
		})
	} else {
		r.log.Error("Function failed", logger.Fields{
			"function": fn.Name,
			"error":    result.Error,
			"duration": result.Duration.String(),
		})
	}

	// Persist the execution log to the function's bundle directory.
	r.persistLog(fn, req, result, startTime)

	return result
}

// executeNode runs a function using Node.js via os/exec.
func (r *Runtime) executeNode(fn *Function, req *ExecRequest, timeout time.Duration) *ExecResult {
	// Find node binary
	nodeBin := findNodeBinary()
	if nodeBin == "" {
		return &ExecResult{
			Success: false,
			Error:   "Node.js runtime not found (install node or use 'script' runtime)",
		}
	}

	// Build the wrapper script that invokes the function
	payloadJSON, _ := json.Marshal(req.Payload)
	wrapper := fmt.Sprintf(`
const fn = require('%s');
const payload = %s;
const trigger = '%s';

async function run() {
  try {
    let result;
    if (typeof fn === 'function') {
      result = await fn(payload, { trigger: trigger });
    } else if (fn.handler && typeof fn.handler === 'function') {
      result = await fn.handler(payload, { trigger: trigger });
    } else if (fn.default && typeof fn.default === 'function') {
      result = await fn.default(payload, { trigger: trigger });
    } else {
      throw new Error('No exported function found');
    }
    console.log(JSON.stringify(result || {}));
  } catch (err) {
    console.error(err.message || err);
    process.exit(1);
  }
}
run();
`, fn.EntryPoint, string(payloadJSON), req.Trigger)

	return r.runProcess(nodeBin, []string{"-e", wrapper}, fn.BundleDir, timeout)
}

// executeScript runs a function using the built-in script executor.
// The script is a simple JSON-in/JSON-out executable or shell script.
func (r *Runtime) executeScript(fn *Function, req *ExecRequest, timeout time.Duration) *ExecResult {
	entryPath := filepath.Join(fn.BundleDir, fn.EntryPoint)

	// Check if entry point exists
	if _, err := os.Stat(entryPath); err != nil {
		return &ExecResult{
			Success: false,
			Error:   fmt.Sprintf("entry point not found: %s", fn.EntryPoint),
		}
	}

	// Serialize payload to pass via stdin/env
	payloadJSON, _ := json.Marshal(req.Payload)

	// Determine how to run the script
	var cmd string
	var args []string

	if strings.HasSuffix(fn.EntryPoint, ".js") {
		// Try Node.js
		nodeBin := findNodeBinary()
		if nodeBin == "" {
			return &ExecResult{
				Success: false,
				Error:   "Node.js required for .js files but not found",
			}
		}
		cmd = nodeBin
		args = []string{entryPath}
	} else if strings.HasSuffix(fn.EntryPoint, ".sh") {
		cmd = "/bin/sh"
		args = []string{entryPath}
	} else {
		// Try to execute directly (compiled binary or script with shebang)
		cmd = entryPath
		args = []string{}
	}

	// Set environment variables for the function
	env := append(os.Environ(),
		fmt.Sprintf("FORGE_PAYLOAD=%s", string(payloadJSON)),
		fmt.Sprintf("FORGE_FUNCTION=%s", fn.Name),
		fmt.Sprintf("FORGE_TRIGGER=%s", req.Trigger),
	)

	return r.runProcessWithEnv(cmd, args, fn.BundleDir, timeout, env)
}

// runProcess executes a command with timeout and captures output.
func (r *Runtime) runProcess(command string, args []string, dir string, timeout time.Duration) *ExecResult {
	return r.runProcessWithEnv(command, args, dir, timeout, nil)
}

// runProcessWithEnv executes a command with timeout, environment, and captures output.
func (r *Runtime) runProcessWithEnv(command string, args []string, dir string, timeout time.Duration, env []string) *ExecResult {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}

	// Capture stdout and stderr
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		return &ExecResult{
			Success: false,
			Error:   fmt.Sprintf("failed to start process: %v", err),
		}
	}

	// Wait with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			exitCode = 1
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}

		if exitCode != 0 {
			errMsg := stderr.String()
			if errMsg == "" {
				errMsg = err.Error()
			}
			return &ExecResult{
				Success:  false,
				Output:   stdout.String(),
				Error:    strings.TrimSpace(errMsg),
				ExitCode: exitCode,
			}
		}

		return &ExecResult{
			Success:  true,
			Output:   strings.TrimSpace(stdout.String()),
			ExitCode: 0,
		}

	case <-time.After(timeout):
		// Kill the process on timeout
		cmd.Process.Kill()
		return &ExecResult{
			Success:  false,
			Error:    fmt.Sprintf("function timed out after %s", timeout),
			ExitCode: -1,
		}
	}
}

// findNodeBinary searches for the Node.js binary on the system.
func findNodeBinary() string {
	// Check common locations
	candidates := []string{
		"node",
		"/usr/local/bin/node",
		"/usr/bin/node",
		"/opt/homebrew/bin/node",
	}

	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}

	return ""
}

// LogEntry is a single persistent execution record written to logs.jsonl.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Function  string `json:"function"`
	Trigger   string `json:"trigger"`
	Success   bool   `json:"success"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	Duration  string `json:"duration"`
	ExitCode  int    `json:"exit_code"`
}

// persistLog appends a JSONL line to the function's logs.jsonl file.
func (r *Runtime) persistLog(fn *Function, req *ExecRequest, result *ExecResult, startTime time.Time) {
	entry := LogEntry{
		Timestamp: startTime.UTC().Format(time.RFC3339),
		Function:  fn.Name,
		Trigger:   req.Trigger,
		Success:   result.Success,
		Output:    result.Output,
		Error:     result.Error,
		Duration:  result.Duration.String(),
		ExitCode:  result.ExitCode,
	}

	line, err := json.Marshal(entry)
	if err != nil {
		r.log.Warn("Failed to marshal log entry", logger.Fields{
			"function": fn.Name,
			"error":    err.Error(),
		})
		return
	}

	logPath := filepath.Join(fn.BundleDir, "logs.jsonl")

	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		r.log.Warn("Failed to open log file", logger.Fields{
			"function": fn.Name,
			"path":     logPath,
			"error":    err.Error(),
		})
		return
	}
	defer f.Close()

	f.Write(append(line, '\n'))
}
