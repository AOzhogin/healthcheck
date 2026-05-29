package healthcheck

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_healthCheck_HandlerHealth(t *testing.T) {
	type args struct {
		Options []HCOption
		CheckFN func(h HealthCheck)
	}
	type want struct {
		status int
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{name: "Normal 200", args: args{Options: nil}, want: want{status: http.StatusOK}},
		{name: "With custom success code", args: args{Options: []HCOption{WithSuccessStatus(http.StatusCreated)}}, want: want{status: http.StatusCreated}},
		{name: "Normal error 503", args: args{Options: nil,
			CheckFN: func(h HealthCheck) {
				_ = h.Add("fail test", "always error check", func(ctx context.Context) error {
					return fmt.Errorf("error")
				})
			},
		}, want: want{status: http.StatusServiceUnavailable}},
		{name: "With custom error 500", args: args{Options: []HCOption{WithErrorStatus(http.StatusInternalServerError)},
			CheckFN: func(h HealthCheck) {
				_ = h.Add("fail test", "always error check", func(ctx context.Context) error {
					return fmt.Errorf("error")
				})
			},
		}, want: want{status: http.StatusInternalServerError}},
		{name: "Check time out", args: args{Options: []HCOption{WithTimeOut(2 * time.Second)},
			CheckFN: func(h HealthCheck) {
				_ = h.Add("fail test", "long time check", func(ctx context.Context) error {
					select {
					case <-ctx.Done():
						return fmt.Errorf("timed out")
					case <-time.After(5 * time.Second):
						return nil
					}
				})
			},
		}, want: want{status: http.StatusServiceUnavailable}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			h := New(tt.args.Options...)

			if tt.args.CheckFN != nil {
				tt.args.CheckFN(h)
			}

			request := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
			response := httptest.NewRecorder()

			h.HandlerHealth(response, request)

			assertResponseCode(t, response.Code, tt.want.status)
			assertResponseContentType(t, response.Header().Get("Content-Type"), "application/json")
			assertResponseBody(t, response.Body.String(), "")
		})
	}
}

