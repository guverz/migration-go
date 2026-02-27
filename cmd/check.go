package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "temp",
	Long:  `temp`,
	Run: func(cmd *cobra.Command, args []string) {
		t, _ := Describe("project")
		fmt.Println(t)
	},
}
