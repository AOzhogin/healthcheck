# MCP example

Runs a healthcheck service with two checks (`db` always passes, `cache` fails on every second call) and
the MCP endpoint at `/mcp` on port `8080`, protected by Basic Auth (`admin`/`admin` by default, override
with `HC_USER`/`HC_PASS`; demo credentials only). The MCP server is built on the official Go SDK and uses
Streamable HTTP in stateless mode.

## Local Docker + Claude Code

From this directory:

```bash
docker compose up --build
claude mcp add --transport http healthcheck http://localhost:8080/mcp \
  --header "Authorization: Basic $(printf admin:admin | base64)"
```

Then run `/mcp` inside Claude Code to check that the `healthcheck` server is connected, and ask
"what is the status of my checks?".

Without Docker: `GOWORK=off go run ./example` from `pkg/mcphc`.

## In-cluster agent

Point the agent at the service DNS name:

```
http://<service>.<ns>.svc:<port>/mcp
```

Take the credentials from a Kubernetes Secret and build the `Authorization: Basic ...` header from them.
A generic MCP client config (Streamable HTTP transport plus a header):

```json
{
  "mcpServers": {
    "healthcheck": {
      "type": "http",
      "url": "http://<service>.<ns>.svc:<port>/mcp",
      "headers": { "Authorization": "Basic <base64(user:pass)>" }
    }
  }
}
```

Use `https://` if the service is started with `WithTLS`.

If requests fail with `403 Forbidden: invalid Host header`, this is the go-sdk DNS-rebinding protection: the
Streamable HTTP handler rejects a request that arrives on a loopback address while its `Host` header is not
loopback, e.g. behind a sidecar reverse proxy (kube-rbac-proxy style) that forwards to 127.0.0.1 and preserves the
original Host. Escape hatch: set the env `MCPGODEBUG=disablelocalhostprotection=1` on the service.

## curl smoke test

```bash
curl -s -u admin:admin http://localhost:8080/mcp \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
```

The response contains `"serverInfo":{"name":"healthcheck"`. Without `-u` the endpoint answers `401`.

## Own mux

If you serve HTTP yourself instead of calling `StartHTTPServer`, mount the handler explicitly:

```go
mux.Handle(healthcheck.HandlerMCP, hc.MCPHandler())
```

`MCPHandler()` is unwrapped: you add authentication and middleware yourself. In this mode
`tls.certificate.loaded` in the config output is always `false` and `http_addr` only reflects the `WithHTTPAddress` option value, not your server's real address.

## Security

- Do not expose `/mcp` outside localhost without Basic Auth or your own middleware.
- Use `mcphc.WithoutRunCheck()` to drop the `run_check` tool if checks are heavy or must not be
  triggered by an agent.
- With MCP enabled, check functions may be called concurrently (a tool call can overlap with the
  background check), so they must be safe for that.

## Resources and tools

| Name | Kind | Description |
|------|------|-------------|
| `healthcheck://config` | resource | Effective healthcheck configuration and the currently served TLS certificate. Secrets are never included. |
| `healthcheck://checks` | resource | Overall status and the last result of every check, same as `/health?body=true`. |
| `get_config` | tool | Effective healthcheck configuration and the currently served TLS certificate. Secrets are never included. |
| `get_status` | tool | Overall status and the last result of every check, same as `/health?body=true`. When background checks are off this runs all checks. |
| `list_checks` | tool | Registered checks (name and notes). Does not run any check. |
| `run_check` | tool | Runs one check now (argument `name`) and returns its result. Executes the real check function; does not affect probes, metrics or logs. |
