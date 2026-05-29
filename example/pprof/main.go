package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/AOzhogin/healthcheck"
	"golang.org/x/sys/unix"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const (
	listenPort = ":8080"
)

func main() {

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)

	hc := healthcheck.New(
		healthcheck.WithMetrics(false, false, true),
		healthcheck.WithBackCheck(5*time.Second),
	)

	hc.Start()
	defer hc.Shutdown()

	Serve(ctx, hc)

}

func Serve(ctx context.Context, hc healthcheck.HealthCheck) {

	var (
		err error
	)

	mux := http.NewServeMux()

	mux.HandleFunc(healthcheck.HandlerHealthCheck, hc.HandlerHealth)

	mux.HandleFunc(healthcheck.HandlerDebug, hc.HandlerPProf)

	srv := http.Server{
		Handler: mux,
		Addr:    listenPort,
	}

	go func() {

		println(fmt.Sprintf("sever started: %s", listenPort))

		if err = srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			println(fmt.Errorf("sever stopped: %w", err))
			return
		}

		println("server stopped")

	}()

	<-ctx.Done()
	if err = srv.Shutdown(context.Background()); err != nil {
		println(fmt.Errorf("server shutdown: %w", err))
	}
}
