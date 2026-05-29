package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/AOzhogin/healthcheck"
	"golang.org/x/sys/unix"
)

const (
	listenPort = ":8083"
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)

	hc := healthcheck.New(
		healthcheck.WithHTTPAddress(listenPort),
		healthcheck.WithBackCheck(2*time.Second),
		healthcheck.WithContext(ctx),
	)

	// A dependency check that the readiness/startup defaults would run.
	if err := hc.Add("db", "db.company:5432", func(ctx context.Context) error {
		return nil // replace with a real dependency ping
	}); err != nil {
		panic(err)
	}

	// started flips to true once warmup completes; gates the startup probe.
	var started atomic.Bool
	go func() {
		<-time.After(5 * time.Second)
		started.Store(true)
		log.Println("warmup complete")
	}()

	// Liveness: deliberately cheap and dependency-free. A DB blip must NOT restart the pod,
	// so we only report that the process itself is alive.
	hc.SetHandlerLive(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Startup: gate on warmup. While starting, return 503 so the kubelet keeps waiting
	// (and holds off liveness/readiness) instead of restarting a slow-booting pod.
	hc.SetHandlerStartup(func(w http.ResponseWriter, _ *http.Request) {
		if started.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	// Readiness: left unset on purpose — it falls back to the default handler, which runs
	// the registered checks (like /health). Set it via hc.SetHandlerReady(...) to customize.

	hc.Start()
	defer hc.Shutdown()

	go func() {
		log.Printf("server started on %s (/live cheap, /startup gated, /ready = checks)", listenPort)
		if err := hc.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("server stopped")
}
