package healthcheck

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"
)

// CertExpiryCheck returns a check that fails when the certificate in certFile expires within
// threshold. Register it with Add so a certificate that silently stopped rotating — every reload
// failing, or the secret never being updated — turns into a failing probe and a visible alert well
// before clients start seeing handshake errors:
//
//	hc.Add("tls-cert", "serving certificate", healthcheck.CertExpiryCheck("/certs/tls.crt", 14*24*time.Hour))
//
// The file is read on every run, so it tracks rotations without any extra wiring.
func CertExpiryCheck(certFile string, threshold time.Duration) HCFunc {
	return func(_ context.Context) error {
		leaf, err := readLeafCertificate(certFile)
		if err != nil {
			return certCheckError{reason: "cert_read", msg: fmt.Sprintf("read certificate %s: %v", certFile, err)}
		}

		if left := time.Until(leaf.NotAfter); left < threshold {
			return certCheckError{
				reason: "cert_expiry",
				msg: fmt.Sprintf("certificate %q expires in %s (threshold %s)",
					leaf.Subject.CommonName, left.Round(time.Second), threshold),
			}
		}

		return nil
	}
}

// readLeafCertificate parses the first CERTIFICATE block of a PEM file — the leaf, following the
// ordering crypto/tls expects in a certificate chain file.
func readLeafCertificate(certFile string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}

	for block, rest := pem.Decode(raw); block != nil; block, rest = pem.Decode(rest) {
		if block.Type != "CERTIFICATE" {
			continue
		}

		return x509.ParseCertificate(block.Bytes)
	}

	return nil, fmt.Errorf("no CERTIFICATE block in %s", certFile)
}

// certCheckError carries a dedicated reason so errors_total gets a specific label instead of the
// generic "error" bucket (see classifyReason).
type certCheckError struct {
	reason string
	msg    string
}

func (e certCheckError) Error() string { return e.msg }

// Reason implements the interface classifyReason looks for.
func (e certCheckError) Reason() string { return e.reason }
