# MCP Bridge Design

## Context

Claude Code and other MCP (Model Context Protocol) clients need to interact
with Combine's REST API. MCP clients communicate via JSON-RPC over stdio. The
bridge translates MCP tool calls into HTTP requests against Combine's existing
`/api/v1` endpoints.

## Architecture

**Approach A: Standalone MCP server (selected)**

A separate `combine mcp-bridge` command that:
- Runs an MCP server on stdio using `github.com/mark3labs/mcp-go`
- Defines tools that map 1:1 to Combine's REST API endpoints
- Makes HTTP requests to a running Combine instance
- Authenticates every request with a Passport agent API key

This differs from Sharkfin/Hive/Nexus which use a pure stdio-to-HTTP proxy
(forwarding raw JSON-RPC to a server-side `/mcp` endpoint). Combine has no
`/mcp` endpoint — it has a standard REST API — so the bridge must define MCP
tools and translate them to REST calls.

```
Claude Code  <--stdio-->  combine mcp-bridge  <--HTTP-->  Combine daemon
                          (MCP server)                    (/api/v1/*)
```

**Why not Approach B (in-process)?** Keeping the bridge as a standalone binary
decouples it from the daemon lifecycle, simplifies testing, and follows the
pattern established by other WorkFort services.

## Configuration

```
combine mcp-bridge --server-url http://localhost:23235 --token <passport-api-key>
```

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--server-url` | `COMBINE_SERVER_URL` | `http://localhost:23235` | Combine HTTP base URL |
| `--token` | `COMBINE_TOKEN` | (required) | Passport agent API key |

The bridge reads these via Viper, consistent with Combine's config pattern.

## MCP Tools

Each tool maps to one REST endpoint. Tool names use snake_case. Arguments map
to path parameters or JSON request body fields.

### Repos

| Tool | Method | Endpoint |
|------|--------|----------|
| `list_repos` | GET | `/api/v1/repos` |
| `create_repo` | POST | `/api/v1/repos` |
| `get_repo` | GET | `/api/v1/repos/{repo}` |
| `update_repo` | PATCH | `/api/v1/repos/{repo}` |
| `delete_repo` | DELETE | `/api/v1/repos/{repo}` |

### Issues

| Tool | Method | Endpoint |
|------|--------|----------|
| `list_issues` | GET | `/api/v1/repos/{repo}/issues` |
| `create_issue` | POST | `/api/v1/repos/{repo}/issues` |
| `get_issue` | GET | `/api/v1/repos/{repo}/issues/{number}` |
| `update_issue` | PATCH | `/api/v1/repos/{repo}/issues/{number}` |
| `list_issue_comments` | GET | `/api/v1/repos/{repo}/issues/{number}/comments` |
| `create_issue_comment` | POST | `/api/v1/repos/{repo}/issues/{number}/comments` |

### Pull Requests

| Tool | Method | Endpoint |
|------|--------|----------|
| `list_pull_requests` | GET | `/api/v1/repos/{repo}/pulls` |
| `create_pull_request` | POST | `/api/v1/repos/{repo}/pulls` |
| `get_pull_request` | GET | `/api/v1/repos/{repo}/pulls/{number}` |
| `update_pull_request` | PATCH | `/api/v1/repos/{repo}/pulls/{number}` |
| `merge_pull_request` | POST | `/api/v1/repos/{repo}/pulls/{number}/merge` |
| `get_pull_request_diff` | GET | `/api/v1/repos/{repo}/pulls/{number}/diff` |
| `list_pull_request_files` | GET | `/api/v1/repos/{repo}/pulls/{number}/files` |
| `submit_review` | POST | `/api/v1/repos/{repo}/pulls/{number}/reviews` |

### Webhooks

| Tool | Method | Endpoint |
|------|--------|----------|
| `list_webhooks` | GET | `/api/v1/repos/{repo}/webhooks` |
| `create_webhook` | POST | `/api/v1/repos/{repo}/webhooks` |
| `delete_webhook` | DELETE | `/api/v1/repos/{repo}/webhooks/{id}` |

### SSH Keys

| Tool | Method | Endpoint |
|------|--------|----------|
| `list_ssh_keys` | GET | `/api/v1/user/keys` |
| `add_ssh_key` | POST | `/api/v1/user/keys` |
| `delete_ssh_key` | DELETE | `/api/v1/user/keys/{id}` |

## Tool Schema Examples

Each tool is registered with `mcp-go`'s `server.NewTool()`. Path parameters
become required string/integer arguments. Body parameters become optional
arguments matching the REST API's JSON field names.

```go
// Example: get_repo
server.NewTool(
    "get_repo",
    "Get repository details",
    mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
)

// Example: create_issue
server.NewTool(
    "create_issue",
    "Create a new issue",
    mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
    mcp.WithString("title", mcp.Required(), mcp.Description("Issue title")),
    mcp.WithString("body", mcp.Description("Issue body")),
)
```

## HTTP Client

The bridge uses a simple HTTP client struct:

```go
type client struct {
    baseURL    string
    token      string
    httpClient *http.Client
}

func (c *client) do(method, path string, body interface{}) ([]byte, error) {
    // Build URL from baseURL + path
    // Marshal body to JSON if non-nil
    // Set Authorization: Bearer <token>
    // Set Content-Type: application/json
    // Return response body bytes
}
```

Tool handlers extract arguments, call `client.do()`, and return the response
as MCP text content. Errors from the REST API (4xx/5xx) are returned as MCP
error results with the response body as the error message.

## File Layout

```
cmd/
  mcpbridge/
    mcp_bridge.go      # Cobra command, MCP server setup, tool registration
    client.go          # HTTP client wrapper
    tools_repos.go     # Repo tool handlers
    tools_issues.go    # Issue tool handlers
    tools_pulls.go     # PR tool handlers
    tools_webhooks.go  # Webhook tool handlers
    tools_keys.go      # SSH key tool handlers
cmd/
  combine/
    main.go            # Add mcpbridge command
```

The command is registered in `cmd/root.go` alongside `NewDaemonCmd()`.

## Claude Code Configuration

Users add this to their MCP config (`.mcp.json` or `settings.json`):

```json
{
  "mcpServers": {
    "combine": {
      "command": "combine",
      "args": ["mcp-bridge", "--server-url", "http://localhost:23235", "--token", "<key>"]
    }
  }
}
```

## Testing

- Unit tests for the HTTP client (`client_test.go`)
- Integration test in `tests/e2e/` that starts Combine, runs `mcp-bridge`,
  sends MCP initialize + tool calls over stdin, and verifies responses
- Each tool handler tested via the integration path (no mocks)

## Non-Goals

- No WebSocket presence channel (unlike Sharkfin/Nexus which have real-time
  messaging). Combine is a Git forge — all operations are request/response.
- No server-side `/mcp` endpoint. The bridge is the MCP server.
- No `wait_for_messages` tool (no real-time messaging in Combine).
