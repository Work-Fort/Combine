# MCP Bridge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `combine mcp-bridge` command — a standalone stdio MCP server that exposes Combine's REST API as MCP tools for Claude Code and other MCP clients.

**Architecture:** Standalone `cmd/mcpbridge/` package. MCP server via `github.com/mark3labs/mcp-go`. HTTP client wrapper for REST API calls. Each tool maps 1:1 to an existing `/api/v1` endpoint. Auth via Passport agent API key passed as `--token` flag.

**Tech Stack:** mcp-go (mark3labs), cobra, viper, net/http

**Design doc:** `docs/2026-04-05-mcp-bridge-design.md`

---

## Task 0: Add mcp-go dependency

**Why:** The MCP server library is needed before any tool registration code can compile.

**Steps:**

1. Run `go get github.com/mark3labs/mcp-go@latest` in the project root
2. Run `go mod tidy`
3. Verify the dependency appears in `go.mod`

**Verify:** `go build ./...` succeeds

---

## Task 1: HTTP client wrapper

**Why:** All tool handlers need a common way to make authenticated HTTP requests to Combine's REST API.

**Files:**
- Create: `cmd/mcpbridge/client.go`

**Step 1: Define the client struct**

```go
package mcpbridge

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

type apiClient struct {
    baseURL    string
    token      string
    httpClient *http.Client
}

func newAPIClient(baseURL, token string) *apiClient {
    return &apiClient{
        baseURL: baseURL,
        token:   token,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}
```

**Step 2: Add the `do` method**

```go
func (c *apiClient) do(method, path string, body interface{}) ([]byte, int, error) {
    var bodyReader io.Reader
    if body != nil {
        data, err := json.Marshal(body)
        if err != nil {
            return nil, 0, fmt.Errorf("marshal body: %w", err)
        }
        bodyReader = bytes.NewReader(data)
    }

    req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
    if err != nil {
        return nil, 0, fmt.Errorf("create request: %w", err)
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    if body != nil {
        req.Header.Set("Content-Type", "application/json")
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, 0, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
    }

    return respBody, resp.StatusCode, nil
}
```

The method returns status code so tool handlers can distinguish success from API errors and format MCP responses accordingly.

**Verify:** File compiles: `go build ./cmd/mcpbridge/...`

---

## Task 2: MCP server and cobra command

**Why:** The entry point that wires up the MCP server, registers tools, and connects stdin/stdout.

**Files:**
- Create: `cmd/mcpbridge/mcp_bridge.go`

**Step 1: Define the cobra command**

```go
package mcpbridge

import (
    "fmt"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

func NewCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "mcp-bridge",
        Short: "MCP stdio bridge to Combine REST API",
        Long:  "Runs an MCP server on stdio that exposes Combine's REST API as MCP tools.",
        Args:  cobra.NoArgs,
        RunE:  runBridge,
    }

    cmd.Flags().String("server-url", "http://localhost:23235", "Combine HTTP base URL")
    cmd.Flags().String("token", "", "Passport agent API key")
    _ = viper.BindPFlag("server-url", cmd.Flags().Lookup("server-url"))
    _ = viper.BindPFlag("token", cmd.Flags().Lookup("token"))

    return cmd
}
```

**Step 2: Implement runBridge**

```go
func runBridge(cmd *cobra.Command, args []string) error {
    serverURL := viper.GetString("server-url")
    token := viper.GetString("token")
    if token == "" {
        return fmt.Errorf("--token is required")
    }

    client := newAPIClient(serverURL, token)

    s := server.NewMCPServer(
        "combine",
        "1.0.0",
        server.WithToolCapabilities(true),
    )

    registerRepoTools(s, client)
    registerIssueTools(s, client)
    registerPullRequestTools(s, client)
    registerWebhookTools(s, client)
    registerKeyTools(s, client)

    stdio := server.NewStdioServer(s)
    return stdio.Listen()
}
```

**Step 3: Register command in `cmd/root.go`**

Add to the `init()` function:

