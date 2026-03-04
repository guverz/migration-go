package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(helpCmd)
}

var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Print help for migration-go",
	Long:  `Print full help for CLI-utility migration-go`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(Help)
	},
}
