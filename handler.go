package healthcheck

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	rtp "runtime/pprof"
	"strings"
)

// HandlerHealth - handler with main logic
func (h *healthCheck) HandlerHealth(w http.ResponseWriter, r *http.Request) {

	var (
		res checkResults
	)

	switch h.routine {
	case true:
		{
			h.cacheMutex.Lock()
			res = h.cache
			h.cacheMutex.Unlock()
		}
	case false:
		res = h.check()
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(res.code)

	if r.URL.Query().Get("body") == "true" {
		if err := json.NewEncoder(w).Encode(res); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			var errRes = newErrorResult(err)
			if err = json.NewEncoder(w).Encode(errRes); err != nil {
				return
			}
		}
		return
	}

}

// HandlerMetrics - handler with simple metrics
func (h *healthCheck) HandlerMetrics(w http.ResponseWriter, r *http.Request) {

	if h.Metrics != nil {
		h.Metrics.HandlerMetrics().ServeHTTP(w, r)
		return
	}

	w.WriteHeader(http.StatusNotImplemented)

}

// HandlerPProf - handler with pprof
func (h *healthCheck) HandlerPProf(w http.ResponseWriter, r *http.Request) {

	for _, v := range rtp.Profiles() {
		ppName := v.Name()
		if strings.HasPrefix(r.URL.Path, "/debug/"+ppName) {
			namedHandler := pprof.Handler(ppName).ServeHTTP
			namedHandler(w, r)
			return
		}
	}
	pprof.Index(w, r)

}