```go
import "github.com/Work-Fort/Combine/cmd/mcpbridge"
// ...
rootCmd.AddCommand(
    NewDaemonCmd(),
    NewServeCmd(),
    mcpbridge.NewCmd(),
)
```

**Verify:** `go build ./cmd/combine/` succeeds. `combine mcp-bridge --help` shows flags.

---

## Task 3: Repo tools

**Why:** Repository CRUD is the most fundamental set of tools.

**Files:**
- Create: `cmd/mcpbridge/tools_repos.go`

**Step 1: Implement registerRepoTools**

Register 5 tools:

1. **list_repos** — `GET /api/v1/repos`. No arguments.
2. **create_repo** — `POST /api/v1/repos`. Required: `name` (string). Optional: `description` (string), `private` (boolean), `project_name` (string).
3. **get_repo** — `GET /api/v1/repos/{repo}`. Required: `repo` (string).
4. **update_repo** — `PATCH /api/v1/repos/{repo}`. Required: `repo` (string). Optional: `description` (string), `private` (boolean), `project_name` (string).
5. **delete_repo** — `DELETE /api/v1/repos/{repo}`. Required: `repo` (string).

**Step 2: Implement tool handlers**

Each handler:
1. Extracts arguments from `request.Params.Arguments`
2. Calls `client.do(method, path, body)`
3. Returns `mcp.NewToolResultText(string(responseBody))` on success
4. Returns `mcp.NewToolResultError(string(responseBody))` on 4xx/5xx

Use a helper function to reduce boilerplate:

```go
func toolResult(body []byte, status int) *mcp.CallToolResult {
    text := string(body)
    if status >= 400 {
        return mcp.NewToolResultError(text)
    }
    return mcp.NewToolResultText(text)
}
```

**Verify:** `go build ./cmd/mcpbridge/...`

---

## Task 4: Issue tools

**Files:**
- Create: `cmd/mcpbridge/tools_issues.go`

**Step 1: Implement registerIssueTools**

Register 6 tools:

1. **list_issues** — `GET /api/v1/repos/{repo}/issues`. Required: `repo`.
2. **create_issue** — `POST /api/v1/repos/{repo}/issues`. Required: `repo`, `title`. Optional: `body`.
3. **get_issue** — `GET /api/v1/repos/{repo}/issues/{number}`. Required: `repo`, `number` (integer).
4. **update_issue** — `PATCH /api/v1/repos/{repo}/issues/{number}`. Required: `repo`, `number`. Optional: `title`, `body`, `status`.
5. **list_issue_comments** — `GET /api/v1/repos/{repo}/issues/{number}/comments`. Required: `repo`, `number`.
6. **create_issue_comment** — `POST /api/v1/repos/{repo}/issues/{number}/comments`. Required: `repo`, `number`, `body`.

**Step 2: Implement handlers**

Same pattern as repo tools. Path parameters are interpolated with `fmt.Sprintf`. Integer arguments extracted via type assertion from `float64` (JSON numbers).

**Verify:** `go build ./cmd/mcpbridge/...`

---

## Task 5: Pull request tools

**Files:**
- Create: `cmd/mcpbridge/tools_pulls.go`

**Step 1: Implement registerPullRequestTools**

Register 8 tools:

1. **list_pull_requests** — `GET /api/v1/repos/{repo}/pulls`. Required: `repo`.
2. **create_pull_request** — `POST /api/v1/repos/{repo}/pulls`. Required: `repo`, `title`, `head`, `base`. Optional: `body`.
3. **get_pull_request** — `GET /api/v1/repos/{repo}/pulls/{number}`. Required: `repo`, `number`.
4. **update_pull_request** — `PATCH /api/v1/repos/{repo}/pulls/{number}`. Required: `repo`, `number`. Optional: `title`, `body`, `status`.
5. **merge_pull_request** — `POST /api/v1/repos/{repo}/pulls/{number}/merge`. Required: `repo`, `number`. Optional: `strategy` (string: merge, squash, rebase).
6. **get_pull_request_diff** — `GET /api/v1/repos/{repo}/pulls/{number}/diff`. Required: `repo`, `number`.
7. **list_pull_request_files** — `GET /api/v1/repos/{repo}/pulls/{number}/files`. Required: `repo`, `number`.
8. **submit_review** — `POST /api/v1/repos/{repo}/pulls/{number}/reviews`. Required: `repo`, `number`, `event` (string: approve, request_changes, comment). Optional: `body`, `comments` (array of line-level comments).

