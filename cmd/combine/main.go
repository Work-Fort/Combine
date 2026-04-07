package main

import (
	"github.com/Work-Fort/Combine/cmd"
	"github.com/Work-Fort/Combine/cmd/combine/admin"
	"github.com/Work-Fort/Combine/cmd/combine/hook"
	"github.com/Work-Fort/Combine/cmd/mcpbridge"
	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"

	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd := cmd.RootCmd()

	manCmd := &cobra.Command{
		Use:    "man",
		Short:  "Generate man pages",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			manPage, err := mcobra.NewManPage(1, rootCmd)
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

	rootCmd.AddCommand(
		manCmd,
		hook.Command,
		admin.Command,
		mcpbridge.NewCmd(),
	)
}

func main() {
	cmd.Execute()
}
