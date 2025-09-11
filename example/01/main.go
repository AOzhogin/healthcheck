package main

import (
	"context"
	"math/rand"
	"net/http"
	"time"

	"github.com/AOzhogin/healthcheck"
)

func main() {

	hc := healthcheck.New(healthcheck.WithMetrics(false, false, true), healthcheck.WithBackCheck(5*time.Second))
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

	mux := http.NewServeMux()

	mux.HandleFunc(healthcheck.HandlerHealthCheck, hc.HandlerHealth)
	mux.HandleFunc(healthcheck.HandlerMetrics, hc.HandlerMetrics)

	if err := http.ListenAndServe(":8080", mux); err != http.ErrServerClosed {
		panic(err)
	}
}
