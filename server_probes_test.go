package healthcheck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// startServer boots StartHTTPServer for h on addr in a goroutine and waits briefly for it to listen.
// The HTTP server is intentionally not shut down (no graceful shutdown in scope), so it keeps
// running for the rest of the package's tests.
func startServer(t *testing.T, h *healthCheck) {
	t.Helper()
	go func() {
		if err := h.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server error: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
}

func getStatus(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestHTTPServer_K8sProbes_Default(t *testing.T) {
	const addr = ":18090"
	h := New(WithHTTPAddress(addr))
	startServer(t, h)
	base := "http://localhost" + addr

	for _, ep := range []string{HandlerLive, HandlerReady, HandlerStartup} {
		if code := getStatus(t, base+ep); code != http.StatusOK {
			t.Errorf("default %s: got %d, want 200", ep, code)
		}
	}
}

func TestHTTPServer_K8sProbes_DefaultIgnoresChecks(t *testing.T) {
	const addr = ":18091"
	h := New(WithHTTPAddress(addr))
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("dependency down") }); err != nil {
		t.Fatal(err)
	}
	startServer(t, h)
	base := "http://localhost" + addr

	// Default probes answer statusCodeSuccess without running the checks.
	for _, ep := range []string{HandlerLive, HandlerReady, HandlerStartup} {
		if code := getStatus(t, base+ep); code != http.StatusOK {
			t.Errorf("default %s with failing check: got %d, want 200", ep, code)
		}
	}

	const addrCustom = ":18300"
	hc := New(WithHTTPAddress(addrCustom), WithSuccessStatus(http.StatusNoContent))
	startServer(t, hc)
	for _, ep := range []string{HandlerLive, HandlerReady, HandlerStartup} {
		if code := getStatus(t, "http://localhost"+addrCustom+ep); code != http.StatusNoContent {
			t.Errorf("default %s with WithSuccessStatus(204): got %d, want 204", ep, code)
		}
	}
}

func TestHTTPServer_K8sProbes_CustomOverride(t *testing.T) {
	const addr = ":18092"
	h := New(WithHTTPAddress(addr))
	// A failing check: default probes must not depend on it.
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("down") }); err != nil {
		t.Fatal(err)
	}
	h.SetHandlerLive(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	// Readiness: custom failure code to prove the override is wired.
	h.SetHandlerReady(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	startServer(t, h)
	base := "http://localhost" + addr

	if code := getStatus(t, base+HandlerLive); code != http.StatusOK {
		t.Errorf("custom live: got %d, want 200", code)
	}
	if code := getStatus(t, base+HandlerReady); code != http.StatusServiceUnavailable {
		t.Errorf("custom ready: got %d, want 503", code)
	}
	// Startup left default -> answers success regardless of the failing dependency.
	if code := getStatus(t, base+HandlerStartup); code != http.StatusOK {
		t.Errorf("default startup with failing check: got %d, want 200", code)
	}
}

func TestHTTPServer_WithoutPProf(t *testing.T) {
	t.Run("disabled returns 404", func(t *testing.T) {
		const addr = ":18094"
		h := New(WithHTTPAddress(addr), WithoutPProf())
		startServer(t, h)
		if code := getStatus(t, "http://localhost"+addr+HandlerDebug); code != http.StatusNotFound {
			t.Errorf("with WithoutPProf, %s: got %d, want 404", HandlerDebug, code)
		}
	})

	t.Run("enabled by default returns 200", func(t *testing.T) {
		const addr = ":18095"
		h := New(WithHTTPAddress(addr))
		startServer(t, h)
		if code := getStatus(t, "http://localhost"+addr+HandlerDebug); code != http.StatusOK {
			t.Errorf("default pprof, %s: got %d, want 200", HandlerDebug, code)
		}
	})
}

func TestHTTPServer_MetricsAndDebug_OverWire(t *testing.T) {
	const addr = ":18096"
	h := New(WithHTTPAddress(addr), WithMetrics(false, false, false))
	startServer(t, h)
	base := "http://localhost" + addr

	if code := getStatus(t, base+HandlerMetrics); code != http.StatusOK {
		t.Errorf("%s: got %d, want 200", HandlerMetrics, code)
	}
	if code := getStatus(t, base+HandlerDebug); code != http.StatusOK {
		t.Errorf("%s: got %d, want 200", HandlerDebug, code)
	}
	if code := getStatus(t, base+HandlerHealthCheck); code != http.StatusOK {
		t.Errorf("%s: got %d, want 200", HandlerHealthCheck, code)
	}
}

func TestHTTPServer_K8sProbes_OpenWithBasicAuth(t *testing.T) {
	const addr = ":18093"
	h := New(WithHTTPAddress(addr), WithBasicAuth("admin", "secret"), WithMetrics(false, false, false))
	h.SetHandlerLive(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	startServer(t, h)
	base := "http://localhost" + addr

	for _, ep := range []string{HandlerLive, HandlerReady, HandlerStartup} {
		if code := getStatus(t, base+ep); code != http.StatusOK {
			t.Errorf("probe %s without creds: got %d, want 200", ep, code)
		}
	}
	for _, ep := range []string{HandlerHealthCheck, HandlerMetrics, HandlerDebug} {
		if code := getStatus(t, base+ep); code != http.StatusUnauthorized {
			t.Errorf("%s without creds: got %d, want 401", ep, code)
		}
	}
}

func TestProbe_BodyQueryIgnored(t *testing.T) {
	const addr = ":18301"
	const user, pass = "admin", "secret"
	h := New(WithHTTPAddress(addr), WithBasicAuth(user, pass))
	h.SetHandlerReady(h.HandlerHealth)
	startServer(t, h)
	base := "http://localhost" + addr

	_, body := doRequest(t, http.MethodGet, base+HandlerReady+"?body=true", nil)
	if body != "" {
		t.Errorf("/ready?body=true: got body %q, want empty", body)
	}

	code, body := doRequest(t, http.MethodGet, base+HandlerHealthCheck+"?body=true", func(r *http.Request) { r.SetBasicAuth(user, pass) })
	var v map[string]any
	if code != http.StatusOK || json.Unmarshal([]byte(body), &v) != nil {
		t.Errorf("/health?body=true with creds: got %d %q, want 200 + JSON", code, body)
	}
}

func TestProbe_StatusPropagates(t *testing.T) {
	const addr = ":18302"
	h := New(WithHTTPAddress(addr))
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("down") }); err != nil {
		t.Fatal(err)
	}
	h.SetHandlerReady(h.HandlerHealth)
	startServer(t, h)

	if code := getStatus(t, "http://localhost"+addr+HandlerReady); code != http.StatusServiceUnavailable {
		t.Errorf("gated /ready with failing check: got %d, want 503", code)
	}
}

