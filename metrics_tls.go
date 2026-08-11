package healthcheck

import "time"

// tlsMetrics holds the certificate lifecycle collectors. Every method is nil-safe so the reloader
// runs unchanged when metrics are disabled.
type tlsMetrics struct{}

func (m *tlsMetrics) certLoaded(time.Time) {
	if m == nil {
		return
	}
}