**Step 2: Implement handlers**

For `get_pull_request_diff`, the response is plain text (unified diff), not JSON. Return it as-is in the MCP text result.

For `submit_review`, the `comments` argument is an array of objects. Pass through to the REST API as-is since mcp-go supports object arguments.

**Verify:** `go build ./cmd/mcpbridge/...`

---

## Task 6: Webhook tools

**Files:**
- Create: `cmd/mcpbridge/tools_webhooks.go`

**Step 1: Implement registerWebhookTools**

Register 3 tools:

1. **list_webhooks** — `GET /api/v1/repos/{repo}/webhooks`. Required: `repo`.
2. **create_webhook** — `POST /api/v1/repos/{repo}/webhooks`. Required: `repo`, `url`. Optional: `events` (array of strings), `content_type` (string), `active` (boolean).
3. **delete_webhook** — `DELETE /api/v1/repos/{repo}/webhooks/{id}`. Required: `repo`, `id` (integer).

**Verify:** `go build ./cmd/mcpbridge/...`

---

## Task 7: SSH key tools

**Files:**
- Create: `cmd/mcpbridge/tools_keys.go`

**Step 1: Implement registerKeyTools**

Register 3 tools:

1. **list_ssh_keys** — `GET /api/v1/user/keys`. No arguments.
2. **add_ssh_key** — `POST /api/v1/user/keys`. Required: `key` (string). Optional: `name` (string).
3. **delete_ssh_key** — `DELETE /api/v1/user/keys/{id}`. Required: `id` (integer).

**Verify:** `go build ./cmd/mcpbridge/...`

---

## Task 8: E2E test

**Why:** Verify the bridge works end-to-end against a real Combine instance.

**Files:**
- Create: `tests/e2e/mcp_bridge_test.go`

**Step 1: Write TestMCPBridge**

Follow the existing E2E test pattern in `tests/e2e/`:

1. Start a Combine daemon (reuse existing test helpers)
2. Start `combine mcp-bridge` as a subprocess with `--server-url` and `--token`
3. Write MCP `initialize` request to stdin, read response, verify capabilities include tools
4. Call `list_repos` tool, verify empty list
5. Call `create_repo` with `name: "test-repo"`, verify success
6. Call `list_repos`, verify `test-repo` appears
7. Call `get_repo` with `repo: "test-repo"`, verify details
8. Call `delete_repo` with `repo: "test-repo"`, verify success
9. Call `create_repo`, then `create_issue`, `list_issues`, `get_issue` — verify issue workflow
10. Close stdin, verify bridge exits cleanly

Use `encoding/json` to build JSON-RPC requests and parse responses. Each MCP request is a newline-delimited JSON-RPC message on stdin.

**Verify:** `cd tests/e2e && go test -run TestMCPBridge -v`

---

## Task 9: Update remaining-features.md

**Files:**
- Modify: `docs/remaining-features.md`

Add section:

```markdown
## 9. MCP Bridge ✅

[Design](2026-04-05-mcp-bridge-design.md) · [Plan](plans/2026-04-05-mcp-bridge-plan.md)

Standalone `combine mcp-bridge` command that runs an MCP server on stdio,
exposing Combine's REST API as MCP tools. 25 tools covering repos, issues,
pull requests, webhooks, and SSH keys. Uses `mcp-go` library. Configured
with `--server-url` and `--token` flags. E2E tested.
```

**Verify:** Doc reads correctly, links are valid.
