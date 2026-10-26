package healthcheck

import (
	"context"
	"encoding/json"
	"net/http"
)

// MCPSource gives an MCPServer read access to the health check state without exposing
// the internals of the checker.
type MCPSource struct {
	// Config returns the effective configuration as JSON.
	Config func() json.RawMessage
	// Status returns a snapshot of the current check statuses as JSON.
	Status func(ctx context.Context) json.RawMessage
	// RunCheck runs a single named check and returns its result as JSON.
	// It returns an error for an unknown check name.
	RunCheck func(ctx context.Context, name string) (json.RawMessage, error)
}

// MCPServer builds an http.Handler that serves the Model Context Protocol on top of src.
// Implementations live outside the core module (see pkg/mcphc).
type MCPServer interface {
	Handler(src MCPSource) http.Handler
}

// MCPHandler returns the MCP handler, or nil when WithMCP was not set.
// The handler is not wrapped in Basic Auth or custom middleware; wrap it yourself when
// mounting it on your own mux. Every call builds a new handler.
func (h *healthCheck) MCPHandler() http.Handler {
	if h.mcp == nil {
		return nil
	}
	return h.mcp.Handler(MCPSource{
		Config:   h.mcpConfig,
		Status:   h.mcpStatus,
		RunCheck: h.mcpRunCheck,
	})
}
