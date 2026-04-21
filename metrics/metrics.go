// Package metrics provides lock-free, zero-dependency telemetry primitives for
// the Forge platform. Counters use atomic operations; histograms use a ring
// buffer of int64 nanosecond samples for percentile computation.
package metrics

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ---- Counter ----

// Counter is a monotonically increasing int64 counter, safe for concurrent use.
type Counter struct {
	v int64
}

// Inc increments the counter by 1.
func (c *Counter) Inc() { atomic.AddInt64(&c.v, 1) }

// Add increments the counter by delta.
func (c *Counter) Add(delta int64) { atomic.AddInt64(&c.v, delta) }

// Value returns the current counter value.
func (c *Counter) Value() int64 { return atomic.LoadInt64(&c.v) }

// Reset resets the counter to zero and returns the previous value.
func (c *Counter) Reset() int64 { return atomic.SwapInt64(&c.v, 0) }

// ---- Gauge ----

// Gauge is an int64 that can go up and down.
type Gauge struct {
	v int64
}

// Set sets the gauge to v.
func (g *Gauge) Set(v int64) { atomic.StoreInt64(&g.v, v) }

// Inc increments the gauge.
func (g *Gauge) Inc() { atomic.AddInt64(&g.v, 1) }

// Dec decrements the gauge.
func (g *Gauge) Dec() { atomic.AddInt64(&g.v, -1) }

// Value returns the current gauge value.
func (g *Gauge) Value() int64 { return atomic.LoadInt64(&g.v) }

// ---- Histogram ----

// Histogram records int64 samples (nanoseconds) in a fixed-size ring buffer.
// It is safe for concurrent use; percentiles are computed on a snapshot.
type Histogram struct {
	mu      sync.Mutex
	buf     []int64
	size    int
	count   int64
	total   int64
	pos     int
}

// NewHistogram creates a histogram with the given ring buffer size.
func NewHistogram(size int) *Histogram {
	return &Histogram{
		buf:  make([]int64, size),
		size: size,
	}
}

// Observe records a single duration sample.
func (h *Histogram) Observe(d time.Duration) {
	ns := d.Nanoseconds()
	h.mu.Lock()
	h.buf[h.pos%h.size] = ns
	h.pos++
	atomic.AddInt64(&h.count, 1)
	atomic.AddInt64(&h.total, ns)
	h.mu.Unlock()
}

// Snapshot returns a sorted copy of the current samples.
func (h *Histogram) snapshot() []int64 {
	h.mu.Lock()
	n := h.size
	if int(atomic.LoadInt64(&h.count)) < h.size {
		n = int(atomic.LoadInt64(&h.count))
	}
	cp := make([]int64, n)
	copy(cp, h.buf[:n])
	h.mu.Unlock()
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp
}

// Percentile returns the p-th percentile of recorded samples (0–100).
func (h *Histogram) Percentile(p float64) time.Duration {
	snap := h.snapshot()
	if len(snap) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(snap)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(snap) {
		idx = len(snap) - 1
	}
	return time.Duration(snap[idx])
}

// Count returns the total number of observations.
func (h *Histogram) Count() int64 { return atomic.LoadInt64(&h.count) }

// Mean returns the mean duration.
func (h *Histogram) Mean() time.Duration {
	c := h.Count()
	if c == 0 {
		return 0
	}
	return time.Duration(atomic.LoadInt64(&h.total) / c)
}

// ---- Registry ----

// Registry is the central store for all named metrics.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
	startTime  time.Time
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		startTime:  time.Now(),
	}
}

// Counter returns (or creates) a named counter.
func (r *Registry) Counter(name string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{}
	r.counters[name] = c
	return c
}

// Gauge returns (or creates) a named gauge.
func (r *Registry) Gauge(name string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g := &Gauge{}
	r.gauges[name] = g
	return g
}

// Histogram returns (or creates) a named histogram with a 1024-sample ring buffer.
func (r *Registry) Histogram(name string) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histograms[name]; ok {
		return h
	}
	h := NewHistogram(1024)
	r.histograms[name] = h
	return h
}

// Snapshot returns a point-in-time copy of all metric values.
func (r *Registry) Snapshot() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]interface{})

	// Runtime stats
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	out["runtime.goroutines"] = runtime.NumGoroutine()
	out["runtime.heap_alloc_bytes"] = ms.HeapAlloc
	out["runtime.heap_sys_bytes"] = ms.HeapSys
	out["runtime.gc_runs"] = ms.NumGC
	out["uptime_seconds"] = int64(time.Since(r.startTime).Seconds())

	// Counters
	for name, c := range r.counters {
		out["counter."+name] = c.Value()
	}

	// Gauges
	for name, g := range r.gauges {
		out["gauge."+name] = g.Value()
	}

	// Histograms
	for name, h := range r.histograms {
		out[fmt.Sprintf("hist.%s.count", name)] = h.Count()
		out[fmt.Sprintf("hist.%s.mean_ms", name)] = h.Mean().Milliseconds()
		out[fmt.Sprintf("hist.%s.p50_ms", name)] = h.Percentile(50).Milliseconds()
		out[fmt.Sprintf("hist.%s.p95_ms", name)] = h.Percentile(95).Milliseconds()
		out[fmt.Sprintf("hist.%s.p99_ms", name)] = h.Percentile(99).Milliseconds()
	}

	return out
}

// Default is the global default registry. Use this for process-wide metrics.
var Default = NewRegistry()
