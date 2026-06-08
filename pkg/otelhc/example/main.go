package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AOzhogin/healthcheck"
	"github.com/AOzhogin/healthcheck/pkg/otelhc"
	"go.opentelemetry.io/otel"
)

const listenPort = ":8086"

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Configure your OpenTelemetry TracerProvider here (SDK + an exporter such as OTLP or stdout)
	// and register it via otel.SetTracerProvider(tp). Without one, otel.GetTracerProvider() returns
	// a no-op provider: spans are created but not exported — this example still shows the wiring.
	tp := otel.GetTracerProvider()

	hc := healthcheck.New(
		healthcheck.WithHTTPAddress(listenPort),
		healthcheck.WithTracer(otelhc.New(tp)), // a span per check; the span ctx flows into each HCFunc
		healthcheck.WithBackCheck(2*time.Second),
		healthcheck.WithContext(ctx),
	)

	if err := hc.Add("db", "db.company:5432", func(ctx context.Context) error {
		// ctx carries the check's span — instrumented clients (DB, HTTP) nest their spans under it.
		<-time.After(20 * time.Millisecond)
		return nil
	}); err != nil {
		panic(err)
	}

	hc.Start()
	defer hc.Shutdown()

	go func() {
		log.Printf("server started on %s", listenPort)
		if err := hc.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("server stopped")
}
