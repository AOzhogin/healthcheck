package healthcheck

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestTLSMetrics_NilReceiverIsSafe(t *testing.T) {
	var m *tlsMetrics

	require.NotPanics(t, func() {
		m.certLoaded(time.Now())
		m.reloadSucceeded()
		m.reloadFailed()
	})
}

func TestTLSMetrics_ExposesExpiryAndReloads(t *testing.T) {
	r := prometheus.NewRegistry()

	m, err := newTLSMetrics(r)
	require.NoError(t, err)

	notAfter := time.Unix(1793404800, 0)
	m.certLoaded(notAfter)
	m.reloadSucceeded()
	m.reloadFailed()
	m.reloadFailed()

	expected := `
# HELP healthcheck_tls_cert_expiry_timestamp_seconds Unix timestamp of the NotAfter date of the served certificate
# TYPE healthcheck_tls_cert_expiry_timestamp_seconds gauge
healthcheck_tls_cert_expiry_timestamp_seconds 1.7934048e+09
# HELP healthcheck_tls_cert_reload_total Total certificate reloads triggered by a change on disk
# TYPE healthcheck_tls_cert_reload_total counter
healthcheck_tls_cert_reload_total{status="error"} 2
healthcheck_tls_cert_reload_total{status="ok"} 1
`

	require.NoError(t, testutil.GatherAndCompare(r, strings.NewReader(expected),
		"healthcheck_tls_cert_expiry_timestamp_seconds", "healthcheck_tls_cert_reload_total"))
}

// Both label values must exist from the start so an alert on the error rate has a baseline before
// the first rotation ever happens.
func TestTLSMetrics_ReloadSeriesArePreCreated(t *testing.T) {
	r := prometheus.NewRegistry()

	_, err := newTLSMetrics(r)
	require.NoError(t, err)

	expected := `
# HELP healthcheck_tls_cert_reload_total Total certificate reloads triggered by a change on disk
# TYPE healthcheck_tls_cert_reload_total counter
healthcheck_tls_cert_reload_total{status="error"} 0
healthcheck_tls_cert_reload_total{status="ok"} 0
`

	require.NoError(t, testutil.GatherAndCompare(r, strings.NewReader(expected), "healthcheck_tls_cert_reload_total"))
}

// StartHTTPServer may run twice against the same registry (a restart in-process); reusing the
// already registered collectors must not fail.
func TestTLSMetrics_ReuseOnAlreadyRegistered(t *testing.T) {
	r := prometheus.NewRegistry()

	first, err := newTLSMetrics(r)
	require.NoError(t, err)

	second, err := newTLSMetrics(r)
	require.NoError(t, err)

	first.reloadSucceeded()
	second.reloadSucceeded()

	require.Equal(t, 2.0, testutil.ToFloat64(second.reloads.WithLabelValues(metricStatusOK)),
		"the second instance must update the collectors that are actually registered")
}

func TestTLSMetrics_WiredIntoHealthCheckRegistry(t *testing.T) {
	r := prometheus.NewRegistry()
	h := New(WithMetricsRegistry(r))

	require.Same(t, r, h.promRegistry())
}

func TestPromRegistry_NilWhenMetricsDisabled(t *testing.T) {
	require.Nil(t, New().promRegistry())
}
