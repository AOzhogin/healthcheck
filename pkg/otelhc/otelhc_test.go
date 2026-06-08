package otelhc

import (
	"context"
	"errors"
	"testing"

	"github.com/AOzhogin/healthcheck"
	"go.opentelemetry.io/otel/trace/noop"
)

// Compile-time interface conformance.
var (
	_ healthcheck.Tracer = tracer{}
	_ healthcheck.Span   = span{}
)

func TestAdapter_ImplementsAndRuns(t *testing.T) {
	tr := New(noop.NewTracerProvider())

	ctx, sp := tr.Start(context.Background(), "healthcheck/db")
	if ctx == nil {
		t.Fatal("Start returned nil context")
	}

	// Exercise every Span method (no panic) across attribute types and error path.
	sp.SetAttr("healthcheck.check", "db")
	sp.SetAttr("healthcheck.notes", "db.company:5432")
	sp.SetAttr("healthcheck.duration_ms", int64(5))
	sp.SetAttr("healthcheck.status", "error")
	sp.SetAttr("custom.any", struct{ X int }{1})
	sp.RecordError(errors.New("boom"))
	sp.End()
}
