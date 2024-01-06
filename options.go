package healthcheck

import (
	"net/http"
	"sync"
	"time"
)

// HCOption - option type for configuration health check
type HCOption func(check *healthCheck)

// WithSuccessStatus - set success status code
func WithSuccessStatus(status int) HCOption {
	return func(check *healthCheck) {
		check.statusCodeSuccess = status
	}
}

// WithErrorStatus - set status code when any one checkers is failed
func WithErrorStatus(status int) HCOption {
	return func(check *healthCheck) {
		check.statusCodeError = status
	}
}

// WithTimeOut - set global checkers time out
func WithTimeOut(timeout time.Duration) HCOption {
	return func(check *healthCheck) {
		check.timeOut = timeout
	}
}

// WithMetrics - collect prometheus metrics
func WithMetrics(buildInfo, goCollector, processCollector bool) HCOption {
	return func(check *healthCheck) {
		check.Metrics = NewMetrics(buildInfo, goCollector, processCollector)
	}
}

// WithBackCheck - check in routine
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
