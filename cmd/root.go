package cmd

import (
	"fmt"

	"github.com/guverz/migration-go/pkg/migration"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	rootCmd = &cobra.Command{
		Use:          "migration",
		Short:        "migration cli info short",
		Long:         `migration cli info long`,
		Version:      "0.1",
		SilenceUsage: true,
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(migration.InitConfig, migration.LoadConfigToConstants)

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Println(`migration helper to create migrations scripts
usage: migration [-h|--help] [-V|--version] add
options:
        -h|--help      print this help and exit
        -V|--version   print script version and exit
commands:
        add            add new migrations script with properly defined name
        collect        collect migrations on submodules between commits into migrations catalog
        check          check unregtistered migrations files at submodules`)
	})

	rootCmd.PersistentFlags().StringVar(&migration.CfgFile, "config", "", "config file is:")
	rootCmd.PersistentFlags().BoolVar(&migration.NoColor, "no-color", false, "non-color logs")
	rootCmd.PersistentFlags().BoolVarP(&migration.Debug, "debug", "d", false, "debug")
	rootCmd.Flags().BoolP("version", "V", false, "print script version and exit")
	rootCmd.Flags().BoolP("help", "h", false, "print this help and exit")

}
