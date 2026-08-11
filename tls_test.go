package healthcheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeSelfSigned writes a self-signed cert/key PEM pair to the given paths. cn ends up in the leaf
// subject so a test can tell one generation of the certificate from the next.
func writeSelfSigned(t *testing.T, certPath, keyPath, cn string, notAfter time.Time) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "EC PRIVATE KEY", keyDER)
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
	require.NoError(t, f.Close())
}

// newSelfSignedPair writes tls.crt/tls.key into dir and returns their paths.
func newSelfSignedPair(t *testing.T, dir, cn string, notAfter time.Time) (certPath, keyPath string) {
	t.Helper()

	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")
	writeSelfSigned(t, certPath, keyPath, cn, notAfter)

	return certPath, keyPath
}

func TestCertReloader_LoadsKeypair(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)

	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert.Leaf, "Leaf must be populated so callers can read NotAfter without re-parsing")
	require.Equal(t, "first", cert.Leaf.Subject.CommonName)
}

func TestCertReloader_MissingFilesFailFast(t *testing.T) {
	dir := t.TempDir()

	_, err := newCertReloader(filepath.Join(dir, "nope.crt"), filepath.Join(dir, "nope.key"), nil, nil, 0)
	require.Error(t, err)
}

func TestCertReloader_MismatchedPairFailsFast(t *testing.T) {
	dir := t.TempDir()
	certPath, _ := newSelfSignedPair(t, dir, "cert-owner", time.Now().Add(24*time.Hour))

	otherDir := t.TempDir()
	_, otherKey := newSelfSignedPair(t, otherDir, "key-owner", time.Now().Add(24*time.Hour))

	_, err := newCertReloader(certPath, otherKey, nil, nil, 0)
	require.Error(t, err, "a cert that does not match the key must not be accepted")
}

func TestCertReloader_ReloadIfChanged_PicksUpNewCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)

	writeSelfSigned(t, certPath, keyPath, "second", time.Now().Add(24*time.Hour))
	r.reloadIfChanged()

	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)
	require.Equal(t, "second", cert.Leaf.Subject.CommonName)
}

func TestCertReloader_ReloadIfChanged_IgnoresUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)

	before, err := r.GetCertificate(nil)
	require.NoError(t, err)

	r.reloadIfChanged()

	after, err := r.GetCertificate(nil)
	require.NoError(t, err)
	require.Same(t, before, after, "unchanged files must not cause a new certificate to be built")
}

func TestCertReloader_ReloadIfChanged_KeepsOldCertOnGarbage(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(certPath, []byte("not a pem file"), 0o600))
	r.reloadIfChanged()

	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)
	require.Equal(t, "first", cert.Leaf.Subject.CommonName, "a broken file must never evict a working keypair")
}

// A torn update — the certificate replaced while the key is still the old one — must be retried, not
// remembered as the current state.
func TestCertReloader_ReloadIfChanged_RecoversAfterTornWrite(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)

	stagingDir := t.TempDir()
	newCert, newKey := newSelfSignedPair(t, stagingDir, "second", time.Now().Add(24*time.Hour))

	// Only the certificate lands first: it no longer matches the key on disk.
	copyFile(t, newCert, certPath)
	r.reloadIfChanged()

	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)
	require.Equal(t, "first", cert.Leaf.Subject.CommonName)

	// The key catches up; the next pass must succeed.
	copyFile(t, newKey, keyPath)
	r.reloadIfChanged()

	cert, err = r.GetCertificate(nil)
	require.NoError(t, err)
	require.Equal(t, "second", cert.Leaf.Subject.CommonName)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	b, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, b, 0o600))
}

func TestFingerprint_ChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	before, err := fingerprint(certPath, keyPath)
	require.NoError(t, err)

	again, err := fingerprint(certPath, keyPath)
	require.NoError(t, err)
	require.Equal(t, before, again, "unchanged files must produce the same fingerprint")

	writeSelfSigned(t, certPath, keyPath, "second", time.Now().Add(24*time.Hour))

	after, err := fingerprint(certPath, keyPath)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

