package healthcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func failing(msg string) HCFunc {
	return func(context.Context) error { return errors.New(msg) }
}

func healthBody(t *testing.T, h *healthCheck) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandlerHealth(rec, httptest.NewRequest(http.MethodGet, "/health?body=true", nil))
	return rec.Body.String()
}

func TestMCPStatus_MatchesHealthBody_Background(t *testing.T) {
	h := New(WithBackCheck(20 * time.Millisecond))
	if err := h.Add("ok", "", okCheck); err != nil {
		t.Fatal(err)
	}
	if err := h.Add("bad", "", failing("boom")); err != nil {
		t.Fatal(err)
	}
	h.Start()

	deadline := time.Now().Add(2 * time.Second)
	for {
		h.cacheMutex.Lock()
		n := len(h.cache.Checks)
		h.cacheMutex.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache not filled")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Stop the routine so the cache is stable between the two reads (h.routine stays true).
	h.Shutdown()

	got := string(h.mcpStatus(context.Background())) + "\n"
	want := healthBody(t, h)
	if got != want {
		t.Errorf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestMCPStatus_MatchesHealthBody_NoBackground(t *testing.T) {
	h := New()
	if err := h.Add("ok", "n1", okCheck); err != nil {
		t.Fatal(err)
	}
	if err := h.Add("bad", "n2", failing("boom")); err != nil {
		t.Fatal(err)
	}

	var a, b checkResults
	if err := json.Unmarshal(h.mcpStatus(context.Background()), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(healthBody(t, h)), &b); err != nil {
		t.Fatal(err)
	}
	if a.Status != b.Status || a.Status == "" {
		t.Errorf("status: %q vs %q", a.Status, b.Status)
	}
	if len(a.Checks) != 2 || len(b.Checks) != 2 {
		t.Fatalf("checks: %d vs %d", len(a.Checks), len(b.Checks))
	}
	for name, ra := range a.Checks {
		rb := b.Checks[name]
		if ra.Status != rb.Status || ra.Notes != rb.Notes {
			t.Errorf("%s: %+v vs %+v", name, ra, rb)
		}
	}
}

func TestMCPStatus_EmptyCacheBeforeFirstTick(t *testing.T) {
	h := New(WithBackCheck(time.Hour))
	h.Start()
	t.Cleanup(h.Shutdown)

	raw := h.mcpStatus(context.Background())
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON %s: %v", raw, err)
	}
	// WithBackCheck seeds the cache with Status "error" (code 102) until the first tick;
	// mcpStatus must report exactly what /health?body=true reports.
	if s, ok := m["status"]; !ok || s != checkStatusError {
		t.Errorf("status = %v, want %q", s, checkStatusError)
	}
	// (HandlerHealth itself cannot be compared here: net/http refuses a body with status 102.)
}

func TestMCPRunCheck_UnknownName(t *testing.T) {
	h := New()
	_, err := h.mcpRunCheck(context.Background(), "nope")
	if err == nil || err.Error() != `healthcheck: mcp: unknown check "nope"` {
		t.Errorf("err = %v", err)
	}
}

type mcpOne struct {
	Name   string    `json:"name"`
	Time   time.Time `json:"time"`
	Status string    `json:"status"`
	Exec   float64   `json:"exec"`
	Notes  string    `json:"notes"`
	Result string    `json:"result"`
}

func runOne(t *testing.T, h *healthCheck, name string) mcpOne {
	t.Helper()
	raw, err := h.mcpRunCheck(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	var r mcpOne
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMCPRunCheck_OkAndFailing(t *testing.T) {
	h := New()
	if err := h.Add("db", "main DB", okCheck); err != nil {
		t.Fatal(err)
	}
	if err := h.Add("redis", "", failing("dial timeout")); err != nil {
		t.Fatal(err)
	}

	r := runOne(t, h, "db")
	if r.Name != "db" || r.Status != "ok" || r.Notes != "main DB" || r.Exec < 0 || r.Time.IsZero() || r.Result != "" {
		t.Errorf("db: %+v", r)
	}
	if r := runOne(t, h, "redis"); r.Status != "dial timeout" {
		t.Errorf("redis: %+v", r)
	}

	h2 := New(WithCheckStatusSuccess("UP"))
	if err := h2.Add("db", "", okCheck); err != nil {
		t.Fatal(err)
	}
	if r := runOne(t, h2, "db"); r.Status != "UP" {
		t.Errorf("status = %q, want UP", r.Status)
	}
}

func TestMCPRunCheck_RespectsTimeout(t *testing.T) {
	h := New(WithTimeOut(50 * time.Millisecond))
	if err := h.Add("slow", "", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r := runOne(t, h, "slow")
	if time.Since(start) > time.Second {
		t.Error("took too long")
	}
	if r.Status != context.DeadlineExceeded.Error() {
		t.Errorf("status = %q", r.Status)
	}
}

func TestMCPRunCheck_NoSideEffects(t *testing.T) {
	var buf bytes.Buffer
	ft := &fakeTracer{}
	h := New(
		WithMetrics(false, false, false),
		WithLogger(slog.New(slog.NewTextHandler(&buf, nil))),
		WithTracer(ft),
		WithBackCheck(time.Hour),
	)
	var fail atomic.Bool
	if err := h.Add("x", "", func(context.Context) error {
		if fail.Load() {
			return errors.New("kaput")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	h.cache = h.check(ctx)
	fail.Store(true)

	m := h.Metrics.(*metrics)
	hist := histogramCount(t, m, "x")
	prev := map[string]bool{}
	for k, v := range h.prevFailed {
		prev[k] = v
	}
	cache := h.cache
	ft.mu.Lock()
	spans := len(ft.spans)
	ft.mu.Unlock()
	logLen := buf.Len()

	r := runOne(t, h, "x")
	if r.Status != "kaput" {
		t.Fatalf("status = %q", r.Status)
	}

	if got := histogramCount(t, m, "x"); got != hist {
		t.Errorf("histogram %d -> %d", hist, got)
	}
	if !reflect.DeepEqual(prev, h.prevFailed) {
		t.Errorf("prevFailed changed: %v -> %v", prev, h.prevFailed)
	}
	if !reflect.DeepEqual(cache, h.cache) {
		t.Error("cache changed")
	}
	ft.mu.Lock()
	if len(ft.spans) != spans {
		t.Errorf("spans %d -> %d", spans, len(ft.spans))
	}
	ft.mu.Unlock()
	if buf.Len() != logLen {
		t.Errorf("log grew: %q", buf.String()[logLen:])
	}
}

func TestMCPRunCheck_ConcurrentWithCheck(t *testing.T) {
	h := New()
	var counter atomic.Int64
	if err := h.Add("c", "", func(context.Context) error {
		counter.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	const goroutines, iters = 8, 20
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if g%2 == 0 {
					raw, err := h.mcpRunCheck(context.Background(), "c")
					var r mcpOne
					if err != nil || json.Unmarshal(raw, &r) != nil || r.Status != "ok" {
						t.Errorf("mcpRunCheck: %s %v", raw, err)
					}
				} else {
					h.check(context.Background())
				}
			}
		}(g)
	}
	wg.Wait()

	if got := counter.Load(); got != goroutines*iters {
		t.Errorf("counter = %d, want %d", got, goroutines*iters)
	}
}
