package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func serveHealth(t *testing.T, h *healthCheck) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandlerHealth(rec, httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))
	return rec.Code
}

// waitStopped asserts that wait returns within d. The background routine's WaitGroup is only done
// once the goroutine has returned, so this verifies the routine actually stopped instead of leaking.
func waitStopped(t *testing.T, wait func(), d time.Duration) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("background routine did not stop")
	}
}

func TestBackgroundRoutine_PopulatesCacheAndShutdownStops(t *testing.T) {
	h := New(WithBackCheck(10 * time.Millisecond))
	if err := h.Add("ok", "", okCheck); err != nil {
		t.Fatal(err)
	}

	h.Start()
	time.Sleep(60 * time.Millisecond) // let the routine tick at least once

	if code := serveHealth(t, h); code != http.StatusOK {
		t.Errorf("cached health after tick: got %d, want 200", code)
	}

	waitStopped(t, h.Shutdown, time.Second) // Shutdown must stop the background goroutine
}

func TestBackgroundRoutine_ContextCancelStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := New(WithBackCheck(10*time.Millisecond), WithContext(ctx))
	if err := h.Add("ok", "", okCheck); err != nil {
		t.Fatal(err)
	}

	h.Start()
	time.Sleep(30 * time.Millisecond)
	cancel() // cancelling the context must stop the routine, without Shutdown being called

	waitStopped(t, h.wg.Wait, time.Second)
}

func TestBackgroundRoutine_ReflectsFailureOverTime(t *testing.T) {
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
	defer waitStopped(t, h.Shutdown, time.Second)

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
