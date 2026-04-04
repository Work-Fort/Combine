package admin

import (
	"fmt"

	"github.com/Work-Fort/Combine/cmd"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/internal/config"
	"github.com/spf13/cobra"
)

var (
	// Command is the admin command.
	Command = &cobra.Command{
		Use:   "admin",
		Short: "Administrate the server",
	}

	migrateCmd = &cobra.Command{
		Use:                "migrate",
		Short:              "Migrate the database to the latest version",
		Long:               "Migrations are now applied automatically when the server starts. This command ensures the store is opened (which triggers Goose migrations) and then exits.",
		PersistentPreRunE:  cmd.ChainedInitBackendContext,
		PersistentPostRunE: cmd.CloseStoreContext,
		RunE: func(c *cobra.Command, _ []string) error {
			// Goose migrations run automatically in infra.Open(), which is
			// called by InitBackendContext. Nothing else to do here.
			fmt.Fprintln(c.OutOrStdout(), "Migrations are up to date.")
			return nil
		},
	}

	syncHooksCmd = &cobra.Command{
		Use:                "sync-hooks",
		Short:              "Update repository hooks",
		PersistentPreRunE:  cmd.ChainedInitBackendContext,
		PersistentPostRunE: cmd.CloseStoreContext,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			cfg := config.FromContext(ctx)
			be := backend.FromContext(ctx)
			if err := cmd.InitializeHooks(ctx, cfg, be); err != nil {
				return fmt.Errorf("initialize hooks: %w", err)
			}

			return nil
		},
	}
)

func init() {
	Command.AddCommand(
		syncHooksCmd,
		migrateCmd,
	)
}
