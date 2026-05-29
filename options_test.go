package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestWithHTTPAddress(t *testing.T) {
	h := &healthCheck{httpAddr: ":8080"}
	WithHTTPAddress(":9090")(h)
	if h.httpAddr != ":9090" {
		t.Errorf("expected httpAddr :9090, got %s", h.httpAddr)
	}
}

func TestWithoutPProf(t *testing.T) {
	h := &healthCheck{pprofEnabled: true}
	WithoutPProf()(h)
	if h.pprofEnabled {
		t.Error("expected pprofEnabled to be false after WithoutPProf")
	}
}

func TestWithSuccessStatus(t *testing.T) {
	h := &healthCheck{statusCodeSuccess: http.StatusOK}
	WithSuccessStatus(201)(h)
	if h.statusCodeSuccess != 201 {
		t.Errorf("expected status 201, got %d", h.statusCodeSuccess)
	}
}

func TestWithErrorStatus(t *testing.T) {
	h := &healthCheck{statusCodeError: http.StatusServiceUnavailable}
	WithErrorStatus(500)(h)
	if h.statusCodeError != 500 {
		t.Errorf("expected status 500, got %d", h.statusCodeError)
	}
}

func TestWithTimeOut(t *testing.T) {
	h := &healthCheck{timeOut: 30 * time.Second}
	WithTimeOut(10 * time.Second)(h)
	if h.timeOut != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", h.timeOut)
	}
}

func TestWithMetrics(t *testing.T) {
	h := &healthCheck{}
	WithMetrics(true, true, true)(h)
	if h.Metrics == nil {
		t.Error("expected Metrics to be set")
	}
}

func TestWithMetricsRegistry(t *testing.T) {
	h := &healthCheck{}
	r := prometheus.NewRegistry()
	WithMetricsRegistry(r)(h)
	if h.Metrics == nil {
		t.Error("expected Metrics to be set with registry")
	}
}

func TestWithBackCheck(t *testing.T) {
	h := &healthCheck{}
	WithBackCheck(5 * time.Second)(h)
	if !h.routine {
		t.Error("expected routine to be true")
	}
	if h.routineInterval != 5*time.Second {
		t.Errorf("expected interval 5s, got %v", h.routineInterval)
	}
}

func TestWithCheckStatusSuccess(t *testing.T) {
	h := &healthCheck{checkStatusSuccess: "ok"}
	WithCheckStatusSuccess("good")(h)
	if h.checkStatusSuccess != "good" {
		t.Errorf("expected checkStatusSuccess 'good', got '%s'", h.checkStatusSuccess)
	}
}

func TestWithCheckStatusError(t *testing.T) {
	h := &healthCheck{checkStatusError: "error"}
	WithCheckStatusError("fail")(h)
	if h.checkStatusError != "fail" {
		t.Errorf("expected checkStatusError 'fail', got '%s'", h.checkStatusError)
	}
}

func TestWithBasicAuth(t *testing.T) {
	h := &healthCheck{}
	WithBasicAuth("user", "pass")(h)
	if h.basicAuthUser != "user" || h.basicAuthPass != "pass" {
		t.Errorf("expected basicAuth user=user pass=pass, got user=%q pass=%q", h.basicAuthUser, h.basicAuthPass)
	}
	WithBasicAuth("", "")(h)
	if h.basicAuthUser != "" {
		t.Error("empty username should disable Basic Auth")
	}
}

func TestWithMiddleware(t *testing.T) {
	h := &healthCheck{}
	called := false
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	}
	WithMiddleware(mw)(h)
	if h.customMiddleware == nil {
		t.Fatal("expected customMiddleware to be set")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.wrapHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(rec, req)
	if !called {
		t.Error("expected custom middleware to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestStartHTTPServer(t *testing.T) {
	h := New() // use constructor for proper initialization
	ts := httptest.NewServer(http.HandlerFunc(h.HandlerHealth))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("failed to GET health endpoint: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("unexpected status code: got %d", resp.StatusCode)
	}
}
