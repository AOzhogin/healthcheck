package healthcheck

import (
	"context"
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

// WithMetricsRegistry - collect prometheus metrics with external registry
func WithMetricsRegistry(r *prometheus.Registry) HCOption {
	return func(check *healthCheck) {
		check.Metrics = NewMetricsWithRegistry(r)
	}
}

// WithMetrics - collect prometheus metrics
func WithMetrics(buildInfo, goCollector, processCollector bool) HCOption {
	return func(check *healthCheck) {
		check.Metrics = NewMetrics(buildInfo, goCollector, processCollector)
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

// WithPort - set port for HTTP server
func WithPort(port string) HCOption {
	return func(check *healthCheck) {
		check.port = port
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
