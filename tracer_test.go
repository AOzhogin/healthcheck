package healthcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeSpan struct {
	name  string
	attrs map[string]any
	errs  []error
	ended bool
}

func (s *fakeSpan) SetAttr(k string, v any) { s.attrs[k] = v }
func (s *fakeSpan) RecordError(e error)     { s.errs = append(s.errs, e) }
func (s *fakeSpan) End()                    { s.ended = true }

type fakeTracer struct {
	mu    sync.Mutex
	spans []*fakeSpan
}

func (t *fakeTracer) Start(ctx context.Context, name string) (context.Context, Span) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := &fakeSpan{name: name, attrs: map[string]any{}}
	t.spans = append(t.spans, s)
	return ctx, s
}

func TestWithTracer_Spans(t *testing.T) {
	tr := &fakeTracer{}
	h := New(WithTracer(tr))
	if err := h.Add("ok", "note", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := h.Add("bad", "", func(context.Context) error { return errors.New("boom") }); err != nil {
		t.Fatal(err)
	}

	h.HandlerHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))

	byName := map[string]*fakeSpan{}
	runCount := 0
	for _, s := range tr.spans {
		if !s.ended {
			t.Errorf("span %q was not ended", s.name)
		}
		if s.name == "healthcheck.run" {
			runCount++
			continue
		}
		byName[s.name] = s
	}

	if runCount != 1 {
		t.Errorf("expected 1 healthcheck.run span, got %d", runCount)
	}
	ok, bad := byName["healthcheck/ok"], byName["healthcheck/bad"]
	if ok == nil || bad == nil {
		t.Fatalf("missing per-check spans; got %v", tr.spans)
	}
	if ok.attrs["healthcheck.status"] != "ok" || len(ok.errs) != 0 {
		t.Errorf("ok span wrong: status=%v errs=%d", ok.attrs["healthcheck.status"], len(ok.errs))
	}
	if ok.attrs["healthcheck.check"] != "ok" || ok.attrs["healthcheck.notes"] != "note" {
		t.Errorf("ok span attrs: %v", ok.attrs)
	}
	if bad.attrs["healthcheck.status"] != "error" || len(bad.errs) != 1 {
		t.Errorf("bad span wrong: status=%v errs=%d", bad.attrs["healthcheck.status"], len(bad.errs))
	}
}

func TestWithTracer_NopDefault(t *testing.T) {
	h := New() // default no-op tracer
	if err := h.Add("db", "", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Must not panic with the default no-op tracer.
	h.HandlerHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, HandlerHealthCheck, nil))
}
