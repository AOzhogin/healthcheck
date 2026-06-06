package healthcheck

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	checkStatusSuccess = "ok"
	checkStatusError   = "error"
	HandlerHealthCheck = "/health"
	HandlerLive        = "/live"
	HandlerReady       = "/ready"
	HandlerStartup     = "/startup"
	HandlerDebug       = "/debug/"
)

type HealthCheck interface {
	Start()
	Shutdown()

	HandlerHealth(w http.ResponseWriter, r *http.Request)
	HandlerMetrics(w http.ResponseWriter, r *http.Request)
	HandlerPProf(w http.ResponseWriter, r *http.Request)

	SetHandlerLive(handler http.HandlerFunc)
	SetHandlerReady(handler http.HandlerFunc)
	SetHandlerStartup(handler http.HandlerFunc)

	Add(name string, notes string, e HCFunc) error
}

type healthCheck struct {
	cache              checkResults
	cacheMutex         sync.Mutex
	checks             checkList
	statusCodeSuccess  int
	statusCodeError    int
	checkStatusSuccess string
	checkStatusError   string
	timeOut            time.Duration
	routine            bool
	routineInterval    time.Duration
	isWorked           chan struct{}
	wg                 sync.WaitGroup
	ctx                context.Context
	Metrics
	httpAddr string // HTTP listen address for StartHTTPServer, e.g. ":8080"

	basicAuthUser string // non-empty enables HTTP Basic Auth for /health, /metrics, /debug
	basicAuthPass string

	customMiddleware func(http.Handler) http.Handler // optional user middleware applied to all registered endpoints

	pprofEnabled bool // when true, StartHTTPServer registers the pprof (/debug/) endpoint

	// custom handlers for the k8s probe endpoints; nil means use the default (HandlerHealth).
	// Read once at route registration in StartHTTPServer; set them before calling it.
	liveHandler    http.HandlerFunc
	readyHandler   http.HandlerFunc
	startupHandler http.HandlerFunc

	// metrics configuration; the Metrics implementation is built in New() after all options are
	// applied, so WithMetricsBuckets is order-independent with WithMetrics/WithMetricsRegistry.
	metricsEnabled          bool
	metricsBuildInfo        bool
	metricsGoCollector      bool
	metricsProcessCollector bool
	metricsRegistry         *prometheus.Registry
	metricsBuckets          []float64

	logger     *slog.Logger    // optional; when set, logs check state transitions (ok<->error)
	prevFailed map[string]bool // last-known failed state per check, for transition logging

	tracer Tracer // span per check; defaults to a no-op (no tracing dependency in core)
}

func New(ops ...HCOption) *healthCheck {

	h := &healthCheck{
		checks:             newCheckList(),
		statusCodeSuccess:  http.StatusOK,
		statusCodeError:    http.StatusServiceUnavailable,
		timeOut:            30 * time.Second,
		routine:            false,
		checkStatusSuccess: checkStatusSuccess,
		checkStatusError:   checkStatusError,
		cacheMutex:         sync.Mutex{},
		isWorked:           make(chan struct{}, 1),
		ctx:                context.Background(),
		httpAddr:           ":8080",
		pprofEnabled:       true,
		prevFailed:         make(map[string]bool),
		tracer:             nopTracer{},
	}

	for _, option := range ops {
		option(h)
	}

	// Build the metrics implementation after all options are applied so bucket/registry/collector
	// choices are independent of option order.
	if h.metricsEnabled && h.Metrics == nil {
		if h.metricsRegistry != nil {
			h.Metrics = NewMetricsWithRegistry(h.metricsRegistry, h.metricsBuckets...)
		} else {
			h.Metrics = NewMetrics(h.metricsBuildInfo, h.metricsGoCollector, h.metricsProcessCollector, h.metricsBuckets...)
		}
	}

	return h

}

// withMetrics - check
func (h *healthCheck) withMetrics() bool {
	return h.Metrics != nil
}

// Add - add new check with name and notes
func (h *healthCheck) Add(name string, notes string, e HCFunc) error {

	h.checks.Mutex.Lock()
	defer h.checks.Mutex.Unlock()

	if _, ok := h.checks.List[name]; ok {
		return fmt.Errorf("same checker with name %s already exists", name)
	}

	if h.withMetrics() {
		if err := h.Register(name); err != nil {
			return err
		}
	}

	h.checks.List[name] = CheckContext{Func: e, Notes: notes}

	return nil

}

// Start - start background thread with checks
func (h *healthCheck) Start() {

	if h.routine {
		h.wg.Add(1)

		go func() {
			defer h.wg.Done()

			var (
				cache checkResults
			)

			for {
				select {
				case <-h.ctx.Done():
					{
						return
					}
				case <-time.After(h.routineInterval):
					{
						cache = h.check(h.ctx)
						h.cacheMutex.Lock()
						h.cache = cache
						h.cacheMutex.Unlock()
					}
				case <-h.isWorked:
					{
						return
					}
				}
			}
		}()

	}
}

// Shutdown - shutdown background check thread
func (h *healthCheck) Shutdown() {

	if h.routine {
		h.isWorked <- struct{}{}
		h.wg.Wait()
	}

}

