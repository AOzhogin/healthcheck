package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestWithPort(t *testing.T) {
	h := &healthCheck{port: 8080}
	WithPort(9090)(h)
	if h.port != 9090 {
		t.Errorf("expected port 9090, got %d", h.port)
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

func TestStartHTTPServer(t *testing.T) {
	h := New().(*healthCheck) // используем конструктор для правильной инициализации
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
