package healthcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// mcpCheckRun is the result of a single check run: the regular checkResult plus the check name.
type mcpCheckRun struct {
	Name string `json:"name"`
	checkResult
}

// mcpStatus returns what /health?body=true would return (without the trailing newline):
// the cached result when the background routine is enabled, a fresh run otherwise.
func (h *healthCheck) mcpStatus(ctx context.Context) json.RawMessage {

	var res checkResults

	if h.routine {
		h.cacheMutex.Lock()
		res = h.cache
		h.cacheMutex.Unlock()
	} else {
		res = h.check(ctx)
	}

	b, err := json.Marshal(res)
	if err != nil {
		b, _ = json.Marshal(newErrorResult(err))
	}

	return b
}

// mcpRunCheck runs one check by name without side effects: it does not touch the cache,
// metrics, transition logging or tracing. A failing check is reported in the result, not as an error.
func (h *healthCheck) mcpRunCheck(ctx context.Context, name string) (json.RawMessage, error) {

	h.checks.Mutex.Lock()
	cc, ok := h.checks.List[name]
	h.checks.Mutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("healthcheck: mcp: unknown check %q", name)
	}

	ctx, done := context.WithTimeout(ctx, h.timeOut)
	defer done()

	start := time.Now()
	err := cc.Func(ctx)
	exec := time.Since(start)

	run := mcpCheckRun{Name: name}
	run.LastAction = time.Now()
	run.Status = h.checkStatusSuccess
	run.Time = exec.Seconds()
	run.Notes = cc.Notes
	if err != nil {
		run.Status = err.Error()
	}

	return json.Marshal(run)
}
