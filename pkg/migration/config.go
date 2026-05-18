package migration

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	CfgFile      string
	Debug        bool
	NoColor      bool
	MiniHelpDir  string
	MigrationDir string
	IncludeHelp  bool
)

func InitConfig() {
	if CfgFile != "" {
		viper.SetConfigFile(CfgFile)
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
	viper.SetDefault("help.include", true)
	viper.SetDefault("directories.mini_help", "migration.template.sql")
	viper.SetDefault("directories.migrations", "./migrations")
}

func LoadConfigToConstants() {
	MiniHelpDir = viper.GetString("directories.mini_help")
	MigrationDir = viper.GetString("directories.migrations")
	IncludeHelp = viper.GetBool("help.include")
}
