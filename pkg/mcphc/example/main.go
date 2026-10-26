package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AOzhogin/healthcheck"
	"github.com/AOzhogin/healthcheck/pkg/mcphc"
)

const listenAddr = ":8080"

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Demo credentials only: take real ones from a secret and never ship the defaults.
	user := getenv("HC_USER", "admin")
	pass := getenv("HC_PASS", "admin")

	hc := healthcheck.New(
		healthcheck.WithHTTPAddress(listenAddr),
		healthcheck.WithBasicAuth(user, pass), // protects /health, /metrics and /mcp
		healthcheck.WithBackCheck(5*time.Second),
		healthcheck.WithMCP(mcphc.New()), // serves the MCP endpoint at /mcp
		healthcheck.WithContext(ctx),
	)

	if err := hc.Add("db", "db.company:5432", func(ctx context.Context) error {
		return nil
	}); err != nil {
		panic(err)
	}

	// "cache" fails on every second call, so the agent has something to diagnose.
	var calls atomic.Int64
	if err := hc.Add("cache", "cache.company:6379", func(ctx context.Context) error {
		if calls.Add(1)%2 == 0 {
			return errors.New("cache: connection refused")
		}
		return nil
	}); err != nil {
		panic(err)
	}

	hc.Start()
	defer hc.Shutdown()

	go func() {
		log.Printf("server started on %s, MCP at %s", listenAddr, healthcheck.HandlerMCP)
		if err := hc.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("server stopped")
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
