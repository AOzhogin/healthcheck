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
