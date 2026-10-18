package main

import (
	"net/http"

	"github.com/lxsmnsyc/wathena/server/retention"
)

// prometheusHandler serves the retention job's metrics in the
// Prometheus text format, at /debug/wathena/metrics/prometheus on the
// debug address (decision 0049).
func prometheusHandler(j *retention.Job) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = j.WritePrometheus(w)
	}
}
