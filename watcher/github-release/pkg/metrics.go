// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import "github.com/prometheus/client_golang/prometheus"

//counterfeiter:generate -o mocks/metrics.go --fake-name Metrics . Metrics

// Metrics is the four observable counters required by [[Watcher Writing Guide]] §
// Required observability.
type Metrics interface {
	// IncPollCycle — result: "success" | "rate_limited" | "github_error"
	IncPollCycle(result string)

	// IncPublished — status: "create" | "skipped" | "error"
	IncPublished(status string)

	// IncReposScanned — increment by N repos scanned in the cycle (cardinality: none).
	IncReposScanned(n int)

	// IncFilterSkipped — reason: "empty_unreleased" | "auto_release" | "sha_unchanged" | "scope"
	IncFilterSkipped(reason string)
}

const metricNamespace = "github_release_watcher"

var (
	pollCycleTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricNamespace + "_poll_cycle_total",
		Help: "Total poll cycles by result.",
	}, []string{"result"})

	publishedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricNamespace + "_published_total",
		Help: "Total task-publish attempts by status.",
	}, []string{"status"})

	reposScannedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: metricNamespace + "_repos_scanned_total",
		Help: "Total number of repos scanned across all poll cycles.",
	})

	filterSkippedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricNamespace + "_filter_skipped_total",
		Help: "Total releases filtered out by reason.",
	}, []string{"reason"})
)

func init() {
	prometheus.MustRegister(pollCycleTotal, publishedTotal, reposScannedTotal, filterSkippedTotal)
	// Pre-init all label combos to .Add(0) so Prometheus exposes them before first event.
	for _, r := range []string{"success", "rate_limited", "github_error"} {
		pollCycleTotal.WithLabelValues(r).Add(0)
	}
	for _, s := range []string{"create", "skipped", "error"} {
		publishedTotal.WithLabelValues(s).Add(0)
	}
	for _, r := range []string{"empty_unreleased", "auto_release", "sha_unchanged", "scope"} {
		filterSkippedTotal.WithLabelValues(r).Add(0)
	}
}

type prometheusMetrics struct{}

// NewMetrics returns the Prometheus-backed Metrics implementation.
func NewMetrics() Metrics {
	return &prometheusMetrics{}
}

func (m *prometheusMetrics) IncPollCycle(result string) {
	pollCycleTotal.WithLabelValues(result).Inc()
}

func (m *prometheusMetrics) IncPublished(status string) {
	publishedTotal.WithLabelValues(status).Inc()
}

func (m *prometheusMetrics) IncReposScanned(n int) {
	reposScannedTotal.Add(float64(n))
}

func (m *prometheusMetrics) IncFilterSkipped(reason string) {
	filterSkippedTotal.WithLabelValues(reason).Inc()
}
