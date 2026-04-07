package mcpbridge

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerIssueTools(s *server.MCPServer, client *apiClient) {
	s.AddTool(
		mcp.NewTool("list_issues",
			mcp.WithDescription("List issues for a repository"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/issues", repo), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("create_issue",
			mcp.WithDescription("Create an issue"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Issue title")),
			mcp.WithString("body", mcp.Description("Issue body")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			title, err := request.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{"title": title}
			if v := request.GetString("body", ""); v != "" {
				payload["body"] = v
			}
			body, status, err := client.do("POST", fmt.Sprintf("/api/v1/repos/%s/issues", repo), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_issue",
			mcp.WithDescription("Get issue details"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("Issue number")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			number, err := request.RequireInt("number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/issues/%d", repo, number), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("update_issue",
			mcp.WithDescription("Update an issue"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("Issue number")),
			mcp.WithString("title", mcp.Description("New title")),
			mcp.WithString("body", mcp.Description("New body")),
			mcp.WithString("status", mcp.Description("New status (open, in_progress, closed)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			number, err := request.RequireInt("number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{}
			if v := request.GetString("title", ""); v != "" {
				payload["title"] = v
			}
			if v := request.GetString("body", ""); v != "" {
				payload["body"] = v
			}
			if v := request.GetString("status", ""); v != "" {
				payload["status"] = v
			}
			body, status, err := client.do("PATCH", fmt.Sprintf("/api/v1/repos/%s/issues/%d", repo, number), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("list_issue_comments",
			mcp.WithDescription("List comments on an issue"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("Issue number")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			number, err := request.RequireInt("number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/issues/%d/comments", repo, number), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("create_issue_comment",
			mcp.WithDescription("Add a comment to an issue"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("Issue number")),
			mcp.WithString("body", mcp.Required(), mcp.Description("Comment body")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			number, err := request.RequireInt("number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			commentBody, err := request.RequireString("body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{"body": commentBody}
			body, status, err := client.do("POST", fmt.Sprintf("/api/v1/repos/%s/issues/%d/comments", repo, number), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)
}
