package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/AOzhogin/healthcheck"
	"golang.org/x/sys/unix"
)

const (
	listenPort = ":8082"
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)

	// Custom middleware: logs request path and adds a header for diagnostics.
	customMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("[middleware] %s %s", r.Method, r.URL.Path)
			w.Header().Set("X-Custom-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	}

	hc := healthcheck.New(
		healthcheck.WithPort(listenPort),
		healthcheck.WithMiddleware(customMiddleware),
		healthcheck.WithBasicAuth("admin", "secret"),
		healthcheck.WithMetrics(false, false, false),
	)

	hc.Start()
	defer hc.Shutdown()

	go func() {
		if err := hc.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	log.Printf("server started on %s (Basic Auth: admin/secret)", listenPort)
	<-ctx.Done()
	log.Println("server stopped")
}
