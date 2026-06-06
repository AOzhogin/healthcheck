package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_LastSuccess(t *testing.T) {
	m := NewMetrics(false, false, false)
	if err := m.Register("db"); err != nil {
		t.Fatal(err)
	}
	if v := testutil.ToFloat64(m.lastSuccess.WithLabelValues("db")); v != 0 {
		t.Errorf("last_success before any run: got %v, want 0", v)
	}

	_ = m.Save("db", 0.1, nil)
	if v := testutil.ToFloat64(m.lastSuccess.WithLabelValues("db")); v <= 0 {
		t.Errorf("last_success after success should be a unix ts > 0, got %v", v)
	}

	// A failure must NOT move the last-success timestamp.
	before := testutil.ToFloat64(m.lastSuccess.WithLabelValues("db"))
	_ = m.Save("db", 0.1, errors.New("boom"))
	if after := testutil.ToFloat64(m.lastSuccess.WithLabelValues("db")); after != before {
		t.Errorf("last_success changed on failure: before=%v after=%v", before, after)
	}
}

func TestMetrics_ErrorsTotal(t *testing.T) {
	m := NewMetrics(false, false, false)

	_ = m.Save("db", 0.1, errors.New("x"))
	_ = m.Save("db", 0.1, errors.New("y"))
	if v := testutil.ToFloat64(m.errors.WithLabelValues("db", "error")); v != 2 {
		t.Errorf(`errors_total{check=db,reason=error}: got %v, want 2`, v)
	}

	// Reason routing.
	_ = m.Save("db", 0.1, context.DeadlineExceeded)
	if v := testutil.ToFloat64(m.errors.WithLabelValues("db", "timeout")); v != 1 {
		t.Errorf(`errors_total{reason=timeout}: got %v, want 1`, v)
	}

	// Success does not increment any error series.
	_ = m.Save("db", 0.1, nil)
	if v := testutil.ToFloat64(m.errors.WithLabelValues("db", "error")); v != 2 {
		t.Errorf("errors_total after success: got %v, want 2", v)
	}
}

type reasonErr struct{ r string }

func (e reasonErr) Error() string  { return e.r }
func (e reasonErr) Reason() string { return e.r }

func TestClassifyReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"plain", errors.New("boom"), "error"},
		{"timeout", context.DeadlineExceeded, "timeout"},
		{"canceled", context.Canceled, "canceled"},
		{"custom reason", reasonErr{"dial"}, "dial"},
		{"wrapped timeout", fmt.Errorf("query: %w", context.DeadlineExceeded), "timeout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyReason(c.err); got != c.want {
				t.Errorf("classifyReason(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}
