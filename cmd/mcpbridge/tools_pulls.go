package mcpbridge

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerPullRequestTools(s *server.MCPServer, client *apiClient) {
	s.AddTool(
		mcp.NewTool("list_pull_requests",
			mcp.WithDescription("List pull requests for a repository"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			repo, err := request.RequireString("repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/pulls", repo), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("create_pull_request",
			mcp.WithDescription("Create a pull request"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("title", mcp.Required(), mcp.Description("PR title")),
			mcp.WithString("head", mcp.Required(), mcp.Description("Source branch")),
			mcp.WithString("base", mcp.Required(), mcp.Description("Target branch")),
			mcp.WithString("body", mcp.Description("PR body")),
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
			head, err := request.RequireString("head")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			base, err := request.RequireString("base")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{
				"title":  title,
				"head":   head,
				"base":   base,
			}
			if v := request.GetString("body", ""); v != "" {
				payload["body"] = v
			}
			body, status, err := client.do("POST", fmt.Sprintf("/api/v1/repos/%s/pulls", repo), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_pull_request",
			mcp.WithDescription("Get pull request details"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("PR number")),
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
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d", repo, number), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("update_pull_request",
			mcp.WithDescription("Update a pull request"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("PR number")),
			mcp.WithString("title", mcp.Description("New title")),
			mcp.WithString("body", mcp.Description("New body")),
			mcp.WithString("status", mcp.Description("New status")),
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
			body, status, err := client.do("PATCH", fmt.Sprintf("/api/v1/repos/%s/pulls/%d", repo, number), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("merge_pull_request",
			mcp.WithDescription("Merge a pull request"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("PR number")),
			mcp.WithString("strategy", mcp.Description("Merge strategy: merge, squash, rebase")),
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
			if v := request.GetString("strategy", ""); v != "" {
				payload["strategy"] = v
			}
			body, status, err := client.do("POST", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/merge", repo, number), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_pull_request_diff",
			mcp.WithDescription("Get the diff for a pull request"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("PR number")),
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
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/diff", repo, number), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("list_pull_request_files",
			mcp.WithDescription("List files changed in a pull request"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("PR number")),
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
			body, status, err := client.do("GET", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/files", repo, number), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)

	s.AddTool(
		mcp.NewTool("submit_review",
			mcp.WithDescription("Submit a review on a pull request"),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithNumber("number", mcp.Required(), mcp.Description("PR number")),
			mcp.WithString("event", mcp.Required(), mcp.Description("Review event: approve, request_changes, comment")),
			mcp.WithString("body", mcp.Description("Review body")),
			mcp.WithArray("comments", mcp.Description("Line-level review comments")),
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
			event, err := request.RequireString("event")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			payload := map[string]any{"event": event}
			if v := request.GetString("body", ""); v != "" {
				payload["body"] = v
			}
			if args := request.GetArguments(); args != nil {
				if v, ok := args["comments"]; ok {
					payload["comments"] = v
				}
			}
			body, status, err := client.do("POST", fmt.Sprintf("/api/v1/repos/%s/pulls/%d/reviews", repo, number), payload)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return toolResult(body, status), nil
		},
	)
}
