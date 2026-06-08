package main

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/AOzhogin/healthcheck"
	"golang.org/x/sys/unix"
)

const listenPort = ":8085"

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	hc := healthcheck.New(
		healthcheck.WithHTTPAddress(listenPort),
		healthcheck.WithMetrics(false, false, false),
		// Custom histogram buckets tuned for fast checks; order-independent with WithMetrics.
		healthcheck.WithMetricsBuckets(0.01, 0.05, 0.1, 0.25, 0.5, 1),
		// Logs each check's ok<->error transition (error level on fail, info on recovery).
		healthcheck.WithLogger(logger),
		healthcheck.WithBackCheck(2*time.Second),
		healthcheck.WithContext(ctx),
	)

	// Flaky dependency (~30% failures) so you can watch transition logs and
	// healthcheck_metrics_errors_total{reason} on /metrics.
	if err := hc.Add("flaky-db", "db.company:5432", func(context.Context) error {
		if rand.Intn(10) < 3 {
			return errors.New("connection refused")
		}
		return nil
	}); err != nil {
		panic(err)
	}
	if err := hc.Add("cache", "redis:6379", func(context.Context) error { return nil }); err != nil {
		panic(err)
	}

	hc.Start()
	defer hc.Shutdown()

	go func() {
		logger.Info("server started", "addr", listenPort)
		if err := hc.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
		}
	}()

	<-ctx.Done()
	logger.Info("server stopped")
}
