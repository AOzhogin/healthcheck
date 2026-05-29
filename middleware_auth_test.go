package healthcheck

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	middlewareAuthTestPort = ":18082"
	middlewareAuthUser     = "admin"
	middlewareAuthPass     = "secret"
)

func TestWithMiddlewareAndBasicAuth_StartHTTPServer(t *testing.T) {
	customMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	}

	hc := New(
		WithHTTPAddress(middlewareAuthTestPort),
		WithMiddleware(customMiddleware),
		WithBasicAuth(middlewareAuthUser, middlewareAuthPass),
	)

	go func() {
		_ = hc.StartHTTPServer()
	}()
	time.Sleep(100 * time.Millisecond)
	defer func() {
		hc.Shutdown()
	}()

	baseURL := "http://localhost" + middlewareAuthTestPort
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(middlewareAuthUser+":"+middlewareAuthPass))

	t.Run("without auth returns 401", func(t *testing.T) {
		resp, err := http.Get(baseURL + HandlerHealthCheck)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
		}
		if resp.Header.Get("WWW-Authenticate") == "" {
			t.Error("expected WWW-Authenticate header on 401")
		}
	})

	t.Run("with valid auth returns 200", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+HandlerHealthCheck, nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 with valid auth, got %d", resp.StatusCode)
		}
	})

	t.Run("custom middleware adds header", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+HandlerHealthCheck, nil)
		req.Header.Set("Authorization", authHeader)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if v := resp.Header.Get("X-Custom-Middleware"); v != "true" {
			t.Errorf("expected X-Custom-Middleware: true, got %q", v)
		}
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+HandlerHealthCheck, nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:wrong")))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 with wrong password, got %d", resp.StatusCode)
		}
	})
}

func TestMiddlewareAuth_PassThroughWhenDisabled(t *testing.T) {
	h := New() // Basic Auth disabled
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
	h.MiddlewareAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("auth disabled should pass through: got %d, want 200", rec.Code)
	}
}

func TestWithMiddleware_WrapsHandler(t *testing.T) {
	called := false
	customMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.Header().Set("X-Custom-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	}

	wrap := customMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
	wrap.ServeHTTP(rec, req)
	if !called {
		t.Error("custom middleware was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom-Middleware") != "true" {
		t.Error("expected X-Custom-Middleware header")
	}
}
