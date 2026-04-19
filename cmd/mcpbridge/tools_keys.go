package mcpbridge

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerKeyTools(s *server.MCPServer, client *apiClient) {
	s.AddTool(
		mcp.NewTool("list_ssh_keys",
			mcp.WithDescription("List SSH keys for the authenticated user"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			body, status, err := client.do(ctx, "GET", "/api/v1/user/keys", nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("add_ssh_key",
			mcp.WithDescription("Add an SSH key"),
			mcp.WithString("key", mcp.Required(), mcp.Description("SSH public key")),
			mcp.WithString("name", mcp.Description("Key name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			key, err := request.RequireString("key")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{"key": key}
			if v := request.GetString("name", ""); v != "" {
				payload["name"] = v
			}
			body, status, err := client.do(ctx, "POST", "/api/v1/user/keys", payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("delete_ssh_key",
			mcp.WithDescription("Delete an SSH key"),
			mcp.WithNumber("id", mcp.Required(), mcp.Description("Key ID")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireInt("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do(ctx, "DELETE", fmt.Sprintf("/api/v1/user/keys/%d", id), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)
}
