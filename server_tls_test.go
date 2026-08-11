package healthcheck

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startTLSServer boots StartHTTPServer for h in a goroutine and waits until the TLS port answers.
func startTLSServer(t *testing.T, h *healthCheck, addr string) {
	t.Helper()

	go func() {
		if err := h.StartHTTPServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server stopped: %v", err)
		}
	}()

	require.Eventually(t, func() bool {
		conn, err := tls.Dial("tcp", "127.0.0.1"+addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test client
		if err != nil {
			return false
		}
		_ = conn.Close()

		return true
	}, 5*time.Second, 20*time.Millisecond)
}

// servedCNOverWire opens a fresh TLS connection — no pooling, so every call re-handshakes — and
// returns the common name of the certificate the server presented.
func servedCNOverWire(t *testing.T, addr string) string {
	t.Helper()

	conn, err := tls.Dial("tcp", "127.0.0.1"+addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test client inspects the presented cert
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	return conn.ConnectionState().PeerCertificates[0].Subject.CommonName
}

func TestHTTPServer_TLS_ServesHTTPS(t *testing.T) {
	const addr = ":18100"

	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	h := New(WithHTTPAddress(addr), WithTLS(certPath, keyPath))
	startTLSServer(t, h, addr)

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed test cert
		DisableKeepAlives: true,
	}}

	resp, err := client.Get("https://127.0.0.1" + addr + HandlerHealthCheck)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPServer_TLS_HotReloadWithoutRestart(t *testing.T) {
	const addr = ":18101"

	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	h := New(WithHTTPAddress(addr), WithTLS(certPath, keyPath))
	startTLSServer(t, h, addr)

	require.Equal(t, "first", servedCNOverWire(t, addr))

	writeSelfSigned(t, certPath, keyPath, "second", time.Now().Add(24*time.Hour))

	require.Eventually(t, func() bool { return servedCNOverWire(t, addr) == "second" }, 5*time.Second, 50*time.Millisecond)
}

func TestHTTPServer_TLS_HotReloadOnProjectedSecretSwap(t *testing.T) {
	const addr = ":18102"

	dir := t.TempDir()
	writeProjectedSecret(t, dir, "gen1", "first")

	h := New(WithHTTPAddress(addr), WithTLS(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")))
	startTLSServer(t, h, addr)

	require.Equal(t, "first", servedCNOverWire(t, addr))

	writeProjectedSecret(t, dir, "gen2", "second")

	require.Eventually(t, func() bool { return servedCNOverWire(t, addr) == "second" }, 5*time.Second, 50*time.Millisecond)
}

func TestHTTPServer_TLS_KeepsServingWhenRotationIsBroken(t *testing.T) {
	const addr = ":18103"

	dir := t.TempDir()
	certPath, keyPath := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	h := New(WithHTTPAddress(addr), WithTLS(certPath, keyPath))
	startTLSServer(t, h, addr)

	require.NoError(t, os.WriteFile(certPath, []byte("garbage"), 0o600))
	time.Sleep(500 * time.Millisecond) // let the watcher process the event

	require.Equal(t, "first", servedCNOverWire(t, addr), "a broken rotation must not take the endpoint down")
}

func TestHTTPServer_TLS_MissingFilesFailFast(t *testing.T) {
	dir := t.TempDir()

	h := New(WithHTTPAddress(":18104"), WithTLS(filepath.Join(dir, "nope.crt"), filepath.Join(dir, "nope.key")))

	err := h.StartHTTPServer()
	require.Error(t, err)
	require.Contains(t, err.Error(), "tls")
}

func TestHTTPServer_TLS_OnlyOneFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	certPath, _ := newSelfSignedPair(t, dir, "first", time.Now().Add(24*time.Hour))

	h := New(WithHTTPAddress(":18105"), WithTLS(certPath, ""))

	err := h.StartHTTPServer()
	require.Error(t, err)
	require.Contains(t, err.Error(), "key file")
}
