package healthcheck

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// histogramCount returns the observation count of the duration histogram series for a given check label.
func histogramCount(t *testing.T, m *metrics, check string) uint64 {
	t.Helper()
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range families {
		if mf.GetName() != "healthcheck_metrics_duration_seconds" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == metricLabelCheck && lp.GetValue() == check {
					return metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

func TestMetrics_RegisterIdempotent(t *testing.T) {
	m := NewMetrics(false, false, false)

	if err := m.Register("db"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Registering the same check again must not error (no per-name collector anymore).
	if err := m.Register("db"); err != nil {
		t.Fatalf("second Register must be idempotent: %v", err)
	}
	// Series exists at 0 immediately after Register, before any check runs.
	if v := testutil.ToFloat64(m.up.WithLabelValues("db")); v != 0 {
		t.Errorf("gauge after Register: got %v, want 0", v)
	}
}

func TestMetrics_Save_GaugeValue(t *testing.T) {
	m := NewMetrics(false, false, false)

	if err := m.Save("db", 0.1, nil); err != nil {
		t.Fatalf("Save(nil err): %v", err)
	}
	if v := testutil.ToFloat64(m.up.WithLabelValues("db")); v != 1 {
		t.Errorf("gauge after success: got %v, want 1", v)
	}

	if err := m.Save("db", 0.2, errors.New("boom")); err != nil {
		t.Fatalf("Save(err): %v", err)
	}
	if v := testutil.ToFloat64(m.up.WithLabelValues("db")); v != 0 {
		t.Errorf("gauge after error: got %v, want 0", v)
	}
}

func TestMetrics_Save_ObservesHistogram(t *testing.T) {
	m := NewMetrics(false, false, false)

	_ = m.Save("db", 0.1, nil)
	_ = m.Save("db", 0.2, nil)

	if got := histogramCount(t, m, "db"); got != 2 {
		t.Errorf("histogram sample count for check=db: got %d, want 2", got)
	}
}

func TestMetrics_CheckNameIsLabel(t *testing.T) {
	m := NewMetrics(false, false, false)

	_ = m.Save("db", 0.1, nil)
	_ = m.Save("redis", 0.2, errors.New("down"))

	const expected = `
# HELP healthcheck_metrics_up Check availability (1 = ok, 0 = error)
# TYPE healthcheck_metrics_up gauge
healthcheck_metrics_up{check="db"} 1
healthcheck_metrics_up{check="redis"} 0
`
	if err := testutil.CollectAndCompare(m.up, strings.NewReader(expected), "healthcheck_metrics_up"); err != nil {
		t.Errorf("unexpected gauge output: %v", err)
	}
}

func TestMetrics_HandlerOutputHasCheckLabel(t *testing.T) {
	h := New(WithMetrics(false, false, false))
	if err := h.Add("db", "db.company", okCheck); err != nil {
		t.Fatal(err)
	}
	// Run the checks so the gauge gets a value.
	h.HandlerHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))

	rec := httptest.NewRecorder()
	h.HandlerMetrics(rec, httptest.NewRequest(http.MethodGet, HandlerMetrics, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status: got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `healthcheck_metrics_up{check="db"} 1`) {
		t.Errorf("/metrics missing labeled gauge; body:\n%s", body)
	}
	if !strings.Contains(body, `healthcheck_metrics_duration_seconds_bucket{check="db"`) {
		t.Errorf("/metrics missing labeled histogram; body:\n%s", body)
	}
}
