package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/Work-Fort/combine-e2e/harness"
)

// jsonrpcRequest is a JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type mcpBridge struct {
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	nextID  int
}

func startMCPBridge(t *testing.T, serverURL, token string) *mcpBridge {
	t.Helper()

	cmd := exec.Command(combineBin, "mcp-bridge", "--server-url", serverURL, "--token", token)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp-bridge: %v", err)
	}

	t.Cleanup(func() {
		stdin.Close()
		cmd.Wait()
	})

	return &mcpBridge{
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
		nextID:  1,
	}
}

func (b *mcpBridge) call(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()

	id := b.nextID
	b.nextID++

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	data = append(data, '\n')

	if _, err := b.stdin.Write(data); err != nil {
		t.Fatalf("write request: %v", err)
	}

	// Read lines until we get a response with our ID
	for b.scanner.Scan() {
		line := b.scanner.Text()
		if line == "" {
			continue
		}
		var resp jsonrpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			// Could be a notification, skip
			continue
		}
		if resp.ID == id {
			if resp.Error != nil {
				t.Fatalf("JSON-RPC error for %s: %s", method, string(resp.Error))
			}
			return resp.Result
		}
	}
	if err := b.scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	t.Fatalf("no response received for %s (id=%d)", method, id)
	return nil
}

func (b *mcpBridge) callTool(t *testing.T, name string, args map[string]any) string {
	t.Helper()

	result := b.call(t, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})

	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &toolResult); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if toolResult.IsError {
		t.Fatalf("tool %s returned error: %s", name, toolResult.Content[0].Text)
	}
	if len(toolResult.Content) == 0 {
		return ""
	}
	return strings.TrimSpace(toolResult.Content[0].Text)
}

func TestMCPBridge(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)
	token := d.SignJWT("uuid-mcpuser", "mcpuser", "MCP User", "user")

	bridge := startMCPBridge(t, fmt.Sprintf("http://%s", d.HTTPAddr), token)

	// Initialize
	initResult := bridge.call(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test-client",
			"version": "1.0.0",
		},
	})

	var initResp map[string]any
	if err := json.Unmarshal(initResult, &initResp); err != nil {
		t.Fatalf("unmarshal init result: %v", err)
	}
	caps, ok := initResp["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("no capabilities in init response")
	}
	if _, ok := caps["tools"]; !ok {
		t.Fatal("no tools capability in init response")
	}

	// Send initialized notification (no response expected)
	notif, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	notif = append(notif, '\n')
	bridge.stdin.Write(notif)

	// List repos — should be empty
	listResult := bridge.callTool(t, "list_repos", nil)
	if listResult != "[]" && listResult != "null" {
		t.Fatalf("expected empty list, got: %s", listResult)
	}

	// Create repo
	createResult := bridge.callTool(t, "create_repo", map[string]any{
		"name": "test-repo",
	})
	var created map[string]any
	if err := json.Unmarshal([]byte(createResult), &created); err != nil {
		t.Fatalf("unmarshal create result: %v", err)
	}
	if created["name"] != "test-repo" {
		t.Fatalf("expected name test-repo, got %v", created["name"])
	}

	// List repos — should have one
	listResult = bridge.callTool(t, "list_repos", nil)
	var repos []map[string]any
	if err := json.Unmarshal([]byte(listResult), &repos); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0]["name"] != "test-repo" {
		t.Fatalf("expected test-repo, got %v", repos[0]["name"])
	}

	// Get repo
	getResult := bridge.callTool(t, "get_repo", map[string]any{
		"repo": "test-repo",
	})
	var repo map[string]any
	if err := json.Unmarshal([]byte(getResult), &repo); err != nil {
		t.Fatalf("unmarshal get result: %v", err)
	}
	if repo["name"] != "test-repo" {
		t.Fatalf("expected test-repo, got %v", repo["name"])
	}

	// Create issue
	issueResult := bridge.callTool(t, "create_issue", map[string]any{
		"repo":  "test-repo",
		"title": "Test issue",
		"body":  "This is a test issue",
	})
	var issue map[string]any
	if err := json.Unmarshal([]byte(issueResult), &issue); err != nil {
		t.Fatalf("unmarshal issue result: %v", err)
	}
	if issue["title"] != "Test issue" {
		t.Fatalf("expected title 'Test issue', got %v", issue["title"])
	}

	// List issues
	issuesResult := bridge.callTool(t, "list_issues", map[string]any{
		"repo": "test-repo",
	})
	var issues []map[string]any
	if err := json.Unmarshal([]byte(issuesResult), &issues); err != nil {
		t.Fatalf("unmarshal issues result: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	// Get issue
	issueNumber := int(issue["number"].(float64))
	getIssueResult := bridge.callTool(t, "get_issue", map[string]any{
		"repo":   "test-repo",
		"number": issueNumber,
	})
	var gotIssue map[string]any
	if err := json.Unmarshal([]byte(getIssueResult), &gotIssue); err != nil {
		t.Fatalf("unmarshal get issue result: %v", err)
	}
	if gotIssue["title"] != "Test issue" {
		t.Fatalf("expected title 'Test issue', got %v", gotIssue["title"])
	}

	// Delete repo
	bridge.callTool(t, "delete_repo", map[string]any{
		"repo": "test-repo",
	})

	// Verify deleted
	listResult = bridge.callTool(t, "list_repos", nil)
	var remaining []any
	if err := json.Unmarshal([]byte(listResult), &remaining); err != nil {
		t.Fatalf("unmarshal list after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected empty list after delete, got %d repos", len(remaining))
	}
}
