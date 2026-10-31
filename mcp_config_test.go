package healthcheck

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeMCPConfig(t *testing.T, h *healthCheck) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(h.mcpConfig(), &m))

	return m
}

func TestMCPConfig_Defaults(t *testing.T) {
	m := decodeMCPConfig(t, New())

	assert.Equal(t, ":8080", m["http_addr"])
	assert.Equal(t, "30s", m["timeout"])
	assert.Equal(t, map[string]any{"enabled": false, "interval": "0s"}, m["background_check"])
	assert.Equal(t, map[string]any{"success": float64(200), "error": float64(503)}, m["status_codes"])
	assert.Equal(t, map[string]any{"success": "ok", "error": "error"}, m["check_status"])
	assert.Equal(t, false, m["basic_auth"])
	assert.Equal(t, false, m["custom_middleware"])
	assert.Equal(t, true, m["pprof"])
	assert.Equal(t, map[string]any{"enabled": false}, m["metrics"])
	assert.Equal(t, false, m["tracer"])
	assert.Equal(t, false, m["logger"])
	assert.Equal(t, map[string]any{"live": false, "ready": false, "startup": false}, m["custom_handlers"])
	assert.Equal(t, []any{}, m["checks"])

	tlsBlock, ok := m["tls"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, tlsBlock["enabled"])
	assert.Equal(t, map[string]any{"loaded": false}, tlsBlock["certificate"])
}

func TestMCPConfig_ReflectsOptions(t *testing.T) {
	h := New(
		WithHTTPAddress(":9000"),
		WithTimeOut(2*time.Second),
		WithBackCheck(10*time.Second),
		WithoutPProf(),
		WithMetrics(false, false, false),
		WithLogger(slog.New(slog.DiscardHandler)),
		WithTracer(&fakeTracer{}),
	)
	h.SetHandlerReady(func(http.ResponseWriter, *http.Request) {})

	m := decodeMCPConfig(t, h)

	assert.Equal(t, ":9000", m["http_addr"])
	assert.Equal(t, "2s", m["timeout"])
	assert.Equal(t, map[string]any{"enabled": true, "interval": "10s"}, m["background_check"])
	assert.Equal(t, false, m["pprof"])
	assert.Equal(t, map[string]any{"enabled": true}, m["metrics"])
	assert.Equal(t, true, m["logger"])
	assert.Equal(t, true, m["tracer"])
	assert.Equal(t, map[string]any{"live": false, "ready": true, "startup": false}, m["custom_handlers"])
}

func TestMCPConfig_RedactsSecrets(t *testing.T) {
	h := New(WithBasicAuth("admin-user", "s3cret-pass"), WithTLS("/etc/tls/tls.crt", "/secret/tls-private.key"))

	raw := string(h.mcpConfig())
	assert.NotContains(t, raw, "admin-user")
	assert.NotContains(t, raw, "s3cret-pass")
	assert.NotContains(t, raw, "tls-private.key")

	m := decodeMCPConfig(t, h)
	assert.Equal(t, true, m["basic_auth"])

	tlsBlock := m["tls"].(map[string]any)
	assert.Equal(t, "/etc/tls/tls.crt", tlsBlock["cert_file"])
	assert.Equal(t, true, tlsBlock["enabled"])
	assert.Equal(t, false, tlsBlock["certificate"].(map[string]any)["loaded"])
}

func TestMCPConfig_ChecksSortedAndLive(t *testing.T) {
	h := New()
	require.NoError(t, h.Add("b", "notes-b", okCheck))
	require.NoError(t, h.Add("a", "notes-a", okCheck))

	m := decodeMCPConfig(t, h)
	assert.Equal(t, []any{
		map[string]any{"name": "a", "notes": "notes-a"},
		map[string]any{"name": "b", "notes": "notes-b"},
	}, m["checks"])

	h.Start()
	t.Cleanup(h.Shutdown)

	require.NoError(t, h.Add("c", "", okCheck))

	m = decodeMCPConfig(t, h)
	assert.Equal(t, []any{
		map[string]any{"name": "a", "notes": "notes-a"},
		map[string]any{"name": "b", "notes": "notes-b"},
		map[string]any{"name": "c", "notes": ""},
	}, m["checks"])
}

func TestMCPConfig_CurrentCertificateAfterReload(t *testing.T) {
	const addr = ":18200"

	dir := t.TempDir()
	notAfter1 := time.Now().Add(24 * time.Hour)
	certPath, keyPath := newSelfSignedPair(t, dir, "first", notAfter1)

	h := New(WithHTTPAddress(addr), WithTLS(certPath, keyPath))
	startTLSServer(t, h, addr)

	cert := func() map[string]any {
		return decodeMCPConfig(t, h)["tls"].(map[string]any)["certificate"].(map[string]any)
	}

	c := cert()
	assert.Equal(t, true, c["loaded"])
	assert.Equal(t, "first", c["subject"])

	got, err := time.Parse(time.RFC3339, c["not_after"].(string))
	require.NoError(t, err)
	assert.Equal(t, notAfter1.UTC().Truncate(time.Second), got.UTC())

	writeSelfSigned(t, certPath, keyPath, "second", time.Now().Add(48*time.Hour))

	require.Eventually(t, func() bool {
		return cert()["subject"] == "second"
	}, 5*time.Second, 50*time.Millisecond)
}

func TestMCPConfig_CertificateWithoutSANsHasEmptyDNSNamesArray(t *testing.T) {
	const addr = ":18299"

	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "no-sans"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "EC PRIVATE KEY", keyDER)

	h := New(WithHTTPAddress(addr), WithTLS(certPath, keyPath))
	startTLSServer(t, h, addr)

	c := decodeMCPConfig(t, h)["tls"].(map[string]any)["certificate"].(map[string]any)
	assert.Equal(t, true, c["loaded"])
	assert.Equal(t, []any{}, c["dns_names"])
	assert.Contains(t, string(h.mcpConfig()), `"dns_names":[]`)
}

func TestMCPConfig_CertificateNotLoadedAfterServerReturns(t *testing.T) {
	const addr = "127.0.0.1:18203"

	// Occupy the port so StartHTTPServer loads the keypair and then fails to listen.
	ln, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	certPath, keyPath := newSelfSignedPair(t, t.TempDir(), "gone", time.Now().Add(time.Hour))

	h := New(WithHTTPAddress(addr), WithTLS(certPath, keyPath))
	require.Error(t, h.StartHTTPServer())

	assert.Equal(t, map[string]any{"loaded": false}, decodeMCPConfig(t, h)["tls"].(map[string]any)["certificate"])
}
