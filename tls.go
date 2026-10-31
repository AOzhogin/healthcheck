package healthcheck

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// defaultTLSReloadInterval is the safety-net rescan period used when the caller does not set one.
	defaultTLSReloadInterval = 5 * time.Minute
	// tlsWatchDebounce collapses the burst of events a rotation produces into a single reload.
	tlsWatchDebounce = 100 * time.Millisecond
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
	debounce time.Duration

	// watcher is nil in rescan-only mode, and then events/errs are nil too.
	watcher  *fsnotify.Watcher
	events   chan fsnotify.Event
	errs     chan error
	done     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
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
		debounce: tlsWatchDebounce,
		done:     make(chan struct{}),
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

// start begins watching the directories that hold the certificate and the key.
//
// Directories, not the files themselves: Kubernetes updates a projected secret by writing a new
// ..data directory and renaming the ..data symlink over the old one, which replaces the inode behind
// the file and silently kills a file-level watch. A periodic rescan runs alongside the watcher as a
// safety net for events lost on remounts or on filesystems with unreliable notifications.
//
// fsnotify has a native backend for Linux (inotify), Windows (ReadDirectoryChangesW), macOS and BSD
// (kqueue) and illumos (fen). Everywhere else NewWatcher fails at runtime; there the reloader falls
// back to the periodic rescan alone instead of refusing to serve HTTPS.
func (c *certReloader) start(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		c.fallbackToPolling(err)
		c.startLoop(ctx)

		return nil
	}

	for _, dir := range c.watchDirs() {
		if err := w.Add(dir); err != nil {
			_ = w.Close()
			return fmt.Errorf("watch %s: %w", dir, err)
		}
	}

	c.watcher = w
	c.events, c.errs = w.Events, w.Errors
	c.startLoop(ctx)

	return nil
}

// fallbackToPolling switches the reloader to rescan-only mode. The interval is forced to a usable
// value: without a watcher it is the only thing left driving reloads.
func (c *certReloader) fallbackToPolling(err error) {
	if c.interval <= 0 {
		c.interval = defaultTLSReloadInterval
	}

	if c.logger != nil {
		c.logger.Warn("healthcheck: tls: filesystem watching unavailable, falling back to a periodic rescan",
			slog.String("error", err.Error()),
			slog.Duration("interval", c.interval),
		)
	}
}

// startLoop launches the single goroutine driving reloads.
func (c *certReloader) startLoop(ctx context.Context) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.watch(ctx)
	}()
}

// watchDirs returns the unique directories holding the certificate and the key.
func (c *certReloader) watchDirs() []string {
	dirs := []string{filepath.Dir(c.certFile)}
	if d := filepath.Dir(c.keyFile); d != dirs[0] {
		dirs = append(dirs, d)
	}

	return dirs
}

// watch is the single goroutine driving reloads. It exits on context cancellation or on close.
// In rescan-only mode c.events and c.errs are nil, and a nil channel simply never fires in a select,
// so the same loop covers both modes.
func (c *certReloader) watch(ctx context.Context) {
	var tick <-chan time.Time
	if c.interval > 0 {
		t := time.NewTicker(c.interval)
		defer t.Stop()
		tick = t.C
	}

	// debounce is armed by every event and fires once the burst settles. Go 1.23+ collects an
	// abandoned time.After timer, so re-arming on each event does not pile up.
	var debounce <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return

		case <-c.done:
			return

		case _, ok := <-c.events:
			if !ok {
				return
			}
			debounce = time.After(c.debounce)

		case <-debounce:
			debounce = nil
			c.rearm()
			c.reloadIfChanged()

		case <-tick:
			c.rearm()
			c.reloadIfChanged()

		case err, ok := <-c.errs:
			if !ok {
				return
			}
			if c.logger != nil {
				c.logger.Error("healthcheck: tls: watcher error", slog.String("error", err.Error()))
			}
		}
	}
}

// rearm re-adds the directory watches. After a ..data swap or a remount the original watch can be
// gone; Add on an already watched path is a cheap no-op.
func (c *certReloader) rearm() {
	if c.watcher == nil {
		return // rescan-only mode: nothing to re-arm
	}

	for _, dir := range c.watchDirs() {
		if err := c.watcher.Add(dir); err != nil && c.logger != nil {
			c.logger.Error("healthcheck: tls: re-adding the watch failed",
				slog.String("dir", dir),
				slog.String("error", err.Error()),
			)
		}
	}
}

// close stops the watch loop and releases the watcher. It is safe to call more than once.
func (c *certReloader) close() error {
	var err error

	c.stopOnce.Do(func() {
		close(c.done)
		if c.watcher != nil {
			err = c.watcher.Close()
		}
	})
	c.wg.Wait()

	return err
}

// leaf returns the leaf certificate currently served, or nil when no keypair is loaded.
func (c *certReloader) leaf() *x509.Certificate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cert == nil {
		return nil
	}

	return c.cert.Leaf
}