// writeProjectedSecret lays dir out the way kubelet lays out a projected secret volume:
//
//	dir/..gen1/tls.crt          real files, one directory per generation
//	dir/..data -> ..gen1        symlink, replaced atomically on update
//	dir/tls.crt -> ..data/tls.crt   the stable path the application opens
//
// This is exactly what a cert-manager rotation looks like inside a pod, and it is why the watcher
// has to watch the directory instead of the files.
func writeProjectedSecret(t *testing.T, dir, generation, cn string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the projected-secret layout relies on symlinks, which need extra privileges on Windows")
	}

	genDir := filepath.Join(dir, ".."+generation)
	require.NoError(t, os.MkdirAll(genDir, 0o755))
	writeSelfSigned(t, filepath.Join(genDir, "tls.crt"), filepath.Join(genDir, "tls.key"), cn, time.Now().Add(24*time.Hour))

	// kubelet creates ..data_tmp and renames it over ..data; rename is atomic within a filesystem.
	tmpLink := filepath.Join(dir, "..data_tmp")
	require.NoError(t, os.Symlink(".."+generation, tmpLink))
	require.NoError(t, os.Rename(tmpLink, filepath.Join(dir, "..data")))

	for _, name := range []string{"tls.crt", "tls.key"} {
		link := filepath.Join(dir, name)
		if _, err := os.Lstat(link); err == nil {
			continue // the stable symlink survives generation swaps
		}
		require.NoError(t, os.Symlink(filepath.Join("..data", name), link))
	}
}

// servedCN returns the common name of the certificate the reloader currently holds.
func servedCN(t *testing.T, r *certReloader) string {
	t.Helper()

	cert, err := r.GetCertificate(nil)
	require.NoError(t, err)

	return cert.Leaf.Subject.CommonName
}

// requireClosesWithin asserts that close returns inside d. close waits on the watch goroutine, so a
// goroutine that ignores the stop signal shows up here as a precise failure instead of hanging the
// package until the go test timeout.
func requireClosesWithin(t *testing.T, r *certReloader, d time.Duration) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- r.close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(d):
		t.Fatal("close did not return: the watch goroutine is still running")
	}
}

func TestCertReloader_WatchPicksUpFileRewrite(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, r.start(context.Background()))
	defer requireClosesWithin(t, r, 2*time.Second)

	writeSelfSigned(t, certPath, keyPath, "second", time.Now().Add(24*time.Hour))

	require.Eventually(t, func() bool { return servedCN(t, r) == "second" }, 5*time.Second, 20*time.Millisecond)
}

func TestCertReloader_WatchPicksUpProjectedSecretSwap(t *testing.T) {
	dir := t.TempDir()
	writeProjectedSecret(t, dir, "gen1", "first")

	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, r.start(context.Background()))
	defer requireClosesWithin(t, r, 2*time.Second)

	require.Equal(t, "first", servedCN(t, r))

	writeProjectedSecret(t, dir, "gen2", "second")

	require.Eventually(t, func() bool { return servedCN(t, r) == "second" }, 5*time.Second, 20*time.Millisecond)
}

// The periodic rescan must catch a change even when no filesystem event ever arrives.
func TestCertReloader_PeriodicRescanCatchesMissedEvents(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 50*time.Millisecond)
	require.NoError(t, err)
	require.NoError(t, r.start(context.Background()))
	defer requireClosesWithin(t, r, 2*time.Second)

	// Write outside the watched directory and move the result in with a plain rename; whatever events
	// this produces, the ticker alone is enough to notice.
	staging := t.TempDir()
	newCert, newKey := newSelfSignedPair(t, staging, "second", time.Now().Add(24*time.Hour))
	copyFile(t, newCert, certPath)
	copyFile(t, newKey, keyPath)

	require.Eventually(t, func() bool { return servedCN(t, r) == "second" }, 5*time.Second, 20*time.Millisecond)
}

// On a platform without an fsnotify backend (plan9, js/wasm, appengine) NewWatcher fails at runtime.
// The reloader must keep working off the periodic rescan instead of refusing to serve.
func TestCertReloader_PollOnlyFallback(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 50*time.Millisecond)
	require.NoError(t, err)

	// No watcher at all — exactly the state start() falls back to.
	r.startLoop(context.Background())
	defer requireClosesWithin(t, r, 2*time.Second)

	writeSelfSigned(t, certPath, keyPath, "second", time.Now().Add(24*time.Hour))

	require.Eventually(t, func() bool { return servedCN(t, r) == "second" }, 5*time.Second, 20*time.Millisecond)
}

// Falling back with the periodic rescan switched off would leave nothing driving reloads at all.
func TestCertReloader_FallbackForcesAnInterval(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	r, err := newCertReloader(certPath, keyPath, nil, nil, 0)
	require.NoError(t, err)

	r.fallbackToPolling(errors.New("fsnotify not supported on the current platform"))

	require.Equal(t, defaultTLSReloadInterval, r.interval)
}

// Cancelling the context must stop the watch goroutine. close waits for it, so if the goroutine
// ignored the cancellation this test fails on the timeout instead of passing silently.
func TestCertReloader_ContextCancelStopsWatcher(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	ctx, cancel := context.WithCancel(context.Background())

	r, err := newCertReloader(certPath, keyPath, nil, nil, time.Minute)
	require.NoError(t, err)
	require.NoError(t, r.start(ctx))

	cancel()

	requireClosesWithin(t, r, 2*time.Second)
	requireClosesWithin(t, r, 2*time.Second) // close must be idempotent
}
