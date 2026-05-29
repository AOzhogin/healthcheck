package healthcheck

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_Register(t *testing.T) {
	m := NewMetrics(false, false, false)

	if err := m.Register("db"); err != nil {
		t.Fatalf("first Register: unexpected error: %v", err)
	}
	if err := m.Register("db"); err == nil {
		t.Error("duplicate Register should return an error")
	}
}

func TestMetrics_Save_GaugeValue(t *testing.T) {
	m := NewMetrics(false, false, false)
	if err := m.Register("db"); err != nil {
		t.Fatal(err)
	}

	gauge := m.mm["db"].(prometheus.Gauge)

	if err := m.Save("db", 0.1, nil); err != nil {
		t.Fatalf("Save(nil err): %v", err)
	}
	if v := testutil.ToFloat64(gauge); v != 1 {
		t.Errorf("gauge after success: got %v, want 1", v)
	}

	if err := m.Save("db", 0.2, errors.New("boom")); err != nil {
		t.Fatalf("Save(err): %v", err)
	}
	if v := testutil.ToFloat64(gauge); v != 0 {
		t.Errorf("gauge after error: got %v, want 0", v)
	}
}

func TestMetrics_Save_Errors(t *testing.T) {
	t.Run("unknown gauge collector", func(t *testing.T) {
		m := NewMetrics(false, false, false)
		if err := m.Save("missing", 0.1, nil); err == nil {
			t.Error("expected error for unknown collector")
		}
	})

	t.Run("wrong gauge collector type", func(t *testing.T) {
		m := NewMetrics(false, false, false)
		m.mm["weird"] = prometheus.NewCounter(prometheus.CounterOpts{Name: "weird"})
		if err := m.Save("weird", 0.1, nil); err == nil {
			t.Error("expected error when collector is not a Gauge")
		}
	})

	t.Run("missing histogram collector", func(t *testing.T) {
		m := NewMetrics(false, false, false)
		m.mm["onlygauge"] = prometheus.NewGauge(prometheus.GaugeOpts{Name: "onlygauge"})
		if err := m.Save("onlygauge", 0.1, nil); err == nil {
			t.Error("expected error when histogram collector is missing")
		}
	})

	t.Run("wrong histogram collector type", func(t *testing.T) {
		m := NewMetrics(false, false, false)
		m.mm["gh"] = prometheus.NewGauge(prometheus.GaugeOpts{Name: "gh"})
		m.mm[fmt.Sprintf("%s_dur", "gh")] = prometheus.NewCounter(prometheus.CounterOpts{Name: "gh_dur"})
		if err := m.Save("gh", 0.1, nil); err == nil {
			t.Error("expected error when _dur collector is not a Histogram")
		}
	})
}
