package mcpbridge

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWebhookTools(s *server.MCPServer, client *apiClient) {
	s.AddTool(
		mcp.NewTool("list_webhooks",
			mcp.WithDescription("List webhooks for a repository"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do(ctx, "GET", fmt.Sprintf("/api/v1/repos/%s/webhooks", repo), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("create_webhook",
			mcp.WithDescription("Create a webhook for a repository"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("url", mcp.Required(), mcp.Description("Webhook URL")),
			mcp.WithArray("events", mcp.Description("Events to trigger on"), mcp.WithStringItems()),
			mcp.WithString("content_type", mcp.Description("Content type (json or form)")),
			mcp.WithBoolean("active", mcp.Description("Whether the webhook is active")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			url, err := request.RequireString("url")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{"url": url}
			if args := request.GetArguments(); args != nil {
				if v, ok := args["events"]; ok {
					payload["events"] = v
				}
				if v, ok := args["active"]; ok {
					payload["active"] = v
				}
			}
			if v := request.GetString("content_type", ""); v != "" {
				payload["content_type"] = v
			}
			body, status, err := client.do(ctx, "POST", fmt.Sprintf("/api/v1/repos/%s/webhooks", repo), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("delete_webhook",
			mcp.WithDescription("Delete a webhook"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("id", mcp.Required(), mcp.Description("Webhook ID")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			id, err := request.RequireInt("id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do(ctx, "DELETE", fmt.Sprintf("/api/v1/repos/%s/webhooks/%d", repo, id), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)
}
