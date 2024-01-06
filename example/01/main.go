package main

import (
	"context"
	"github.com/AOzhogin/healthcheck"
	"math/rand"
	"net/http"
	"time"
)

func main() {

	hc := healthcheck.New(healthcheck.WithMetrics(false, false, true), healthcheck.WithBackCheck(5*time.Second))
	hc.Add("db", "db.company:1521", func(ctx context.Context) error {
		<-time.After(time.Duration(rand.Intn(999)) * time.Millisecond)
		return nil
	})

	err := hc.Add("redis", "redis.company:9056", func(ctx context.Context) error {
		<-time.After(time.Duration(rand.Intn(999)) * time.Millisecond)
		return nil
	})

	if err != nil {
		panic(err)
	}

	err = hc.Add("oracle11g", "oracle.company", func(ctx context.Context) error {
		<-time.After(time.Duration(rand.Intn(999)) * time.Millisecond)
		return nil
	})

	if err != nil {
		panic(err)
	}

	hc.Start()
	defer hc.Shutdown()

	mux := http.NewServeMux()

	mux.HandleFunc(healthcheck.HandlerHealthCheck, hc.HandlerHealth)
	mux.HandleFunc(healthcheck.HandlerMetrics, hc.HandlerMetrics)

	http.ListenAndServe(":8080", mux)

}
