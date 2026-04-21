package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// ExportPrometheus writes all metrics in Prometheus text exposition format
// (https://prometheus.io/docs/instrumenting/exposition_formats/) to w.
//
// No dependencies — hand-written plain text format.
// Suitable for scraping with prometheus or victoria-metrics.
func ExportPrometheus(r *Registry, w io.Writer) error {
	snap := r.Snapshot()

	// Sort keys for deterministic output
	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := snap[key]
		promName := prometheusName(key)

		// Comment / TYPE line
		fmt.Fprintf(w, "# HELP %s Forge metric\n", promName)
		fmt.Fprintf(w, "# TYPE %s gauge\n", promName)
		fmt.Fprintf(w, "%s %v\n", promName, val)
	}

	return nil
}

// prometheusName converts a dot-separated Forge metric name to
// the underscore-separated Prometheus convention.
func prometheusName(name string) string {
	s := strings.ReplaceAll(name, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return "forge_" + s
}

// ExportJSON writes all metrics as a flat JSON object.
func ExportJSON(r *Registry, w io.Writer) error {
	snap := r.Snapshot()

	keys := make([]string, 0, len(snap))
	for k := range snap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprint(w, "{\n")
	for i, key := range keys {
		comma := ","
		if i == len(keys)-1 {
			comma = ""
		}
		fmt.Fprintf(w, "  %q: %v%s\n", key, snap[key], comma)
	}
	fmt.Fprint(w, "}\n")
	return nil
}
