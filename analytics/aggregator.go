package analytics

import (
	"sync"
	"time"
)

// Aggregator provides time-series bucketing and aggregation of events.
// It maintains rolling windows at minute, hour, and day granularity
// for fast dashboard queries without scanning raw event logs.
type Aggregator struct {
	mu sync.RWMutex

	// minuteBuckets stores counts per event name per minute.
	// Key format: "YYYY-MM-DDTHH:MM"
	minuteBuckets map[string]map[string]int64

	// hourBuckets stores counts per event name per hour.
	// Key format: "YYYY-MM-DDTHH"
	hourBuckets map[string]map[string]int64

	// dayBuckets stores counts per event name per day.
	// Key format: "YYYY-MM-DD"
	dayBuckets map[string]map[string]int64

	// retentionMinutes is how many minutes of minute-level data to keep.
	retentionMinutes int
	// retentionHours is how many hours of hour-level data to keep.
	retentionHours int
	// retentionDays is how many days of day-level data to keep.
	retentionDays int
}

// TimeSeriesPoint is a single data point in a time series.
type TimeSeriesPoint struct {
	Timestamp string `json:"timestamp"`
	Count     int64  `json:"count"`
}

// NewAggregator creates a new time-series aggregator.
func NewAggregator() *Aggregator {
	a := &Aggregator{
		minuteBuckets:    make(map[string]map[string]int64),
		hourBuckets:      make(map[string]map[string]int64),
		dayBuckets:       make(map[string]map[string]int64),
		retentionMinutes: 60 * 24, // 24 hours of minute data
		retentionHours:   24 * 30, // 30 days of hour data
		retentionDays:    365,     // 1 year of day data
	}

	// Start cleanup loop
	go a.cleanupLoop()

	return a
}

// Record adds an event to all applicable time buckets.
func (a *Aggregator) Record(eventName string, ts time.Time) {
	minuteKey := ts.UTC().Format("2006-01-02T15:04")
	hourKey := ts.UTC().Format("2006-01-02T15")
	dayKey := ts.UTC().Format("2006-01-02")

	a.mu.Lock()
	defer a.mu.Unlock()

	// Minute bucket
	if a.minuteBuckets[minuteKey] == nil {
		a.minuteBuckets[minuteKey] = make(map[string]int64)
	}
	a.minuteBuckets[minuteKey][eventName]++

	// Hour bucket
	if a.hourBuckets[hourKey] == nil {
		a.hourBuckets[hourKey] = make(map[string]int64)
	}
	a.hourBuckets[hourKey][eventName]++

	// Day bucket
	if a.dayBuckets[dayKey] == nil {
		a.dayBuckets[dayKey] = make(map[string]int64)
	}
	a.dayBuckets[dayKey][eventName]++
}

// QueryMinutes returns minute-level time series for a given event within a range.
func (a *Aggregator) QueryMinutes(eventName string, from, to time.Time) []TimeSeriesPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var points []TimeSeriesPoint
	current := from.Truncate(time.Minute)

	for !current.After(to) {
		key := current.UTC().Format("2006-01-02T15:04")
		count := int64(0)
		if bucket, ok := a.minuteBuckets[key]; ok {
			count = bucket[eventName]
		}
		points = append(points, TimeSeriesPoint{
			Timestamp: key,
			Count:     count,
		})
		current = current.Add(time.Minute)
	}

	return points
}

// QueryHours returns hour-level time series for a given event within a range.
func (a *Aggregator) QueryHours(eventName string, from, to time.Time) []TimeSeriesPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var points []TimeSeriesPoint
	current := from.Truncate(time.Hour)

	for !current.After(to) {
		key := current.UTC().Format("2006-01-02T15")
		count := int64(0)
		if bucket, ok := a.hourBuckets[key]; ok {
			count = bucket[eventName]
		}
		points = append(points, TimeSeriesPoint{
			Timestamp: key,
			Count:     count,
		})
		current = current.Add(time.Hour)
	}

	return points
}

// QueryDays returns day-level time series for a given event within a range.
func (a *Aggregator) QueryDays(eventName string, from, to time.Time) []TimeSeriesPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var points []TimeSeriesPoint
	current := from.Truncate(24 * time.Hour)

	for !current.After(to) {
		key := current.UTC().Format("2006-01-02")
		count := int64(0)
		if bucket, ok := a.dayBuckets[key]; ok {
			count = bucket[eventName]
		}
		points = append(points, TimeSeriesPoint{
			Timestamp: key,
			Count:     count,
		})
		current = current.Add(24 * time.Hour)
	}

	return points
}

// TopEvents returns the top N events by count for a given day.
func (a *Aggregator) TopEvents(day string, limit int) []map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	bucket, ok := a.dayBuckets[day]
	if !ok {
		return nil
	}

	// Collect and sort by count (simple selection since N is small)
	type kv struct {
		Name  string
		Count int64
	}
	var items []kv
	for name, count := range bucket {
		items = append(items, kv{name, count})
	}

	// Sort descending by count (insertion sort for small N)
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j].Count > items[j-1].Count {
			items[j], items[j-1] = items[j-1], items[j]
			j--
		}
	}

	if limit > len(items) {
		limit = len(items)
	}

	result := make([]map[string]interface{}, limit)
	for i := 0; i < limit; i++ {
		result[i] = map[string]interface{}{
			"name":  items[i].Name,
			"count": items[i].Count,
		}
	}
	return result
}

// Stats returns aggregator statistics.
func (a *Aggregator) Stats() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]interface{}{
		"minute_buckets": len(a.minuteBuckets),
		"hour_buckets":   len(a.hourBuckets),
		"day_buckets":    len(a.dayBuckets),
	}
}

// cleanupLoop periodically removes expired buckets.
func (a *Aggregator) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		a.cleanup()
	}
}

// cleanup removes entries older than the retention periods.
func (a *Aggregator) cleanup() {
	now := time.Now().UTC()
	a.mu.Lock()
	defer a.mu.Unlock()

	// Cleanup minute buckets
	minCutoff := now.Add(-time.Duration(a.retentionMinutes) * time.Minute).Format("2006-01-02T15:04")
	for key := range a.minuteBuckets {
		if key < minCutoff {
			delete(a.minuteBuckets, key)
		}
	}

	// Cleanup hour buckets
	hourCutoff := now.Add(-time.Duration(a.retentionHours) * time.Hour).Format("2006-01-02T15")
	for key := range a.hourBuckets {
		if key < hourCutoff {
			delete(a.hourBuckets, key)
		}
	}

	// Cleanup day buckets
	dayCutoff := now.Add(-time.Duration(a.retentionDays) * 24 * time.Hour).Format("2006-01-02")
	for key := range a.dayBuckets {
		if key < dayCutoff {
			delete(a.dayBuckets, key)
		}
	}
}
