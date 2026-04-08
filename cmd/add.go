package cmd

import (
	"cobra-migration/pkg/migration"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "temp",
	Long:  `temp`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return add()
	},
}

func add() error {
	if _, err := migration.Add(migration.IncludeHelp); err != nil {
		return err
	}
	return nil
}
