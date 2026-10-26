// Package mcphc serves the healthcheck state over the Model Context Protocol, using the
// official MCP Go SDK (github.com/modelcontextprotocol/go-sdk).
//
// It lives in a separate module so the core healthcheck library keeps NO MCP SDK
// dependency: only import this package if you want an MCP endpoint. With WithMCP set,
// StartHTTPServer serves it at /mcp (Streamable HTTP, stateless) behind the same
// Basic Auth and middleware as the other endpoints.
//
//	hc := healthcheck.New(
//	    healthcheck.WithMCP(mcphc.New(mcphc.WithServerName("orders-api"))),
//	)
//
// Resources: healthcheck://config (effective configuration) and healthcheck://checks
// (current status). Tools: get_config, get_status, list_checks and run_check (the latter
// can be disabled with WithoutRunCheck). Secrets are never exposed.
package mcphc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/AOzhogin/healthcheck"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultServerName = "healthcheck"
	serverVersion     = "v1.5.0"
	mimeJSON          = "application/json"

	uriConfig = "healthcheck://config"
	uriChecks = "healthcheck://checks"
)

type config struct {
	name     string
	runCheck bool
}

// Option configures the MCP server returned by New.
type Option func(*config)

// WithoutRunCheck does not register the run_check tool, so MCP clients cannot execute checks
// on demand.
func WithoutRunCheck() Option {
	return func(c *config) { c.runCheck = false }
}

// WithServerName sets the server name reported to MCP clients on initialize.
// The default is "healthcheck".
func WithServerName(name string) Option {
	return func(c *config) { c.name = name }
}

// New returns a healthcheck.MCPServer to pass to healthcheck.WithMCP.
func New(opts ...Option) healthcheck.MCPServer {
	c := config{name: defaultServerName, runCheck: true}
	for _, o := range opts {
		o(&c)
	}
	return server{cfg: c}
}

type server struct{ cfg config }

// runCheckInput is the input of the run_check tool.
type runCheckInput struct {
	Name string `json:"name" jsonschema:"name of the registered check to run"`
}

// Handler builds the MCP server over src and returns a stateless Streamable HTTP handler.
func (s server) Handler(src healthcheck.MCPSource) http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: s.cfg.name, Version: serverVersion}, nil)

	srv.AddResource(&mcp.Resource{
		URI:         uriConfig,
		Name:        "config",
		Description: "Effective healthcheck configuration and the currently served TLS certificate. Secrets are never included.",
		MIMEType:    mimeJSON,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return resource(uriConfig, src.Config()), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         uriChecks,
		Name:        "checks",
		Description: "Overall status and the last result of every check, same as /health?body=true.",
		MIMEType:    mimeJSON,
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return resource(uriChecks, src.Status(ctx)), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_config",
		Description: "Effective healthcheck configuration and the currently served TLS certificate. Secrets are never included.",
	}, func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, any, error) {
		return text(src.Config()), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_status",
		Description: "Overall status and the last result of every check, same as /health?body=true. When background checks are off this runs all checks.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		return text(src.Status(ctx)), nil, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_checks",
		Description: "Registered checks (name and notes). Does not run any check.",
	}, func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, any, error) {
		var doc struct {
			Checks json.RawMessage `json:"checks"`
		}
		if err := json.Unmarshal(src.Config(), &doc); err != nil {
			return nil, nil, err
		}
		if len(doc.Checks) == 0 {
			doc.Checks = json.RawMessage(`[]`)
		}
		return text(doc.Checks), nil, nil
	})

	if s.cfg.runCheck {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "run_check",
			Description: "Runs one check now and returns its result. Executes the real check function; does not affect probes, metrics or logs.",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in runCheckInput) (*mcp.CallToolResult, any, error) {
			res, err := src.RunCheck(ctx, in.Name)
			if err != nil {
				// A plain error becomes a tool result with IsError set, not a protocol error.
				return nil, nil, err
			}
			return text(res), nil, nil
		})
	}

	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true})
}

func resource(uri string, b json.RawMessage) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI:      uri,
		MIMEType: mimeJSON,
		Text:     string(b),
	}}}
}

func text(b json.RawMessage) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}
