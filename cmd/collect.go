package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(collectCmd)
}

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "temp",
	Long:  `temp`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("temp collect")
	},
}
