// Package logger provides structured, concurrent-safe logging for Forge.
// It supports JSON and pretty-print output modes with configurable log levels.
package logger

import "strings"

// Level represents the severity of a log message.
type Level int

const (
	// DEBUG is for detailed diagnostic information.
	DEBUG Level = iota
	// INFO is for general operational information.
	INFO
	// WARN is for potentially harmful situations.
	WARN
	// ERROR is for error events that might still allow the application to continue.
	ERROR
	// FATAL is for severe error events that will likely lead the application to abort.
	FATAL
)

// String returns the human-readable name of the log level.
func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Color returns the ANSI color code for the log level (for pretty-print mode).
func (l Level) Color() string {
	switch l {
	case DEBUG:
		return "\033[36m" // Cyan
	case INFO:
		return "\033[32m" // Green
	case WARN:
		return "\033[33m" // Yellow
	case ERROR:
		return "\033[31m" // Red
	case FATAL:
		return "\033[35m" // Magenta
	default:
		return "\033[0m" // Reset
	}
}

// ColorReset is the ANSI escape code to reset terminal color.
const ColorReset = "\033[0m"

// ParseLevel converts a string to a Level.
// Returns INFO if the string is not recognized.
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}
