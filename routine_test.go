package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func serveHealth(t *testing.T, h *healthCheck) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandlerHealth(rec, httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))
	return rec.Code
}

func TestBackgroundRoutine_PopulatesCacheAndShutdownStops(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	h := New(WithBackCheck(10 * time.Millisecond))
	if err := h.Add("ok", "", okCheck); err != nil {
		t.Fatal(err)
	}

	h.Start()
	time.Sleep(60 * time.Millisecond) // let the routine tick at least once

	if code := serveHealth(t, h); code != http.StatusOK {
		t.Errorf("cached health after tick: got %d, want 200", code)
	}

	h.Shutdown() // must stop the background goroutine (goleak verifies no leak)
}

func TestBackgroundRoutine_ContextCancelStops(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx, cancel := context.WithCancel(context.Background())
	h := New(WithBackCheck(10*time.Millisecond), WithContext(ctx))
	if err := h.Add("ok", "", okCheck); err != nil {
		t.Fatal(err)
	}

	h.Start()
	time.Sleep(30 * time.Millisecond)
	cancel() // cancelling the context must stop the routine; goleak (with retry) confirms
}

func TestBackgroundRoutine_ReflectsFailureOverTime(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	var down atomic.Bool
	h := New(WithBackCheck(10 * time.Millisecond))
	if err := h.Add("dep", "", func(context.Context) error {
		if down.Load() {
			return errors.New("dependency down")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	h.Start()
	defer h.Shutdown()

	time.Sleep(40 * time.Millisecond)
	if code := serveHealth(t, h); code != http.StatusOK {
		t.Errorf("initial cached health: got %d, want 200", code)
	}

	down.Store(true)
	time.Sleep(40 * time.Millisecond)
	if code := serveHealth(t, h); code != http.StatusServiceUnavailable {
		t.Errorf("cached health after failure: got %d, want 503", code)
	}
}
