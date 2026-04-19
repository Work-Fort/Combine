package mcpbridge

import (
	"fmt"

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
	cmd.Flags().String("api-key", "", "Passport API key (sent as ApiKey-v1)")
	_ = viper.BindPFlag("server-url", cmd.Flags().Lookup("server-url"))
	_ = viper.BindPFlag("api-key", cmd.Flags().Lookup("api-key"))

	return cmd
}

func runBridge(cmd *cobra.Command, args []string) error {
	serverURL := viper.GetString("server-url")
	apiKey := viper.GetString("api-key")
	if apiKey == "" {
		return fmt.Errorf("--api-key is required")
	}

	client := newAPIClient(serverURL, apiKey)

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

	return server.ServeStdio(s)
}
