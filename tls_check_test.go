package healthcheck

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCertExpiryCheck(t *testing.T) {
	dir := t.TempDir()

	valid, _ := newSelfSignedPair(t, dir, "valid", time.Now().Add(30*24*time.Hour))

	expiringDir := t.TempDir()
	expiring, _ := newSelfSignedPair(t, expiringDir, "expiring", time.Now().Add(time.Hour))

	garbageDir := t.TempDir()
	garbage := filepath.Join(garbageDir, "tls.crt")
	require.NoError(t, os.WriteFile(garbage, []byte("not a pem file"), 0o600))

	tests := []struct {
		name       string
		certFile   string
		threshold  time.Duration
		wantErr    bool
		wantReason string
	}{
		{
			name:      "valid far from expiry",
			certFile:  valid,
			threshold: 14 * 24 * time.Hour,
			wantErr:   false,
		},
		{
			name:       "inside the threshold",
			certFile:   expiring,
			threshold:  14 * 24 * time.Hour,
			wantErr:    true,
			wantReason: "cert_expiry",
		},
		{
			name:       "missing file",
			certFile:   filepath.Join(dir, "nope.crt"),
			threshold:  time.Hour,
			wantErr:    true,
			wantReason: "cert_read",
		},
		{
			name:       "not a certificate",
			certFile:   garbage,
			threshold:  time.Hour,
			wantErr:    true,
			wantReason: "cert_read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CertExpiryCheck(tt.certFile, tt.threshold)(context.Background())

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Equal(t, tt.wantReason, classifyReason(err),
				"the reason drives the errors_total label, so it must be specific")
		})
	}
}

func TestCertExpiryCheck_WorksThroughAdd(t *testing.T) {
	dir := t.TempDir()
	certPath, _ := newSelfSignedPair(t, dir, "valid", time.Now().Add(30*24*time.Hour))

	h := New()
	require.NoError(t, h.Add("tls-cert", "serving certificate", CertExpiryCheck(certPath, 24*time.Hour)))

	res := h.check(context.Background())
	require.Equal(t, checkStatusSuccess, res.Status)
}
