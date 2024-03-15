package healthcheck

import (
	"encoding/json"
	"net/http"
)

// HandlerHealth - handler with main logic
func (h *healthCheck) HandlerHealth(w http.ResponseWriter, r *http.Request) {

	var res checkResults

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
			var r = resultError{
				Status: "error",
				Error:  err.Error(),
				Checks: any(""),
			}
			if err = json.NewEncoder(w).Encode(r); err != nil {
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
