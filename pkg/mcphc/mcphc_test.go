package mcphc_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AOzhogin/healthcheck"
	"github.com/AOzhogin/healthcheck/pkg/mcphc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// headerTransport adds fixed headers to every request.
type headerTransport struct {
	hdr  http.Header
	base http.RoundTripper
}

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, vs := range t.hdr {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return t.base.RoundTrip(r)
}

func connect(t *testing.T, url string, hdr http.Header) *mcp.ClientSession {
	t.Helper()
	cs, err := tryConnect(url, hdr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func tryConnect(url string, hdr http.Header) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	tr := &mcp.StreamableClientTransport{
		Endpoint:   url,
		HTTPClient: &http.Client{Transport: headerTransport{hdr: hdr, base: http.DefaultTransport}},
		MaxRetries: -1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.Connect(ctx, tr, nil)
}

// newHC returns a checker with "db" (ok, counts calls) and "cache" (fails), no background check.
func newHC(t *testing.T, calls *atomic.Int64, opts ...healthcheck.HCOption) interface {
	MCPHandler() http.Handler
	StartHTTPServer() error
} {
	t.Helper()
	hc := healthcheck.New(opts...)
	require.NoError(t, hc.Add("db", "main DB", func(context.Context) error {
		calls.Add(1)
		return nil
	}))
	require.NoError(t, hc.Add("cache", "", func(context.Context) error {
		return errors.New("conn refused")
	}))
	return hc
}

// serve mounts the MCP handler on an httptest server and returns its /mcp URL.
func serve(t *testing.T, calls *atomic.Int64, opts ...healthcheck.HCOption) string {
	t.Helper()
	hc := newHC(t, calls, opts...)
	mux := http.NewServeMux()
	mux.Handle(healthcheck.HandlerMCP, hc.MCPHandler())
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + healthcheck.HandlerMCP
}

// startServer runs StartHTTPServer on addr in the background and waits until it accepts connections.
func startServer(t *testing.T, addr string, calls *atomic.Int64, opts ...healthcheck.HCOption) string {
	t.Helper()
	hc := newHC(t, calls, append(opts, healthcheck.WithHTTPAddress(addr))...)
	go func() { _ = hc.StartHTTPServer() }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", "127.0.0.1"+addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server on %s did not start: %v", addr, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return "http://127.0.0.1" + addr + healthcheck.HandlerMCP
}

func basicAuth(user, pass string) http.Header {
	r, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	r.SetBasicAuth(user, pass)
	return http.Header{"Authorization": r.Header.Values("Authorization")}
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

func callText(t *testing.T, cs *mcp.ClientSession, name string, args any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "content is %T", res.Content[0])
	return tc.Text, res.IsError
}

func TestMCP_ListsToolsAndResources(t *testing.T) {
	var calls atomic.Int64
	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New())), nil)

	assert.Equal(t, []string{"get_config", "get_status", "list_checks", "run_check"}, toolNames(t, cs))

	res, err := cs.ListResources(context.Background(), nil)
	require.NoError(t, err)
	uris := make([]string, 0, len(res.Resources))
	for _, r := range res.Resources {
		uris = append(uris, r.URI)
	}
	assert.ElementsMatch(t, []string{"healthcheck://config", "healthcheck://checks"}, uris)
}

func TestMCP_WithoutRunCheck(t *testing.T) {
	var calls atomic.Int64
	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New(mcphc.WithoutRunCheck()))), nil)

	assert.Equal(t, []string{"get_config", "get_status", "list_checks"}, toolNames(t, cs))
}

func TestMCP_WithServerName(t *testing.T) {
	var calls atomic.Int64

	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New(mcphc.WithServerName("orders-api")))), nil)
	assert.Equal(t, "orders-api", cs.InitializeResult().ServerInfo.Name)

	def := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New())), nil)
	assert.Equal(t, "healthcheck", def.InitializeResult().ServerInfo.Name)
}

func TestMCP_ReadResources(t *testing.T) {
	var calls atomic.Int64
	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New())), nil)
	ctx := context.Background()

	cfg, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "healthcheck://config"})
	require.NoError(t, err)
	require.Len(t, cfg.Contents, 1)
	assert.Equal(t, "application/json", cfg.Contents[0].MIMEType)
	assert.Contains(t, cfg.Contents[0].Text, `"http_addr"`)

	st, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "healthcheck://checks"})
	require.NoError(t, err)
	require.Len(t, st.Contents, 1)
	assert.Equal(t, "application/json", st.Contents[0].MIMEType)
	assert.Contains(t, st.Contents[0].Text, `"checks"`)
	assert.Contains(t, st.Contents[0].Text, `"conn refused"`)
}

func TestMCP_GetConfigHasNoSecrets(t *testing.T) {
	var calls atomic.Int64
	url := startServer(t, ":18210", &calls,
		healthcheck.WithMCP(mcphc.New()),
		healthcheck.WithBasicAuth("admin-user", "s3cret-pass"),
	)
	cs := connect(t, url, basicAuth("admin-user", "s3cret-pass"))

	text, isErr := callText(t, cs, "get_config", nil)
	assert.False(t, isErr)
	assert.Contains(t, text, `"basic_auth":true`)
	assert.NotContains(t, text, "admin-user")
	assert.NotContains(t, text, "s3cret-pass")
}

func TestMCP_ListChecksDoesNotRunChecks(t *testing.T) {
	var calls atomic.Int64
	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New())), nil)

	text, isErr := callText(t, cs, "list_checks", nil)
	assert.False(t, isErr)

	var checks []struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &checks))
	require.Len(t, checks, 2)
	assert.Equal(t, "cache", checks[0].Name)
	assert.Equal(t, "db", checks[1].Name)
	assert.Equal(t, "main DB", checks[1].Notes)
	assert.Zero(t, calls.Load())
}

func TestMCP_GetStatusRunsChecksWithoutBackground(t *testing.T) {
	var calls atomic.Int64
	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New())), nil)

	text, isErr := callText(t, cs, "get_status", nil)
	assert.False(t, isErr)
	assert.Equal(t, int64(1), calls.Load())
	assert.Contains(t, text, `"conn refused"`)
}

func TestMCP_RunCheck(t *testing.T) {
	var calls atomic.Int64
	cs := connect(t, serve(t, &calls, healthcheck.WithMCP(mcphc.New())), nil)

	text, isErr := callText(t, cs, "run_check", map[string]any{"name": "db"})
	assert.False(t, isErr)
	assert.Contains(t, text, `"status":"ok"`)

	text, isErr = callText(t, cs, "run_check", map[string]any{"name": "cache"})
	assert.False(t, isErr)
	assert.Contains(t, text, `"status":"conn refused"`)

	text, isErr = callText(t, cs, "run_check", map[string]any{"name": "nope"})
	assert.True(t, isErr)
	assert.Equal(t, `healthcheck: mcp: unknown check "nope"`, text)

	// "name" is required by the input schema.
	_, isErr = callText(t, cs, "run_check", map[string]any{})
	assert.True(t, isErr)
}

func TestMCP_BasicAuthRequired(t *testing.T) {
	var calls atomic.Int64
	url := startServer(t, ":18211", &calls,
		healthcheck.WithMCP(mcphc.New()),
		healthcheck.WithBasicAuth("u", "p"),
	)

	cs, err := tryConnect(url, nil)
	if cs != nil {
		_ = cs.Close()
	}
	require.Error(t, err)

	authed := connect(t, url, basicAuth("u", "p"))
	_, err = authed.ListTools(context.Background(), nil)
	require.NoError(t, err)
}
