package healthcheck

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeMCP is a test MCPServer that records the source it is given.
type fakeMCP struct{ src MCPSource }

func (f *fakeMCP) Handler(src MCPSource) http.Handler {
	f.src = src
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(src.Config())
	})
}

func TestMCPHandler_NilWithoutOption(t *testing.T) {
	if New().MCPHandler() != nil {
		t.Fatal("MCPHandler without WithMCP must be nil")
	}
	if New(WithMCP(nil)).MCPHandler() != nil {
		t.Fatal("MCPHandler with WithMCP(nil) must be nil")
	}
}

func TestMCPHandler_SourceIsWired(t *testing.T) {
	f := &fakeMCP{}
	h := New(WithMCP(f))
	h.Add("db", "n", func(context.Context) error { return nil })
	if h.MCPHandler() == nil {
		t.Fatal("expected handler")
	}
	ctx := context.Background()
	if !strings.Contains(string(f.src.Config()), `"name":"db"`) {
		t.Errorf("config: %s", f.src.Config())
	}
	if !strings.Contains(string(f.src.Status(ctx)), `"checks"`) {
		t.Errorf("status: %s", f.src.Status(ctx))
	}
	res, err := f.src.RunCheck(ctx, "db")
	if err != nil || !strings.Contains(string(res), `"status":"ok"`) {
		t.Errorf("run: %s %v", res, err)
	}
	if _, err := f.src.RunCheck(ctx, "x"); err == nil || err.Error() != `healthcheck: mcp: unknown check "x"` {
		t.Errorf("unknown: %v", err)
	}
	var _ json.RawMessage = f.src.Config()
}

func TestMCPHandler_OwnMux(t *testing.T) {
	f := &fakeMCP{}
	h := New(WithMCP(f))
	mux := http.NewServeMux()
	mux.Handle(HandlerMCP, h.MCPHandler())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if rec.Code != 200 || rec.Body.String() != string(f.src.Config()) {
		t.Errorf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestStartHTTPServer_NoMCPRouteByDefault(t *testing.T) {
	hc := New(WithHTTPAddress(":18201"))
	go func() { _ = hc.StartHTTPServer() }()
	time.Sleep(100 * time.Millisecond)
	defer hc.Shutdown()
	resp, err := http.Get("http://localhost:18201/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestStartHTTPServer_MCPRouteWithAuthAndMiddleware(t *testing.T) {
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	}
	hc := New(WithHTTPAddress(":18202"), WithMCP(&fakeMCP{}), WithBasicAuth("u", "p"), WithMiddleware(mw))
	go func() { _ = hc.StartHTTPServer() }()
	time.Sleep(100 * time.Millisecond)
	defer hc.Shutdown()

	resp, err := http.Get("http://localhost:18202/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no auth: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://localhost:18202/mcp", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Custom-Middleware") != "true" {
		t.Errorf("auth: %d hdr=%q", resp.StatusCode, resp.Header.Get("X-Custom-Middleware"))
	}
}
