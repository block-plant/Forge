// Package load provides a zero-dependency load testing harness for the Forge
// platform. It exercises the HTTP API endpoints using concurrent goroutines and
// reports throughput, latency percentiles, and error rates.
//
// Run it as a Go test:
//
//	go test ./load/ -run TestForgeLoad -v -endpoint http://localhost:8080 -rps 100 -duration 10s
package load

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- Config ----

var (
	endpoint = flag.String("endpoint", "http://localhost:8080", "Forge server endpoint")
	rps      = flag.Int("rps", 50, "Target requests per second")
	duration = flag.Duration("duration", 10*time.Second, "Test duration")
)

// ---- Result ----

// Result holds the outcome of a single request.
type Result struct {
	Latency    time.Duration
	StatusCode int
	Err        error
}

// ---- LoadRunner ----

// LoadRunner drives load against a single endpoint.
type LoadRunner struct {
	Name        string
	Method      string
	URL         string
	Body        []byte
	ContentType string

	total   int64
	errors  int64
	latency []int64
	mu      sync.Mutex
}

// Run executes `rpsTarget` requests per second for `dur`, returns summary.
func (lr *LoadRunner) Run(ctx context.Context, rpsTarget int, dur time.Duration) {
	interval := time.Second / time.Duration(rpsTarget)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	deadline := time.Now().Add(dur)
	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if t.After(deadline) {
				return
			}
			go lr.do(client)
		}
	}
}

func (lr *LoadRunner) do(client *http.Client) {
	start := time.Now()

	var body io.Reader
	if len(lr.Body) > 0 {
		body = bytes.NewReader(lr.Body)
	}

	req, err := http.NewRequest(lr.Method, lr.URL, body)
	if err != nil {
		atomic.AddInt64(&lr.errors, 1)
		return
	}
	if lr.ContentType != "" {
		req.Header.Set("Content-Type", lr.ContentType)
	}

	resp, err := client.Do(req)
	elapsed := time.Since(start)
	atomic.AddInt64(&lr.total, 1)

	if err != nil {
		atomic.AddInt64(&lr.errors, 1)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		atomic.AddInt64(&lr.errors, 1)
	}

	lr.mu.Lock()
	lr.latency = append(lr.latency, elapsed.Nanoseconds())
	lr.mu.Unlock()
}

// Percentile returns the p-th percentile latency.
func (lr *LoadRunner) Percentile(p float64) time.Duration {
	lr.mu.Lock()
	snap := make([]int64, len(lr.latency))
	copy(snap, lr.latency)
	lr.mu.Unlock()

	if len(snap) == 0 {
		return 0
	}
	sort.Slice(snap, func(i, j int) bool { return snap[i] < snap[j] })
	idx := int(math.Ceil(p/100.0*float64(len(snap)))) - 1
	if idx < 0 {
		idx = 0
	}
	return time.Duration(snap[idx])
}

// Print prints a summary.
func (lr *LoadRunner) Print(t *testing.T) {
	total := atomic.LoadInt64(&lr.total)
	errors := atomic.LoadInt64(&lr.errors)
	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errors) / float64(total) * 100
	}
	t.Logf("[%s] total=%d errors=%d (%.1f%%) p50=%v p95=%v p99=%v",
		lr.Name, total, errors, errorRate,
		lr.Percentile(50),
		lr.Percentile(95),
		lr.Percentile(99),
	)
}

// ---- Tests ----

// TestForgeLoad runs a comprehensive load test suite against the Forge server.
// Requires a running Forge instance at the --endpoint flag.
func TestForgeLoad(t *testing.T) {
	flag.Parse()
	base := *endpoint
	dur := *duration
	rpsTarget := *rps

	ctx, cancel := context.WithTimeout(context.Background(), dur+5*time.Second)
	defer cancel()

	// 1. Health check — baseline
	healthLR := &LoadRunner{
		Name:   "GET /health",
		Method: "GET",
		URL:    base + "/health",
	}

	// 2. DB writes
	docBody, _ := json.Marshal(map[string]interface{}{
		"load_test": true,
		"ts":        time.Now().Unix(),
		"value":     42,
	})
	dbWriteLR := &LoadRunner{
		Name:        "POST /db/load_test",
		Method:      "POST",
		URL:         base + "/db/load_test",
		Body:        docBody,
		ContentType: "application/json",
	}

	// 3. DB reads
	dbReadLR := &LoadRunner{
		Name:   "GET /db/load_test",
		Method: "GET",
		URL:    base + "/db/load_test",
	}

	runners := []*LoadRunner{healthLR, dbWriteLR, dbReadLR}

	var wg sync.WaitGroup
	for _, lr := range runners {
		wg.Add(1)
		go func(r *LoadRunner) {
			defer wg.Done()
			r.Run(ctx, rpsTarget, dur)
		}(lr)
	}
	wg.Wait()

	t.Log("=== Forge Load Test Results ===")
	for _, lr := range runners {
		lr.Print(t)
	}

	// Fail if any runner had >5% error rate
	for _, lr := range runners {
		total := atomic.LoadInt64(&lr.total)
		errors := atomic.LoadInt64(&lr.errors)
		if total > 0 && float64(errors)/float64(total) > 0.05 {
			t.Errorf("%s: error rate %.1f%% exceeds 5%% threshold",
				lr.Name, float64(errors)/float64(total)*100)
		}
	}
}

// BenchmarkHealthEndpoint runs a Go benchmark for the /health endpoint.
func BenchmarkHealthEndpoint(b *testing.B) {
	flag.Parse()
	client := &http.Client{Timeout: 2 * time.Second}
	url := *endpoint + "/health"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(url)
			if err != nil {
				b.Error(err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

// BenchmarkDBWrite runs a Go benchmark for document writes.
func BenchmarkDBWrite(b *testing.B) {
	flag.Parse()
	client := &http.Client{Timeout: 2 * time.Second}
	url := *endpoint + "/db/bench_writes"
	body, _ := json.Marshal(map[string]interface{}{"bench": true, "n": 1})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Post(url, "application/json", bytes.NewReader(body))
			if err != nil {
				b.Error(err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	})
}

// summary prints a single-line summary for reporting.
func summary(name string, total, errors int64, p50, p95, p99 time.Duration) string {
	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errors) / float64(total) * 100
	}
	return fmt.Sprintf("[%s] total=%d err=%.1f%% p50=%v p95=%v p99=%v",
		name, total, errorRate, p50, p95, p99)
}

var _ = summary // suppress unused warning
