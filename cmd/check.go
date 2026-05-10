package cmd

import (
	"fmt"

	"github.com/guverz/migration-go/pkg/migration"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "short check description",
	Long:  `long check description`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return check()
	},
}

func check() error {
	if err := migration.Check(); err != nil {
		return fmt.Errorf("check command failed: %w", err)
	}
	return nil
}
