package healthcheck

import "context"

// Tracer starts a span for each check execution. The core library has NO tracing dependency:
// implement this with an adapter (e.g. the otelhc OpenTelemetry adapter) and pass it via
// WithTracer. The default is a no-op with zero overhead.
//
// Start returns a child context carrying the span; the library passes that context to the
// HCFunc, so user check code can nest its own spans under it.
type Tracer interface {
	Start(ctx context.Context, name string) (context.Context, Span)
}

// Span is the minimal span surface the library uses. End must be safe to call once.
type Span interface {
	// SetAttr attaches a key/value attribute to the span.
	SetAttr(key string, value any)
	// RecordError marks the span as errored and records the error.
	RecordError(err error)
	// End finishes the span.
	End()
}

// nopTracer is the default Tracer: it does nothing and allocates no span.
type nopTracer struct{}

func (nopTracer) Start(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) SetAttr(string, any) {}
func (nopSpan) RecordError(error)   {}
func (nopSpan) End()                {}
