package healthcheck

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

// certReloader holds the TLS keypair currently served and swaps it atomically when the files on disk
// change. It is wired into tls.Config.GetCertificate, so every new handshake picks up the latest
// keypair without restarting the process or the listener.
type certReloader struct {
	certFile string
	keyFile  string

	mu   sync.RWMutex
	cert *tls.Certificate

	// fp is the hash of the files behind the currently loaded keypair. It is only touched during the
	// initial load and from the single watch goroutine, so it needs no lock of its own.
	fp [sha256.Size]byte

	logger  *slog.Logger // optional
	metrics *tlsMetrics  // optional; all its methods are nil-safe

	interval time.Duration // safety-net rescan interval; 0 disables the periodic rescan
}

// newCertReloader loads the keypair once and fails if it cannot be used, so a wrong path or an
// unreadable certificate surfaces at startup instead of at the first handshake.
func newCertReloader(certFile, keyFile string, logger *slog.Logger, m *tlsMetrics, interval time.Duration) (*certReloader, error) {
	c := &certReloader{
		certFile: certFile,
		keyFile:  keyFile,
		logger:   logger,
		metrics:  m,
		interval: interval,
	}

	if err := c.reload(); err != nil {
		return nil, err
	}

	return c, nil
}

// GetCertificate is the tls.Config hook: it returns the keypair currently held in memory.
func (c *certReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cert == nil {
		return nil, errors.New("healthcheck: tls: no certificate loaded")
	}

	return c.cert, nil
}

// reload reads both files and swaps the keypair in. The fingerprint is updated only on success, so a
// torn write (certificate replaced before the key) is retried on the next event instead of being
// remembered as the current state.
func (c *certReloader) reload() error {
	fp, err := fingerprint(c.certFile, c.keyFile)
	if err != nil {
		return err
	}

	cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return err
	}

	// Go 1.23+ populates Leaf itself; parse defensively so NotAfter is always available.
	if cert.Leaf == nil {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return err
		}
		cert.Leaf = leaf
	}

	c.mu.Lock()
	c.cert = &cert
	c.mu.Unlock()
	c.fp = fp

	c.metrics.certLoaded(cert.Leaf.NotAfter)

	if c.logger != nil {
		c.logger.Info("healthcheck: tls: certificate loaded",
			slog.String("cert", c.certFile),
			slog.String("subject", cert.Leaf.Subject.CommonName),
			slog.Time("not_after", cert.Leaf.NotAfter),
		)
	}

	return nil
}

// reloadIfChanged is the watch-loop entry point. It skips events that did not change the bytes on
// disk and never replaces a working keypair with a broken one: a failed load is logged and counted,
// and the previous certificate keeps being served until a valid pair appears.
func (c *certReloader) reloadIfChanged() {
	if fp, err := fingerprint(c.certFile, c.keyFile); err == nil && fp == c.fp {
		return
	}

	if err := c.reload(); err != nil {
		c.metrics.reloadFailed()

		if c.logger != nil {
			c.logger.Error("healthcheck: tls: certificate reload failed, keeping the previous keypair",
				slog.String("cert", c.certFile),
				slog.String("error", err.Error()),
			)
		}

		return
	}

	c.metrics.reloadSucceeded()
}

// fingerprint hashes both files so identical content — a rewritten but unchanged secret, or a
// duplicate filesystem event — does not trigger a pointless reload.
func fingerprint(files ...string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte

	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return sum, err
		}
		_, _ = h.Write(b)
	}
	copy(sum[:], h.Sum(nil))

	return sum, nil
}
