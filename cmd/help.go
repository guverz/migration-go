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
		fmt.Println(`migration helper to create migrations scripts
usage: migration [-h|--help] [-V|--version] add
options:
        -h|--help      print this help and exit
        -V|--version   print script version and exit
commands:
        add            add new migrations script with properly defined name
        collect        collect migrations on submodules between commits into migrations catalog
        check          check unregtistered migrations files at submodules`)
	},
}
