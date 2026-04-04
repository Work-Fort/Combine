package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/cmd/combine/admin"
	"github.com/Work-Fort/Combine/cmd/combine/hook"
	"github.com/Work-Fort/Combine/cmd/combine/serve"
	"github.com/Work-Fort/Combine/pkg/config"
	logr "github.com/Work-Fort/Combine/pkg/log"
	"github.com/Work-Fort/Combine/pkg/version"
	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"
	"go.uber.org/automaxprocs/maxprocs"
)

var (
	// Version contains the application version number. It's set via ldflags
	// when building.
	Version = ""

	// CommitSHA contains the SHA of the commit that this application was built
	// against. It's set via ldflags when building.
	CommitSHA = ""

	// CommitDate contains the date of the commit that this application was
	// built against. It's set via ldflags when building.
	CommitDate = ""

	rootCmd = &cobra.Command{
		Use:          "combine",
		Short:        "A self-hostable Git forge",
		Long:         "Combine is a self-hostable Git forge for the WorkFort platform.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	manCmd = &cobra.Command{
		Use:    "man",
		Short:  "Generate man pages",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			manPage, err := mcobra.NewManPage(1, rootCmd) //.
			if err != nil {
				return err
			}

			manPage = manPage.WithSection("Copyright", "(C) 2021-2023 Charmbracelet, Inc.\n"+
				"(C) 2026 WorkFort\n"+
				"Released under MIT license.")
			fmt.Println(manPage.Build(roff.NewDocument()))
			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(
		manCmd,
		serve.Command,
		hook.Command,
		admin.Command,
	)
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

func main() {
	ctx := context.Background()
	cfg := config.DefaultConfig()
	if cfg.Exist() {
		if err := cfg.Parse(); err != nil {
			log.Fatal(err)
		}
	}

	if err := cfg.ParseEnv(); err != nil {
		log.Fatal(err)
	}

	ctx = config.WithContext(ctx, cfg)
	logger, f, err := logr.NewLogger(cfg)
	if err != nil {
		log.Errorf("failed to create logger: %v", err)
	}

	ctx = log.WithContext(ctx, logger)
	if f != nil {
		defer f.Close() //nolint: errcheck
	}

	// Set global logger
	log.SetDefault(logger)

	var opts []maxprocs.Option
	if config.IsVerbose() {
		opts = append(opts, maxprocs.Logger(log.Debugf))
	}

	// Set the max number of processes to the number of CPUs
	// This is useful when running Combine in a container
	if _, err := maxprocs.Set(opts...); err != nil {
		log.Warn("couldn't set automaxprocs", "error", err)
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
