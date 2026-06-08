package healthcheck

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HCOption - option type for configuration health check
type HCOption func(check *healthCheck)

// WithSuccessStatus - set success status code
func WithSuccessStatus(status int) HCOption {
	return func(check *healthCheck) {
		check.statusCodeSuccess = status
	}
}

// WithErrorStatus - set status code when any one checker fails
func WithErrorStatus(status int) HCOption {
	return func(check *healthCheck) {
		check.statusCodeError = status
	}
}

// WithTimeOut - set global checkers timeout
func WithTimeOut(timeout time.Duration) HCOption {
	return func(check *healthCheck) {
		check.timeOut = timeout
	}
}

// WithMetricsRegistry - collect prometheus metrics with an external registry.
// The metrics implementation is built in New() after all options are applied.
func WithMetricsRegistry(r *prometheus.Registry) HCOption {
	return func(check *healthCheck) {
		check.metricsEnabled = true
		check.metricsRegistry = r
	}
}

// WithMetrics - collect prometheus metrics.
// The metrics implementation is built in New() after all options are applied.
func WithMetrics(buildInfo, goCollector, processCollector bool) HCOption {
	return func(check *healthCheck) {
		check.metricsEnabled = true
		check.metricsBuildInfo = buildInfo
		check.metricsGoCollector = goCollector
		check.metricsProcessCollector = processCollector
	}
}

// WithMetricsBuckets sets custom buckets for the check duration histogram
// (healthcheck_metrics_duration_seconds). Empty uses the Prometheus default buckets.
// Effective only together with WithMetrics/WithMetricsRegistry; order-independent.
func WithMetricsBuckets(buckets ...float64) HCOption {
	return func(check *healthCheck) {
		check.metricsBuckets = buckets
	}
}

// WithLogger enables structured logging of check state transitions (ok<->error) via the given
// *slog.Logger: an error-level line when a check starts failing, info-level when it recovers,
// with check name, error and duration attributes. No logging when nil (the default).
func WithLogger(logger *slog.Logger) HCOption {
	return func(check *healthCheck) {
		check.logger = logger
	}
}

// WithTracer enables a span per check via the given Tracer (e.g. the otelhc adapter). The span
// context is passed to each check func so user code can nest its own spans. nil keeps the default
// no-op tracer.
func WithTracer(t Tracer) HCOption {
	return func(check *healthCheck) {
		if t != nil {
			check.tracer = t
		}
	}
}

// WithBackCheck - run checks in a background routine
func WithBackCheck(interval time.Duration) HCOption {
	return func(check *healthCheck) {
		check.routine = true
		check.routineInterval = interval
		check.cache = checkResults{
			code:   http.StatusProcessing,
			Status: checkStatusError,
		}
		check.wg = sync.WaitGroup{}
	}
}

// WithContext - set self context
func WithContext(ctx context.Context) HCOption {
	return func(check *healthCheck) {
		check.ctx = ctx
	}
}

// WithCheckStatusSuccess - set string status when check is success, default "ok"
func WithCheckStatusSuccess(status string) HCOption {
	return func(check *healthCheck) {
		check.checkStatusSuccess = status
	}
}

// WithCheckStatusError - set string status when check is error, default "error"
func WithCheckStatusError(status string) HCOption {
	return func(check *healthCheck) {
		check.checkStatusError = status
	}
}

// WithHTTPAddress sets the HTTP listen address used by StartHTTPServer, e.g. ":8080".
// It is a full net address (host:port), not just a port number.
func WithHTTPAddress(addr string) HCOption {
	return func(check *healthCheck) {
		check.httpAddr = addr
	}
}

// WithoutPProf disables the pprof (/debug/) endpoint in StartHTTPServer.
// pprof is enabled by default; the HandlerPProf method remains available for manual wiring.
func WithoutPProf() HCOption {
	return func(check *healthCheck) {
		check.pprofEnabled = false
	}
}

// WithBasicAuth enables HTTP Basic Auth for /health, /metrics, and /debug endpoints.
// If username is empty, Basic Auth is disabled.
func WithBasicAuth(username, password string) HCOption {
	return func(check *healthCheck) {
		check.basicAuthUser = username
		check.basicAuthPass = password
	}
}

// WithMiddleware sets a custom middleware applied to /health, /metrics, and /debug in StartHTTPServer.
// The middleware runs before Basic Auth (if enabled). Pass nil to disable.
func WithMiddleware(mw func(http.Handler) http.Handler) HCOption {
	return func(check *healthCheck) {
		check.customMiddleware = mw
	}
}
