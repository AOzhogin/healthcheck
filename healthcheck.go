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
	HandlerReady       = "/read"
	HandlerStartup     = "/startup"
)

type HealthCheck interface {
	Start()
	Shutdown()

	HandlerHealth(w http.ResponseWriter, r *http.Request)
	HandlerMetrics(w http.ResponseWriter, r *http.Request)

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
	isWorked           bool
	wg                 sync.WaitGroup
	Metrics
}

func New(ops ...HCOption) HealthCheck {

	h := &healthCheck{
		checks:             newCheckList(),
		statusCodeSuccess:  http.StatusOK,
		statusCodeError:    http.StatusServiceUnavailable,
		timeOut:            30 * time.Second,
		routine:            false,
		checkStatusSuccess: checkStatusSuccess,
		checkStatusError:   checkStatusError,
		cacheMutex:         sync.Mutex{},
		isWorked:           true,
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

	var err error

	h.checks.Lock()
	defer h.checks.Unlock()

	if h.withMetrics() {
		err = h.Metrics.Register(name)
		if err != nil {
			return err
		}
	}

	if _, ok := h.checks.List[name]; ok {
		return fmt.Errorf("same checker with name %s already exists", name)
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
			var cache checkResults
			for {
				cache = h.check()
				h.cacheMutex.Lock()
				h.cache = cache
				h.cacheMutex.Unlock()
				<-time.After(h.routineInterval)
				if !h.isWorked {
					return
				}
			}
		}()

	}
}

// Shutdown - shutdown background check thread
func (h *healthCheck) Shutdown() {

	if h.routine {
		h.isWorked = false
		h.wg.Wait()
	}

}

// check - main proc with process all checks
func (h *healthCheck) check() checkResults {

	h.checks.Lock()
	defer h.checks.Unlock()

	ctx, done := context.WithTimeout(context.Background(), h.timeOut)
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
			h.Metrics.Save(name, execTime.Seconds(), err)
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
