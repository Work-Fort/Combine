package mcpbridge

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func toolResult(body []byte, status int) *mcp.CallToolResult {
	text := string(body)
	if status >= 400 {
		return mcp.NewToolResultError(text)
	}
	return mcp.NewToolResultText(text)
}

func registerRepoTools(s *server.MCPServer, client *apiClient) {
	s.AddTool(
		mcp.NewTool("list_repos",
			mcp.WithDescription("List all repositories"),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			body, status, err := client.do("GET", "/api/v1/repos", nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("create_repo",
			mcp.WithDescription("Create a new repository"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("description", mcp.Description("Repository description")),
			mcp.WithBoolean("private", mcp.Description("Whether the repository is private")),
			mcp.WithString("project_name", mcp.Description("Project name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := request.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{"name": name}
			if v := request.GetString("description", ""); v != "" {
				payload["description"] = v
			}
			if args := request.GetArguments(); args != nil {
				if v, ok := args["private"]; ok {
					payload["private"] = v
				}
			}
			if v := request.GetString("project_name", ""); v != "" {
				payload["project_name"] = v
			}
			body, status, err := client.do("POST", "/api/v1/repos", payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_repo",
			mcp.WithDescription("Get repository details"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s", repo), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("update_repo",
			mcp.WithDescription("Update a repository"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("description", mcp.Description("New description")),
			mcp.WithBoolean("private", mcp.Description("Whether the repository is private")),
			mcp.WithString("project_name", mcp.Description("Project name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{}
			if v := request.GetString("description", ""); v != "" {
				payload["description"] = v
			}
			if args := request.GetArguments(); args != nil {
				if v, ok := args["private"]; ok {
					payload["private"] = v
				}
			}
			if v := request.GetString("project_name", ""); v != "" {
				payload["project_name"] = v
			}
			body, status, err := client.do("PATCH", fmt.Sprintf("/api/v1/repos/%s", repo), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("delete_repo",
			mcp.WithDescription("Delete a repository"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do("DELETE", fmt.Sprintf("/api/v1/repos/%s", repo), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)
}
