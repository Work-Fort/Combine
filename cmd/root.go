package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/config"
	"github.com/Work-Fort/Combine/internal/infra/version"
	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"
)

var (
	// Version contains the application version number. It's set via ldflags.
	Version = ""

	// CommitSHA contains the SHA of the commit this was built against.
	CommitSHA = ""

	// CommitDate contains the date of the commit.
	CommitDate = ""

	rootCmd = &cobra.Command{
		Use:          "combine",
		Short:        "A self-hostable Git forge",
		Long:         "Combine is a self-hostable Git forge for the WorkFort platform.",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 1. Ensure XDG dirs exist
			if err := config.InitDirs(); err != nil {
				return err
			}

			// 2. Load config file (YAML)
			if err := config.LoadConfig(); err != nil {
				return err
			}

			// 3. Build Config from viper and attach to context
			cfg, err := config.FromViper()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cmd.SetContext(config.WithContext(cmd.Context(), cfg))

			// 4. Create logger
			logger, f, err := newLogger(cfg)
			if err != nil {
				log.Errorf("failed to create logger: %v", err)
			} else {
				log.SetDefault(logger)
				cmd.SetContext(log.WithContext(cmd.Context(), logger))
				if f != nil {
					// The file will be closed when the process exits.
					// For long-lived daemons the file stays open.
					_ = f
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
)

func init() {
	config.InitViper()

	rootCmd.AddCommand(
		NewDaemonCmd(),
		NewServeCmd(), // alias for daemon, for backward compat with E2E tests
	)
	// admin and hook commands are added in main.go

	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	if len(CommitSHA) >= 7 {
		vt := rootCmd.VersionTemplate()
		rootCmd.SetVersionTemplate(vt[:len(vt)-1] + " (" + CommitSHA[0:7] + ")\n")
	}
	if Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			Version = info.Main.Version
		} else {
			Version = "unknown (built from source)"
		}
	}
	rootCmd.Version = Version

	version.Version = Version
	version.CommitSHA = CommitSHA
	version.CommitDate = CommitDate
}

// RootCmd returns the root cobra command for adding subcommands externally.
func RootCmd() *cobra.Command {
	return rootCmd
}

// Execute runs the root command.
func Execute() {
	// Set automaxprocs
	var opts []maxprocs.Option
	if config.IsVerbose() {
		opts = append(opts, maxprocs.Logger(log.Debugf))
	}
	if _, err := maxprocs.Set(opts...); err != nil {
		log.Warn("couldn't set automaxprocs", "error", err)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
