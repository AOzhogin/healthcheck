package healthcheck

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
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
