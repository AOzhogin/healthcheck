package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
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
	}

	for _, option := range ops {
		option(h)
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
						cache = h.check()
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

// check - main proc with process all checks
func (h *healthCheck) check() checkResults {

	h.checks.Mutex.Lock()
	defer h.checks.Mutex.Unlock()

	ctx, done := context.WithTimeout(h.ctx, h.timeOut)
	defer done()

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

		startDt = time.Now()
		err = value.Func(ctx)
		execTime = time.Since(startDt)

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

			if res.Status != h.checkStatusError {
				res.Status = h.checkStatusError
				res.code = h.statusCodeError
			}

		}

		res.Checks[name] = r

	}

	return res

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
