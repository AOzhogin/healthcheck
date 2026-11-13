package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProbeHandler_Default(t *testing.T) {
	h := New(WithSuccessStatus(http.StatusNoContent))
	rec := httptest.NewRecorder()
	h.resolveHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HandlerReady, nil))

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Empty(t, rec.Header().Get("Content-Type"))
}

func TestMiddlewareProbe_DropsBodyAndContentType(t *testing.T) {
	h := New()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"x":1}`))
	})
	rec := httptest.NewRecorder()
	h.MiddlewareProbe(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HandlerReady, nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Empty(t, rec.Header().Get("Content-Type"))
}

func TestMiddlewareProbe_StripsQuery(t *testing.T) {
	h := New()
	query := "unset"
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	})
	h.MiddlewareProbe(inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, HandlerReady+"?body=true", nil))

	assert.Empty(t, query)
}

func TestMiddlewareProbe_MethodNotAllowed(t *testing.T) {
	h := New()
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true })
	rec := httptest.NewRecorder()
	h.MiddlewareProbe(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, HandlerLive, nil))

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
	assert.False(t, called)
}

func TestMiddlewareProbe_Head(t *testing.T) {
	h := New()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	h.MiddlewareProbe(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodHead, HandlerLive, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestMiddlewareProbe_WriteWithoutHeader(t *testing.T) {
	h := New()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) })
	rec := httptest.NewRecorder()
	h.MiddlewareProbe(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HandlerLive, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestMiddlewareProbe_HealthHandlerAsCustom(t *testing.T) {
	h := New()
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("down") }); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.MiddlewareProbe(http.HandlerFunc(h.HandlerHealth)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, HandlerReady+"?body=true", nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Empty(t, rec.Header().Get("Content-Type"))
}

func TestServeProbes_OwnMux(t *testing.T) {
	h := New()
	if err := h.Add("dep", "down", func(context.Context) error { return errors.New("down") }); err != nil {
		t.Fatal(err)
	}
	h.SetHandlerReady(h.HandlerHealth)

	mux := http.NewServeMux()
	mux.Handle(HandlerLive, h.ServeLive())
	mux.Handle(HandlerReady, h.ServeReady())
	mux.Handle(HandlerStartup, h.ServeStartup())
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, ep := range []string{HandlerLive, HandlerStartup} {
		code, body := doRequest(t, http.MethodGet, srv.URL+ep, nil)
		assert.Equal(t, http.StatusOK, code, ep)
		assert.Empty(t, body, ep)
	}

	code, body := doRequest(t, http.MethodGet, srv.URL+HandlerReady+"?body=true", nil)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Empty(t, body)

	code, _ = doRequest(t, http.MethodPost, srv.URL+HandlerLive, nil)
	assert.Equal(t, http.StatusMethodNotAllowed, code)
}
