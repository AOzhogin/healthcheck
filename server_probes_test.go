package healthcheck

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"
)

// startServer boots StartHTTPServer for h on addr in a goroutine and waits briefly for it to listen.
// The HTTP server is intentionally not shut down (no graceful shutdown in scope), so these tests
// must not be wrapped with goleak.
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

func TestHTTPServer_K8sProbes_DefaultReflectsFailure(t *testing.T) {
	const addr = ":18091"
	h := New(WithHTTPAddress(addr))
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("dependency down") }); err != nil {
		t.Fatal(err)
	}
	startServer(t, h)
	base := "http://localhost" + addr

	// All probes default to running the checks, so a failing check yields 503.
	for _, ep := range []string{HandlerLive, HandlerReady, HandlerStartup} {
		if code := getStatus(t, base+ep); code != http.StatusServiceUnavailable {
			t.Errorf("default %s with failing check: got %d, want 503", ep, code)
		}
	}
}

func TestHTTPServer_K8sProbes_CustomOverride(t *testing.T) {
	const addr = ":18092"
	h := New(WithHTTPAddress(addr))
	// Add a failing check to prove liveness is decoupled from it via a custom handler.
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("down") }); err != nil {
		t.Fatal(err)
	}
	// Liveness: cheap, always alive regardless of dependencies.
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
	// Startup left default -> runs checks -> 503 because of the failing dependency.
	if code := getStatus(t, base+HandlerStartup); code != http.StatusServiceUnavailable {
		t.Errorf("default startup with failing check: got %d, want 503", code)
	}
}

func TestHTTPServer_K8sProbes_CustomBehindBasicAuth(t *testing.T) {
	const addr = ":18093"
	const user, pass = "admin", "secret"
	h := New(WithHTTPAddress(addr), WithBasicAuth(user, pass))
	h.SetHandlerLive(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	startServer(t, h)
	base := "http://localhost" + addr

	if code := getStatus(t, base+HandlerLive); code != http.StatusUnauthorized {
		t.Errorf("custom live without creds: got %d, want 401", code)
	}

	req, _ := http.NewRequest(http.MethodGet, base+HandlerLive, nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("custom live with creds: got %d, want 200", resp.StatusCode)
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
