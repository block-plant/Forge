package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Fields is a map of structured key-value pairs attached to a log entry.
type Fields map[string]interface{}

// Logger is a structured, concurrent-safe logger.
// It supports both JSON output (for production) and pretty-print (for development).
type Logger struct {
	mu       sync.Mutex
	output   io.Writer
	level    Level
	pretty   bool
	fields   Fields // persistent fields attached to every log entry
	service  string
}

// Config holds configuration for creating a new Logger.
type Config struct {
	// Output is the writer where logs are sent. Defaults to os.Stdout.
	Output io.Writer
	// Level is the minimum log level. Messages below this level are discarded.
	Level Level
	// Pretty enables color-coded, human-readable output instead of JSON.
	Pretty bool
	// Service is the service name attached to every log entry.
	Service string
}

// New creates a new Logger with the given configuration.
func New(cfg Config) *Logger {
	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}
	return &Logger{
		output:  output,
		level:   cfg.Level,
		pretty:  cfg.Pretty,
		fields:  make(Fields),
		service: cfg.Service,
	}
}

// Default creates a logger with sensible defaults for development.
func Default() *Logger {
	return New(Config{
		Output:  os.Stdout,
		Level:   DEBUG,
		Pretty:  true,
		Service: "forge",
	})
}

// WithFields returns a new Logger that includes the given fields in every log entry.
// The original logger is not modified.
func (l *Logger) WithFields(fields Fields) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	merged := make(Fields, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}

	return &Logger{
		output:  l.output,
		level:   l.level,
		pretty:  l.pretty,
		fields:  merged,
		service: l.service,
	}
}

// WithField returns a new Logger that includes a single additional field.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return l.WithFields(Fields{key: value})
}

// SetLevel changes the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Debug logs a message at DEBUG level.
func (l *Logger) Debug(msg string, fields ...Fields) {
	l.log(DEBUG, msg, fields...)
}

// Info logs a message at INFO level.
func (l *Logger) Info(msg string, fields ...Fields) {
	l.log(INFO, msg, fields...)
}

// Warn logs a message at WARN level.
func (l *Logger) Warn(msg string, fields ...Fields) {
	l.log(WARN, msg, fields...)
}

// Error logs a message at ERROR level.
func (l *Logger) Error(msg string, fields ...Fields) {
	l.log(ERROR, msg, fields...)
}

// Fatal logs a message at FATAL level and exits the process.
func (l *Logger) Fatal(msg string, fields ...Fields) {
	l.log(FATAL, msg, fields...)
	os.Exit(1)
}

// log is the internal method that formats and writes the log entry.
func (l *Logger) log(level Level, msg string, extraFields ...Fields) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	now := time.Now()

	if l.pretty {
		l.writePretty(level, msg, now, extraFields...)
	} else {
		l.writeJSON(level, msg, now, extraFields...)
	}
}

// writeJSON writes a structured JSON log entry.
func (l *Logger) writeJSON(level Level, msg string, now time.Time, extraFields ...Fields) {
	entry := make(map[string]interface{}, len(l.fields)+5)

	entry["timestamp"] = now.UTC().Format(time.RFC3339Nano)
	entry["level"] = level.String()
	entry["message"] = msg

	if l.service != "" {
		entry["service"] = l.service
	}

	// Add persistent fields
	for k, v := range l.fields {
		entry[k] = v
	}

	// Add extra fields from this specific call
	for _, f := range extraFields {
		for k, v := range f {
			entry[k] = v
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback: write raw message if JSON marshaling fails
		fmt.Fprintf(l.output, `{"timestamp":"%s","level":"%s","message":"%s","error":"marshal_failed"}`+"\n",
			now.UTC().Format(time.RFC3339Nano), level.String(), msg)
		return
	}

	l.output.Write(data)
	l.output.Write([]byte("\n"))
}

// writePretty writes a color-coded, human-readable log entry.
func (l *Logger) writePretty(level Level, msg string, now time.Time, extraFields ...Fields) {
	timeStr := now.Format("15:04:05.000")
	levelStr := fmt.Sprintf("%s%-5s%s", level.Color(), level.String(), ColorReset)

	// Build the base log line
	line := fmt.Sprintf("%s %s %s", timeStr, levelStr, msg)

	// Collect all fields
	allFields := make(Fields, len(l.fields))
	for k, v := range l.fields {
		allFields[k] = v
	}
	for _, f := range extraFields {
		for k, v := range f {
			allFields[k] = v
		}
	}

	// Append fields inline
	if len(allFields) > 0 {
		line += " \033[90m|"
		for k, v := range allFields {
			line += fmt.Sprintf(" %s=%v", k, v)
		}
		line += ColorReset
	}

	fmt.Fprintln(l.output, line)
}
