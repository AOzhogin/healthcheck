package healthcheck

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsNamespace = "healthcheck"
	metricsSubsystem = "metrics"
	metricLabelCheck = "check"
	HandlerMetrics   = "/metrics"
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
	up  *prometheus.GaugeVec
	dur *prometheus.HistogramVec

	registry *prometheus.Registry
}

// newCheckVectors builds the gauge/histogram vectors keyed by the check label.
func newCheckVectors() (*prometheus.GaugeVec, *prometheus.HistogramVec) {
	up := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "up",
		Help:      "Check availability (1 = ok, 0 = error)",
	}, []string{metricLabelCheck})

	dur := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "duration_seconds",
		Help:      "Check execution duration in seconds",
	}, []string{metricLabelCheck})

	return up, dur
}

func NewMetricsWithRegistry(r *prometheus.Registry) *metrics {

	up, dur := newCheckVectors()
	r.MustRegister(up, dur)

	return &metrics{
		up:       up,
		dur:      dur,
		registry: r,
	}

}

func NewMetrics(buildInfo, goCollector, processCollector bool) *metrics {

	r := prometheus.NewRegistry()

	up, dur := newCheckVectors()
	r.MustRegister(up, dur)

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

	return &metrics{
		up:       up,
		dur:      dur,
		registry: r,
	}

}

// Register instantiates the metric series for a check so they appear in /metrics
// immediately (gauge at 0, no observations) before the first check runs.
// It is idempotent and never returns an error.
func (m *metrics) Register(name string) error {

	m.up.WithLabelValues(name)
	m.dur.WithLabelValues(name)

	return nil

}

// Save records a check result: the gauge is set to 1 on success and 0 on error,
// and the execution duration is observed in the histogram, both keyed by the
// check label. It never returns an error.
func (m *metrics) Save(name string, t float64, err error) error {

	if err != nil {
		m.up.WithLabelValues(name).Set(0)
	} else {
		m.up.WithLabelValues(name).Set(1)
	}

	m.dur.WithLabelValues(name).Observe(t)

	return nil

}

// HandlerMetrics - handler for incoming requests on endpoint like /metrics
func (m *metrics) HandlerMetrics() http.Handler {

	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})

}