func TestProbe_CustomHandlerBodyDropped(t *testing.T) {
	const addr = ":18303"
	h := New(WithHTTPAddress(addr))
	h.SetHandlerReady(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"secret":"details"}`))
	})
	startServer(t, h)

	resp, err := http.Get("http://localhost" + addr + HandlerReady)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(b) != 0 || resp.Header.Get("Content-Type") != "" {
		t.Errorf("custom ready: got %d %q Content-Type=%q, want 200, empty, none", resp.StatusCode, b, resp.Header.Get("Content-Type"))
	}
}

func TestProbe_MethodNotAllowed(t *testing.T) {
	const addr = ":18304"
	h := New(WithHTTPAddress(addr))
	startServer(t, h)
	base := "http://localhost" + addr

	if code, _ := doRequest(t, http.MethodPost, base+HandlerLive, nil); code != http.StatusMethodNotAllowed {
		t.Errorf("POST /live: got %d, want 405", code)
	}
	if code, _ := doRequest(t, http.MethodHead, base+HandlerLive, nil); code != http.StatusOK {
		t.Errorf("HEAD /live: got %d, want 200", code)
	}
}

func TestProbe_MiddlewareStillApplied(t *testing.T) {
	const addr = ":18305"
	var mu sync.Mutex
	seen := map[string]int{}
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen[r.URL.Path]++
			mu.Unlock()
			if r.Header.Get("X-Deny") == "1" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	h := New(WithHTTPAddress(addr), WithMiddleware(mw))
	startServer(t, h)
	base := "http://localhost" + addr

	if code := getStatus(t, base+HandlerReady); code != http.StatusOK {
		t.Errorf("/ready through middleware: got %d, want 200", code)
	}
	if code, _ := doRequest(t, http.MethodGet, base+HandlerReady, func(r *http.Request) { r.Header.Set("X-Deny", "1") }); code != http.StatusForbidden {
		t.Errorf("/ready denied by middleware: got %d, want 403", code)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen[HandlerReady] != 2 {
		t.Errorf("middleware saw /ready %d times, want 2", seen[HandlerReady])
	}
}

// doRequest sends method to url (after optional mutation of the request) and returns code and body.
func doRequest(t *testing.T, method, url string, mutate func(*http.Request)) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
