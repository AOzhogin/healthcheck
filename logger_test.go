package healthcheck

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWithLogger_Transitions(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var fail atomic.Bool
	h := New(WithLogger(logger))
	if err := h.Add("db", "", func(context.Context) error {
		if fail.Load() {
			return errors.New("down")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	run := func() {
		h.HandlerHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))
	}

	// Healthy baseline -> no transition, no log.
	run()
	if strings.Contains(buf.String(), "check") {
		t.Errorf("no log expected on healthy baseline; got: %s", buf.String())
	}

	// ok -> error : error-level "check failed".
	fail.Store(true)
	run()
	if !strings.Contains(buf.String(), "check failed") || !strings.Contains(buf.String(), "check=db") {
		t.Errorf("expected failed-transition log; got: %s", buf.String())
	}

	// Still failing -> no new transition (count stays 1).
	prev := buf.String()
	run()
	if buf.String() != prev {
		t.Errorf("expected no log while state unchanged; got extra: %s", strings.TrimPrefix(buf.String(), prev))
	}

	// error -> ok : info-level "check recovered".
	fail.Store(false)
	run()
	if !strings.Contains(buf.String(), "check recovered") {
		t.Errorf("expected recovered-transition log; got: %s", buf.String())
	}
}

func TestWithLogger_NilIsSilent(t *testing.T) {
	h := New() // no logger
	if err := h.Add("db", "", func(context.Context) error { return errors.New("x") }); err != nil {
		t.Fatal(err)
	}
	// Must not panic with a nil logger.
	h.HandlerHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))
}
