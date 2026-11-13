package healthcheck

import "net/http"

// withBasicAuth reports whether HTTP Basic Auth is enabled.
func (h *healthCheck) withBasicAuth() bool {
	return h.basicAuthUser != ""
}

// MiddlewareAuth wraps an http.Handler with HTTP Basic Auth when enabled.
func (h *healthCheck) MiddlewareAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.withBasicAuth() {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != h.basicAuthUser || pass != h.basicAuthPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="healthcheck"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MiddlewareProbe makes a probe endpoint status-only: GET/HEAD only, the query string is stripped
// before next sees it, and any body or Content-Type next writes is dropped. Only the status code
// reaches the client, so even a custom SetHandler* handler cannot leak check details.
func (h *healthCheck) MiddlewareProbe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		r = r.Clone(r.Context())
		r.URL.RawQuery = ""
		next.ServeHTTP(statusOnly{w}, r)
	})
}

// statusOnly forwards the status code and discards the body (and the headers describing it).
type statusOnly struct{ http.ResponseWriter }

func (s statusOnly) WriteHeader(code int) {
	s.Header().Del("Content-Type")
	s.Header().Del("Content-Length")
	s.ResponseWriter.WriteHeader(code)
}

func (s statusOnly) Write(b []byte) (int, error) { return len(b), nil }
