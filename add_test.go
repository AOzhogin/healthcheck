package healthcheck

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestAdd_DuplicateNameWithoutMetrics(t *testing.T) {
	h := New()
	if err := h.Add("x", "", okCheck); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := h.Add("x", "", okCheck); err == nil {
		t.Error("duplicate Add should return an error")
	}
}

func TestAdd_DuplicateNameWithMetrics(t *testing.T) {
	h := New(WithMetrics(false, false, false))
	if err := h.Add("db", "", okCheck); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	// With metrics, the duplicate is caught at collector registration.
	if err := h.Add("db", "", okCheck); err == nil {
		t.Error("duplicate Add with metrics should return an error")
	}
}

func TestCheck_RecordsMetrics(t *testing.T) {
	h := New(WithMetrics(false, false, false))
	if err := h.Add("db", "db.company", okCheck); err != nil {
		t.Fatal(err)
	}

	// Non-routine HandlerHealth runs check() synchronously, which calls Save.
	rec := httptest.NewRecorder()
	h.HandlerHealth(rec, httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))

	m := h.Metrics.(*metrics)
	gauge := m.mm["db"].(prometheus.Gauge)
	if v := testutil.ToFloat64(gauge); v != 1 {
		t.Errorf("gauge after successful check: got %v, want 1", v)
	}
}

func TestNewErrorResult(t *testing.T) {
	e := newErrorResult(errors.New("boom"))
	if e.Status != "error" {
		t.Errorf("status: got %q, want %q", e.Status, "error")
	}
	if e.Error != "boom" {
		t.Errorf("error: got %q, want %q", e.Error, "boom")
	}
}
