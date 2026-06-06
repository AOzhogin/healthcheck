// Package otelhc adapts OpenTelemetry tracing to the healthcheck.Tracer interface.
//
// It lives in a separate module so the core healthcheck library keeps NO OpenTelemetry
// dependency: only import this package if you want OTel spans for checks.
//
//	hc := healthcheck.New(
//	    healthcheck.WithTracer(otelhc.New(otel.GetTracerProvider())),
//	)
package otelhc

import (
	"context"
	"fmt"

	"github.com/AOzhogin/healthcheck"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/AOzhogin/healthcheck"

// New returns a healthcheck.Tracer backed by the given OpenTelemetry TracerProvider.
func New(tp oteltrace.TracerProvider) healthcheck.Tracer {
	return tracer{t: tp.Tracer(instrumentationName)}
}

type tracer struct{ t oteltrace.Tracer }

func (a tracer) Start(ctx context.Context, name string) (context.Context, healthcheck.Span) {
	ctx, s := a.t.Start(ctx, name)
	return ctx, span{s: s}
}

type span struct{ s oteltrace.Span }

func (sp span) SetAttr(key string, value any) {
	sp.s.SetAttributes(toKeyValue(key, value))
}

func (sp span) RecordError(err error) {
	sp.s.RecordError(err)
	sp.s.SetStatus(codes.Error, err.Error())
}

func (sp span) End() { sp.s.End() }

func toKeyValue(key string, value any) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case bool:
		return attribute.Bool(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}