func Test_healthCheck_HandlerHealth_BodyJSON(t *testing.T) {
	h := New()
	if err := h.Add("db", "db.company:5432", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck+"?body=true", nil)
	rec := httptest.NewRecorder()
	h.HandlerHealth(rec, req)

	assertResponseCode(t, rec.Code, http.StatusOK)
	assertResponseContentType(t, rec.Header().Get("Content-Type"), "application/json")

	var res struct {
		Status string `json:"status"`
		Checks map[string]struct {
			Status string `json:"status"`
			Notes  string `json:"notes"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("body is not valid JSON: %v; body=%s", err, rec.Body.String())
	}
	if res.Status != "ok" {
		t.Errorf("status: got %q, want %q", res.Status, "ok")
	}
	db, ok := res.Checks["db"]
	if !ok {
		t.Fatalf("checks does not contain %q: %s", "db", rec.Body.String())
	}
	if db.Status != "ok" {
		t.Errorf("db check status: got %q, want %q", db.Status, "ok")
	}
	if db.Notes != "db.company:5432" {
		t.Errorf("db check notes: got %q, want %q", db.Notes, "db.company:5432")
	}
}

func Test_healthCheck_HandlerMetrics(t *testing.T) {

	type args struct {
		Options []HCOption
	}
	type want struct {
		status      int
		contentType string // empty = do not check
		bodyContains string // empty = do not check; if set, body must contain this
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Normal",
			args: args{Options: []HCOption{WithMetrics(false, false, false)}},
			want: want{status: http.StatusOK, contentType: "text/plain", bodyContains: "promhttp"},
		},
		{
			name: "Not implemented",
			args: args{Options: nil},
			want: want{status: http.StatusNotImplemented},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			request := httptest.NewRequest(http.MethodGet, HandlerMetrics, nil)
			response := httptest.NewRecorder()

			h := New(tt.args.Options...)

			h.HandlerMetrics(response, request)

			assertResponseCode(t, response.Code, tt.want.status)
			if tt.want.contentType != "" {
				ct := response.Header().Get("Content-Type")
				if !strings.Contains(ct, tt.want.contentType) {
					t.Errorf("response content type: got %q, want to contain %q", ct, tt.want.contentType)
				}
			}
			if tt.want.bodyContains != "" {
				assertResponseBodyContains(t, response.Body.String(), tt.want.bodyContains)
			}
		})
	}
}

func Test_healthCheck_HandlerPProf(t *testing.T) {

	type args struct {
		Options []HCOption
	}
	type want struct {
		status int
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{name: "Index page", args: args{Options: nil}, want: want{status: http.StatusOK}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			request := httptest.NewRequest(http.MethodGet, HandlerDebug, nil)
			response := httptest.NewRecorder()

			h := New(tt.args.Options...)

			h.HandlerPProf(response, request)

			assertResponseCode(t, response.Code, tt.want.status)
			assertResponseContentType(t, response.Header().Get("Content-Type"), "text/html; charset=utf-8")
			body := response.Body.String()
			// pprof index has dynamic counts (goroutine, allocs, etc.); check key markers only
			assertResponseBodyContains(t, body, "/debug/pprof/")
			assertResponseBodyContains(t, body, "Types of profiles available")
			for _, profile := range []string{"allocs", "block", "cmdline", "goroutine", "heap", "mutex", "profile", "threadcreate", "trace"} {
				assertResponseBodyContains(t, body, profile)
			}
		})
	}
}

func Test_healthCheck_HandlerPProf_ProfileEndpoints(t *testing.T) {
	// Ensure each pprof profile is served and returns 200.
	// Without debug=1 profiles are served in binary form (application/octet-stream).
	profiles := []struct {
		path         string
		contentType  string // empty means do not check
		bodyContains string // empty means do not check
	}{
		{path: "/debug/goroutine", contentType: "", bodyContains: ""}, // binary
		{path: "/debug/goroutine?debug=1", contentType: "text/plain; charset=utf-8", bodyContains: "goroutine"},
		{path: "/debug/heap", contentType: "", bodyContains: ""},
		{path: "/debug/heap?debug=1", contentType: "text/plain; charset=utf-8", bodyContains: "heap"},
		{path: "/debug/allocs", contentType: "", bodyContains: ""},
		{path: "/debug/allocs?debug=1", contentType: "text/plain; charset=utf-8", bodyContains: "allocs"},
		{path: "/debug/block?debug=1", contentType: "text/plain; charset=utf-8", bodyContains: "contention"}, // pprof block text format
		{path: "/debug/mutex?debug=1", contentType: "text/plain; charset=utf-8", bodyContains: "mutex"},
		{path: "/debug/threadcreate?debug=1", contentType: "text/plain; charset=utf-8", bodyContains: "threadcreate"},
	}

	h := New()

	for _, p := range profiles {
		t.Run(p.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p.path, nil)
			rec := httptest.NewRecorder()
			h.HandlerPProf(rec, req)

			assertResponseCode(t, rec.Code, http.StatusOK)
			if p.contentType != "" {
				ct := rec.Header().Get("Content-Type")
				if ct != p.contentType {
					t.Errorf("Content-Type: got %q, want %q", ct, p.contentType)
				}
			}
			if p.bodyContains != "" {
				assertResponseBodyContains(t, rec.Body.String(), p.bodyContains)
			}
		})
	}
}

func Test_healthCheck_HandlerPProf_IndexVsProfile(t *testing.T) {
	// Ensure /debug/ serves the index (HTML) and /debug/goroutine serves the profile (text)
	h := New()

	t.Run("index is HTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, HandlerDebug, nil)
		rec := httptest.NewRecorder()
		h.HandlerPProf(rec, req)
		assertResponseCode(t, rec.Code, http.StatusOK)
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("index Content-Type: got %q, want text/html", ct)
		}
		if !strings.Contains(rec.Body.String(), "<html>") {
			t.Error("index response should contain <html>")
		}
	})

	t.Run("goroutine profile is text when debug=1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/goroutine?debug=1", nil)
		rec := httptest.NewRecorder()
		h.HandlerPProf(rec, req)
		assertResponseCode(t, rec.Code, http.StatusOK)
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("goroutine profile Content-Type: got %q, want text/plain", ct)
		}
	})
}

func assertResponseBodyContains(t testing.TB, body, substr string) {
	t.Helper()
	if substr != "" && !strings.Contains(body, substr) {
		t.Errorf("response body does not contain %q, got: %s", substr, body)
	}
}

func assertResponseCode(t testing.TB, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("response statusCode is wrong, got %d want %d", got, want)
	}
}

func assertResponseContentType(t testing.TB, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("response content type is wrong, got %q want %q", got, want)
	}
}

func assertResponseBody(t testing.TB, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("response body is wrong, got %q want %q", got, want)
	}
}

func Test_healthCheck_RequireBasicAuth(t *testing.T) {
	const user, pass = "admin", "secret"

	t.Run("no auth when disabled", func(t *testing.T) {
		h := New()
		req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
		rec := httptest.NewRecorder()
		h.HandlerHealth(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("without Basic Auth option: got status %d, want 200", rec.Code)
		}
	})

	t.Run("401 without Authorization header when enabled", func(t *testing.T) {
		h := New(WithBasicAuth(user, pass))
		req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
		rec := httptest.NewRecorder()
		h.MiddlewareAuth(http.HandlerFunc(h.HandlerHealth)).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("no creds: got status %d, want 401", rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Error("want WWW-Authenticate header on 401")
		}
	})

	t.Run("401 with wrong credentials", func(t *testing.T) {
		h := New(WithBasicAuth(user, pass))
		req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("wrong:wrong")))
		rec := httptest.NewRecorder()
		h.MiddlewareAuth(http.HandlerFunc(h.HandlerHealth)).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("wrong creds: got status %d, want 401", rec.Code)
		}
	})

	t.Run("200 with valid credentials", func(t *testing.T) {
		h := New(WithBasicAuth(user, pass))
		req := httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		rec := httptest.NewRecorder()
		h.MiddlewareAuth(http.HandlerFunc(h.HandlerHealth)).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("valid creds: got status %d, want 200", rec.Code)
		}
	})

	t.Run("401 then 200 for /metrics with Basic Auth", func(t *testing.T) {
		h := New(WithBasicAuth(user, pass), WithMetrics(false, false, false))
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
		metricsHandler := h.MiddlewareAuth(http.HandlerFunc(h.HandlerMetrics))

		req := httptest.NewRequest(http.MethodGet, HandlerMetrics, nil)
		rec := httptest.NewRecorder()
		metricsHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("metrics no auth: got %d, want 401", rec.Code)
		}

		req2 := httptest.NewRequest(http.MethodGet, HandlerMetrics, nil)
		req2.Header.Set("Authorization", auth)
		rec2 := httptest.NewRecorder()
		metricsHandler.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Errorf("metrics with auth: got %d, want 200", rec2.Code)
		}
	})

	t.Run("401 then 200 for /debug/ (pprof) with Basic Auth", func(t *testing.T) {
		h := New(WithBasicAuth(user, pass))
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))

		req := httptest.NewRequest(http.MethodGet, HandlerDebug, nil)
		rec := httptest.NewRecorder()
		h.MiddlewareAuth(http.HandlerFunc(h.HandlerPProf)).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("debug no auth: got %d, want 401", rec.Code)
		}

		req2 := httptest.NewRequest(http.MethodGet, HandlerDebug, nil)
		req2.Header.Set("Authorization", auth)
		rec2 := httptest.NewRecorder()
		h.MiddlewareAuth(http.HandlerFunc(h.HandlerPProf)).ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Errorf("debug with auth: got %d, want 200", rec2.Code)
		}
		assertResponseBodyContains(t, rec2.Body.String(), "/debug/pprof/")
	})
}
