package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile      string
	Help         string
	MiniHelpDir  string
	MigrationDir string
	TestDir      string //unnecessary
	IncludeHelp  bool

	rootCmd = &cobra.Command{
		Use:     "migration",
		Short:   "migration cli info short",
		Long:    `migration cli info long`,
		Version: "0.1",
	}
)

// const (
// 	Help = `migration helper to create migrations scripts
// usage: migration [-h|--help] [-V|--version] add
// options:
//         -h|--help      print this help and exit
//         -V|--version   print script version and exit
// commands:
//         add            add new migrations script with properly defined name
//         collect        collect migrations on submodules between commits into migrations catalog
//         check          check unregtistered migrations files at submodules`
// 	MiniHelpDir  = "migration.template.sql"
// 	MigrationDir = "./migrations"
// 	IncludeHelp  = true
// 	TestDir      = "./test"
// )

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig, loadConfigToConstants)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file is:")

	rootCmd.Flags().BoolP("version", "V", false, "print script version and exit")
	rootCmd.Flags().BoolP("help", "h", false, "print this help and exit")

	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(collectCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")

		home, err := os.UserHomeDir()
		cobra.CheckErr(err)
		viper.AddConfigPath(home)

		viper.SetConfigName("config")

		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	} else {
		fmt.Fprintln(os.Stderr, "No config file found, using defaults")
		setDefaults()
	}
}

func setDefaults() {
	viper.SetDefault("help.full", `migration helper to create migrations scripts
usage: migration [-h|--help] [-V|--version] add
options:
        -h|--help      print this help and exit
        -V|--version   print script version and exit
commands:
        add            add new migrations script with properly defined name
        collect        collect migrations on submodules between commits into migrations catalog
        check          check unregtistered migrations files at submodules`)

	viper.SetDefault("help.include", true)
	viper.SetDefault("directories.mini_help", "migration.template.sql")
	viper.SetDefault("directories.migrations", "./migrations")
	viper.SetDefault("app.name", "migration-go")
	viper.SetDefault("app.version", "0.1")
}

func loadConfigToConstants() {
	Help = viper.GetString("help.full")

	MiniHelpDir = viper.GetString("directories.mini_help")
	MigrationDir = viper.GetString("directories.migrations")

	IncludeHelp = viper.GetBool("help.include")

	if version := viper.GetString("app.version"); version != "" {
		rootCmd.Version = version
	}
}
