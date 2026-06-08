package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsNamespace  = "healthcheck"
	metricsSubsystem  = "metrics"
	metricLabelCheck  = "check"
	metricLabelReason = "reason"
	HandlerMetrics    = "/metrics"
)

var (
	// Metrics interface implementation check for struct metrics
	_ Metrics = (*metrics)(nil)
)

// Metrics - Interface for metrics access and mock it
type Metrics interface {
	HandlerMetrics() http.Handler
	Register(name string) error
	Save(name string, t float64, err error) error
}

// metrics - holds the labeled collectors and the registry they live in.
// The check name is a label on the vectors, not part of the metric name.
type metrics struct {
	up          *prometheus.GaugeVec
	dur         *prometheus.HistogramVec
	lastSuccess *prometheus.GaugeVec
	errors      *prometheus.CounterVec

	registry *prometheus.Registry
}

// checkVectors bundles the per-check collectors keyed by the check label.
type checkVectors struct {
	up          *prometheus.GaugeVec
	dur         *prometheus.HistogramVec
	lastSuccess *prometheus.GaugeVec
	errors      *prometheus.CounterVec
}

// newCheckVectors builds the per-check collectors. buckets sets the duration histogram buckets;
// nil/empty uses the Prometheus default buckets.
func newCheckVectors(buckets []float64) checkVectors {
	return checkVectors{
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "up",
			Help:      "Check availability (1 = ok, 0 = error)",
		}, []string{metricLabelCheck}),
		dur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "duration_seconds",
			Help:      "Check execution duration in seconds",
			Buckets:   buckets,
		}, []string{metricLabelCheck}),
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "last_success_timestamp_seconds",
			Help:      "Unix timestamp of the last successful check",
		}, []string{metricLabelCheck}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "errors_total",
			Help:      "Total failed checks by reason",
		}, []string{metricLabelCheck, metricLabelReason}),
	}
}

func newMetricsFromVectors(r *prometheus.Registry, v checkVectors) *metrics {
	r.MustRegister(v.up, v.dur, v.lastSuccess, v.errors)
	return &metrics{
		up:          v.up,
		dur:         v.dur,
		lastSuccess: v.lastSuccess,
		errors:      v.errors,
		registry:    r,
	}
}

func NewMetricsWithRegistry(r *prometheus.Registry, buckets ...float64) *metrics {
	return newMetricsFromVectors(r, newCheckVectors(buckets))
}

func NewMetrics(buildInfo, goCollector, processCollector bool, buckets ...float64) *metrics {

	r := prometheus.NewRegistry()
	m := newMetricsFromVectors(r, newCheckVectors(buckets))

	if buildInfo {
		r.MustRegister(
			collectors.NewBuildInfoCollector(),
		)
	}

	if goCollector {
		r.MustRegister(
			collectors.NewGoCollector(),
		)
	}

	if processCollector {
		r.MustRegister(
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{ReportErrors: true}),
		)
	}

	return m

}

// Register instantiates the metric series for a check so they appear in /metrics
// immediately (gauge at 0, no observations) before the first check runs.
// It is idempotent and never returns an error.
func (m *metrics) Register(name string) error {

	m.up.WithLabelValues(name)
	m.dur.WithLabelValues(name)
	m.lastSuccess.WithLabelValues(name)

	return nil

}

// Save records a check result keyed by the check label: the up gauge is set to 1 on success
// (and the last-success timestamp updated) or 0 on error (and errors_total incremented with a
// classified reason); the execution duration is observed in the histogram. It never returns an error.
func (m *metrics) Save(name string, t float64, err error) error {

	if err != nil {
		m.up.WithLabelValues(name).Set(0)
		m.errors.WithLabelValues(name, classifyReason(err)).Inc()
	} else {
		m.up.WithLabelValues(name).Set(1)
		m.lastSuccess.WithLabelValues(name).Set(float64(time.Now().Unix()))
	}

	m.dur.WithLabelValues(name).Observe(t)

	return nil

}

// classifyReason maps a check error to a low-cardinality reason for the errors_total label.
// A check may control the reason by returning an error that implements interface{ Reason() string };
// otherwise context deadline/cancel are recognised, and everything else is "error".
func classifyReason(err error) string {
	var re interface{ Reason() string }
	if errors.As(err, &re) {
		if r := re.Reason(); r != "" {
			return r
		}
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}

// HandlerMetrics - handler for incoming requests on endpoint like /metrics
func (m *metrics) HandlerMetrics() http.Handler {

	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})

}
