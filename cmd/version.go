package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(helpCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print out version of migration-go",
	Long:  `Print out version of CLI-utility migration-go`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("0.0.1")
	},
}
