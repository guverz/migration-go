package cmd

import (
	"fmt"

	"github.com/guverz/migration-go/pkg/migration"

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
	if err := migration.Add(); err != nil {
		return fmt.Errorf("error adding migration pair: %w", err)
	}
	return nil
}