// check - run all checks. base seeds the timeout context and carries trace context: the request
// context for a synchronous /health, or h.ctx for the background routine.
func (h *healthCheck) check(base context.Context) checkResults {

	h.checks.Mutex.Lock()
	defer h.checks.Mutex.Unlock()

	ctx, done := context.WithTimeout(base, h.timeOut)
	defer done()

	ctx, runSpan := h.tracer.Start(ctx, "healthcheck.run")
	defer runSpan.End()

	var (
		err      error
		res      checkResults
		r        checkResult
		startDt  time.Time
		execTime time.Duration
	)

	res.Status = h.checkStatusSuccess
	res.code = h.statusCodeSuccess
	res.Checks = make(map[string]checkResult)

	for name, value := range h.checks.List {

		cctx, span := h.tracer.Start(ctx, "healthcheck/"+name)
		span.SetAttr("healthcheck.check", name)
		if value.Notes != "" {
			span.SetAttr("healthcheck.notes", value.Notes)
		}

		startDt = time.Now()
		err = value.Func(cctx)
		execTime = time.Since(startDt)

		span.SetAttr("healthcheck.duration_ms", execTime.Milliseconds())

		r.LastAction = time.Now()
		r.Status = h.checkStatusSuccess
		r.Time = execTime.Seconds()
		r.Notes = value.Notes
		r.Result = ""

		if h.withMetrics() {
			if err := h.Save(name, execTime.Seconds(), err); err != nil {
				fmt.Printf("error saving metric: %v \n", err)
			}
		}

		if err != nil {

			r.Status = err.Error()
			span.RecordError(err)
			span.SetAttr("healthcheck.status", "error")

			if res.Status != h.checkStatusError {
				res.Status = h.checkStatusError
				res.code = h.statusCodeError
			}

		} else {
			span.SetAttr("healthcheck.status", "ok")
		}

		span.End()

		h.logTransition(cctx, name, err, execTime)

		res.Checks[name] = r

	}

	runSpan.SetAttr("healthcheck.status", res.Status)

	return res

}

// logTransition logs a check's state change (ok<->error) when a logger is configured.
// Baseline is "ok": a check that starts failing logs an error; recovery logs at info.
// Called within check() under checks.Mutex, so prevFailed access is safe.
func (h *healthCheck) logTransition(ctx context.Context, name string, err error, dur time.Duration) {

	if h.logger == nil {
		return
	}

	failed := err != nil
	if h.prevFailed[name] == failed {
		return
	}
	h.prevFailed[name] = failed

	if failed {
		h.logger.LogAttrs(ctx, slog.LevelError, "healthcheck: check failed",
			slog.String("check", name),
			slog.String("error", err.Error()),
			slog.Duration("duration", dur),
		)
		return
	}

	h.logger.LogAttrs(ctx, slog.LevelInfo, "healthcheck: check recovered",
		slog.String("check", name),
		slog.Duration("duration", dur),
	)
}

// SetHandlerLive sets a custom handler for the /live (liveness) endpoint.
// Pass nil to fall back to the default handler (runs the checks, like /health).
// Read once at route registration; call before StartHTTPServer.
func (h *healthCheck) SetHandlerLive(handler http.HandlerFunc) {
	h.liveHandler = handler
}

// SetHandlerReady sets a custom handler for the /ready (readiness) endpoint.
// Pass nil to fall back to the default handler (runs the checks, like /health).
// Read once at route registration; call before StartHTTPServer.
func (h *healthCheck) SetHandlerReady(handler http.HandlerFunc) {
	h.readyHandler = handler
}

// SetHandlerStartup sets a custom handler for the /startup endpoint.
// Pass nil to fall back to the default handler (runs the checks, like /health).
// Read once at route registration; call before StartHTTPServer.
func (h *healthCheck) SetHandlerStartup(handler http.HandlerFunc) {
	h.startupHandler = handler
}

// resolveHandler returns custom if non-nil, otherwise the default probe handler (HandlerHealth).
func (h *healthCheck) resolveHandler(custom http.HandlerFunc) http.Handler {
	if custom != nil {
		return custom
	}
	return http.HandlerFunc(h.HandlerHealth)
}

// wrapHandler applies custom middleware (if set) and then Basic Auth (if enabled) to next.
func (h *healthCheck) wrapHandler(next http.Handler) http.Handler {
	if h.customMiddleware != nil {
		next = h.customMiddleware(next)
	}
	if h.withBasicAuth() {
		next = h.MiddlewareAuth(next)
	}
	return next
}

// StartHTTPServer starts the HTTP server on the address set via WithHTTPAddress (default ":8080").
// Registers /health, the k8s probes /live, /ready, /startup, /metrics (if metrics enabled),
// and /debug/ (pprof, unless disabled via WithoutPProf). The probe endpoints use the custom
// handlers set via SetHandlerLive/SetHandlerReady/SetHandlerStartup, or default to running the
// checks (like /health) when no custom handler is set.
// When WithBasicAuth is set, these endpoints require HTTP Basic Auth.
// When WithMiddleware is set, the custom middleware is applied first, then Basic Auth (if enabled).
func (h *healthCheck) StartHTTPServer() error {
	mux := http.NewServeMux()
	mux.Handle(HandlerHealthCheck, h.wrapHandler(http.HandlerFunc(h.HandlerHealth)))
	mux.Handle(HandlerLive, h.wrapHandler(h.resolveHandler(h.liveHandler)))
	mux.Handle(HandlerReady, h.wrapHandler(h.resolveHandler(h.readyHandler)))
	mux.Handle(HandlerStartup, h.wrapHandler(h.resolveHandler(h.startupHandler)))
	if h.Metrics != nil {
		mux.Handle(HandlerMetrics, h.wrapHandler(h.Metrics.HandlerMetrics()))
	}
	if h.pprofEnabled {
		mux.Handle(HandlerDebug, h.wrapHandler(http.HandlerFunc(h.HandlerPProf)))
	}
	return http.ListenAndServe(h.httpAddr, mux)
}
