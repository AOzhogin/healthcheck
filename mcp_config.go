package healthcheck

import (
	"encoding/json"
	"sort"
	"time"
)

type (
	mcpConfigDoc struct {
		HTTPAddr         string             `json:"http_addr"`
		Timeout          string             `json:"timeout"`
		BackgroundCheck  mcpBackgroundCheck `json:"background_check"`
		StatusCodes      mcpStatusCodes     `json:"status_codes"`
		CheckStatus      mcpCheckStatus     `json:"check_status"`
		BasicAuth        bool               `json:"basic_auth"`
		CustomMiddleware bool               `json:"custom_middleware"`
		Pprof            bool               `json:"pprof"`
		Metrics          mcpMetrics         `json:"metrics"`
		Tracer           bool               `json:"tracer"`
		Logger           bool               `json:"logger"`
		CustomHandlers   mcpCustomHandlers  `json:"custom_handlers"`
		Checks           []mcpCheckInfo     `json:"checks"`
		TLS              mcpTLS             `json:"tls"`
	}

	mcpBackgroundCheck struct {
		Enabled  bool   `json:"enabled"`
		Interval string `json:"interval"`
	}

	mcpStatusCodes struct {
		Success int `json:"success"`
		Error   int `json:"error"`
	}

	mcpCheckStatus struct {
		Success string `json:"success"`
		Error   string `json:"error"`
	}

	mcpMetrics struct {
		Enabled bool `json:"enabled"`
	}

	mcpCustomHandlers struct {
		Live    bool `json:"live"`
		Ready   bool `json:"ready"`
		Startup bool `json:"startup"`
	}

	mcpCheckInfo struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}

	mcpTLS struct {
		Enabled         bool           `json:"enabled"`
		CertFile        string         `json:"cert_file"`
		CustomTLSConfig bool           `json:"custom_tls_config"`
		ReloadInterval  string         `json:"reload_interval"`
		Certificate     mcpCertificate `json:"certificate"`
	}

	// mcpCertificate is {"loaded":false} alone when no certificate is being served.
	mcpCertificate struct {
		Loaded    bool       `json:"loaded"`
		Subject   string     `json:"subject,omitempty"`
		DNSNames  []string   `json:"dns_names"`
		Issuer    string     `json:"issuer,omitempty"`
		Serial    string     `json:"serial,omitempty"`
		NotBefore *time.Time `json:"not_before,omitempty"`
		NotAfter  *time.Time `json:"not_after,omitempty"`
	}
)

// MarshalJSON emits exactly {"loaded":false} when no certificate is served; for a loaded one every
// field is present and dns_names is always an array.
func (c mcpCertificate) MarshalJSON() ([]byte, error) {
	if !c.Loaded {
		return []byte(`{"loaded":false}`), nil
	}

	type plain mcpCertificate

	return json.Marshal(plain(c))
}

// mcpConfig serializes the effective configuration for the MCP endpoint. Secrets (Basic Auth
// credentials, the TLS key path) are never included.
func (h *healthCheck) mcpConfig() json.RawMessage {
	doc := mcpConfigDoc{
		HTTPAddr:         h.httpAddr,
		Timeout:          h.timeOut.String(),
		BackgroundCheck:  mcpBackgroundCheck{Enabled: h.routine, Interval: h.routineInterval.String()},
		StatusCodes:      mcpStatusCodes{Success: h.statusCodeSuccess, Error: h.statusCodeError},
		CheckStatus:      mcpCheckStatus{Success: h.checkStatusSuccess, Error: h.checkStatusError},
		BasicAuth:        h.withBasicAuth(),
		CustomMiddleware: h.customMiddleware != nil,
		Pprof:            h.pprofEnabled,
		Metrics:          mcpMetrics{Enabled: h.withMetrics()},
		Logger:           h.logger != nil,
		CustomHandlers: mcpCustomHandlers{
			Live:    h.liveHandler != nil,
			Ready:   h.readyHandler != nil,
			Startup: h.startupHandler != nil,
		},
		Checks: h.mcpChecks(),
		TLS: mcpTLS{
			Enabled:         h.withTLS(),
			CertFile:        h.tlsCertFile,
			CustomTLSConfig: h.tlsConfig != nil,
			ReloadInterval:  h.tlsReloadInterval.String(),
			Certificate:     h.mcpCertificate(),
		},
	}

	if _, nop := h.tracer.(nopTracer); !nop && h.tracer != nil {
		doc.Tracer = true
	}

	b, err := json.Marshal(doc)
	if err != nil {
		return json.RawMessage(`{}`)
	}

	return b
}

func (h *healthCheck) mcpChecks() []mcpCheckInfo {
	h.checks.Mutex.Lock()
	defer h.checks.Mutex.Unlock()

	out := make([]mcpCheckInfo, 0, len(h.checks.List))
	for name, c := range h.checks.List {
		out = append(out, mcpCheckInfo{Name: name, Notes: c.Notes})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

func (h *healthCheck) mcpCertificate() mcpCertificate {
	r := h.tlsReloader.Load()
	if r == nil {
		return mcpCertificate{}
	}

	leaf := r.leaf()
	if leaf == nil {
		return mcpCertificate{}
	}

	nb, na := leaf.NotBefore.UTC(), leaf.NotAfter.UTC()

	return mcpCertificate{
		Loaded:    true,
		Subject:   leaf.Subject.CommonName,
		DNSNames:  append([]string{}, leaf.DNSNames...),
		Issuer:    leaf.Issuer.CommonName,
		Serial:    leaf.SerialNumber.Text(16),
		NotBefore: &nb,
		NotAfter:  &na,
	}
}
