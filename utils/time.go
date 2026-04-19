package utils

import (
	"fmt"
	"time"
)

// NowUnix returns the current time as a Unix timestamp in seconds.
func NowUnix() int64 {
	return time.Now().Unix()
}

// NowUnixMilli returns the current time as a Unix timestamp in milliseconds.
func NowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// NowISO returns the current time formatted as ISO 8601 (RFC 3339).
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// NowISOMilli returns the current time formatted as ISO 8601 with millisecond precision.
func NowISOMilli() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// UnixToTime converts a Unix timestamp (seconds) to a time.Time value.
func UnixToTime(ts int64) time.Time {
	return time.Unix(ts, 0).UTC()
}

// UnixMilliToTime converts a Unix timestamp (milliseconds) to a time.Time value.
func UnixMilliToTime(ts int64) time.Time {
	return time.UnixMilli(ts).UTC()
}

// TimeToISO formats a time.Time as ISO 8601 string.
func TimeToISO(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseISO parses an ISO 8601 (RFC 3339) formatted time string.
func ParseISO(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try with nanosecond precision
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse ISO time %q: %w", s, err)
		}
	}
	return t.UTC(), nil
}

// ParseDuration parses a human-readable duration string.
// Supports Go duration format: "300ms", "1.5h", "2h45m", etc.
func ParseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration %q: %w", s, err)
	}
	return d, nil
}

// FormatDuration formats a duration as a human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dμs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// IsExpired checks if a Unix timestamp (seconds) is in the past.
func IsExpired(ts int64) bool {
	return time.Now().Unix() > ts
}

// ExpiresIn returns the duration until a Unix timestamp (seconds).
// Returns a negative duration if the time has already passed.
func ExpiresIn(ts int64) time.Duration {
	return time.Until(UnixToTime(ts))
}
