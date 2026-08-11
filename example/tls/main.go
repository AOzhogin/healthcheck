package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/AOzhogin/healthcheck"
	"golang.org/x/sys/unix"
)

const (
	listenPort = ":8443"
	certFile   = "/etc/tls/tls.crt"
	keyFile    = "/etc/tls/tls.key"
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)

	hc := healthcheck.New(
		healthcheck.WithContext(ctx),
		healthcheck.WithHTTPAddress(listenPort),
		// The keypair is re-read whenever the mounted secret changes, so a cert-manager rotation is
		// picked up by the next handshake without restarting this process.
		healthcheck.WithTLS(certFile, keyFile),
		healthcheck.WithMetrics(false, false, false),
	)

	// Fail readiness if the certificate ever stops rotating and drifts towards expiry.
	if err := hc.Add("tls-cert", "serving certificate", healthcheck.CertExpiryCheck(certFile, 14*24*time.Hour)); err != nil {
		log.Fatalf("add check: %v", err)
	}

	hc.Start()
	defer hc.Shutdown()

	go func() {
		if err := hc.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	log.Printf("https server started on %s (cert: %s)", listenPort, certFile)
	<-ctx.Done()
	log.Println("server stopped")
}
