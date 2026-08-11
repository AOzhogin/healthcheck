package healthcheck

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsSubsystemTLS = "tls"
	metricLabelStatus   = "status"
	metricStatusOK      = "ok"
	metricStatusError   = "error"
)

// tlsMetrics holds the certificate lifecycle collectors. Every method is nil-safe so the reloader
// runs unchanged when metrics are disabled.
type tlsMetrics struct {
	expiry     prometheus.Gauge
	reloads    *prometheus.CounterVec
	lastReload prometheus.Gauge
}

// newTLSMetrics registers the certificate collectors on the registry the library already exposes at
// /metrics. Reusing a registry that already has them is not an error: StartHTTPServer may run more
// than once in the same process.
func newTLSMetrics(r *prometheus.Registry) (*tlsMetrics, error) {
	expiry, err := registerOrReuse(r, prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystemTLS,
		Name:      "cert_expiry_timestamp_seconds",
		Help:      "Unix timestamp of the NotAfter date of the served certificate",
	}))
	if err != nil {
		return nil, err
	}

	reloads, err := registerOrReuse(r, prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystemTLS,
		Name:      "cert_reload_total",
		Help:      "Total certificate reloads triggered by a change on disk",
	}, []string{metricLabelStatus}))
	if err != nil {
		return nil, err
	}

	lastReload, err := registerOrReuse(r, prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystemTLS,
		Name:      "cert_reload_timestamp_seconds",
		Help:      "Unix timestamp of the last successful certificate reload",
	}))
	if err != nil {
		return nil, err
	}

	// Pre-create both series so an alert on the error rate has a baseline before the first rotation.
	reloads.WithLabelValues(metricStatusOK)
	reloads.WithLabelValues(metricStatusError)

	return &tlsMetrics{expiry: expiry, reloads: reloads, lastReload: lastReload}, nil
}

// registerOrReuse registers c, or returns the equivalent collector already present in the registry.
func registerOrReuse[T prometheus.Collector](r *prometheus.Registry, c T) (T, error) {
	if err := r.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(T); ok {
				return existing, nil
			}
		}

		var zero T

		return zero, err
	}

	return c, nil
}

// certLoaded records the expiry of the keypair now being served, including the initial load.
func (m *tlsMetrics) certLoaded(notAfter time.Time) {
	if m == nil {
		return
	}
	m.expiry.Set(float64(notAfter.Unix()))
}

// reloadSucceeded counts a rotation that was picked up successfully at runtime.
func (m *tlsMetrics) reloadSucceeded() {
	if m == nil {
		return
	}
	m.reloads.WithLabelValues(metricStatusOK).Inc()
	m.lastReload.Set(float64(time.Now().Unix()))
}

// reloadFailed counts a rotation that could not be applied; the previous keypair is still served.
func (m *tlsMetrics) reloadFailed() {
	if m == nil {
		return
	}
	m.reloads.WithLabelValues(metricStatusError).Inc()
}
