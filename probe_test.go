package healthcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// okCheck is a shared check that always succeeds.
func okCheck(context.Context) error { return nil }

func TestHandlerReadyConstant(t *testing.T) {
	if HandlerReady != "/ready" {
		t.Errorf("HandlerReady = %q, want %q", HandlerReady, "/ready")
	}
}

func TestSetHandlerProbes(t *testing.T) {
	cases := []struct {
		name string
		set  func(h *healthCheck, fn http.HandlerFunc)
		get  func(h *healthCheck) http.HandlerFunc
	}{
		{"live", (*healthCheck).SetHandlerLive, func(h *healthCheck) http.HandlerFunc { return h.liveHandler }},
		{"ready", (*healthCheck).SetHandlerReady, func(h *healthCheck) http.HandlerFunc { return h.readyHandler }},
		{"startup", (*healthCheck).SetHandlerStartup, func(h *healthCheck) http.HandlerFunc { return h.startupHandler }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := New()
			if tc.get(h) != nil {
				t.Fatalf("%s handler should default to nil", tc.name)
			}

			tc.set(h, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
			if tc.get(h) == nil {
				t.Fatalf("%s handler should be set after Set", tc.name)
			}

			tc.set(h, nil)
			if tc.get(h) != nil {
				t.Errorf("%s handler should reset to nil (default) when set to nil", tc.name)
			}
		})
	}
}

func TestResolveHandler(t *testing.T) {
	h := New()

	t.Run("nil falls back to default health handler", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, HandlerLive, nil)
		h.resolveHandler(nil).ServeHTTP(rec, req)
		// default (HandlerHealth) with no checks returns success.
		assertResponseCode(t, rec.Code, http.StatusOK)
		assertResponseContentType(t, rec.Header().Get("Content-Type"), "application/json")
	})

	t.Run("custom handler is used", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, HandlerLive, nil)
		h.resolveHandler(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }).ServeHTTP(rec, req)
		assertResponseCode(t, rec.Code, http.StatusTeapot)
	})
}
