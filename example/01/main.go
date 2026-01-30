package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/AOzhogin/healthcheck"
	"golang.org/x/sys/unix"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/AOzhogin/healthcheck"
)

const (
	listenPort = ":8080"
)

func main() {

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)

	hc := healthcheck.New(
		healthcheck.WithMetrics(false, false, true),
		healthcheck.WithBackCheck(5*time.Second),
		healthcheck.WithContext(ctx),
	)
	
	if err := hc.Add("db", "db.company:1521", func(ctx context.Context) error {
		<-time.After(time.Duration(rand.Intn(999)) * time.Millisecond)
		return nil
	}); err != nil {
		panic(err)
	}

	if err := hc.Add("redis", "redis.company:9056", func(ctx context.Context) error {
		<-time.After(time.Duration(rand.Intn(999)) * time.Millisecond)
		return nil
	}); err != nil {
		panic(err)
	}

	if err := hc.Add("oracle11g", "oracle.company", func(ctx context.Context) error {
		<-time.After(time.Duration(rand.Intn(999)) * time.Millisecond)
		return nil
	}); err != nil {
		panic(err)
	}

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
	mux.HandleFunc(healthcheck.HandlerMetrics, hc.HandlerMetrics)

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

		println(fmt.Sprintf("sever stopped"))

	}()

	select {
	case <-ctx.Done():
		{
			if err = srv.Shutdown(context.Background()); err != nil {
				println(fmt.Errorf("sever shutdown: %w", err))
			}
			return
		}
	}

}
