package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile      string
	MiniHelpDir  string
	MigrationDir string
	IncludeHelp  bool
	ConcLimit    int
	// Verbose      bool
	Debug   bool
	NoColor bool
	rootCmd = &cobra.Command{
		Use:          "migration",
		Short:        "migration cli info short",
		Long:         `migration cli info long`,
		Version:      "0.1",
		SilenceUsage: true,
	}
)

const (
	red    = "\033[31m"
	yellow = "\033[33m"
	purple = "\033[35m"
	bold   = "\033[1m"
	reset  = "\033[0m"
	green  = "\033[32m"
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig, loadConfigToConstants)

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

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file is:")
	// rootCmd.Flags().BoolVarP(&Verbose, "verbose", "v", false, "verbose")
	rootCmd.PersistentFlags().BoolVar(&NoColor, "no-color", false, "non-color logs")
	rootCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "debug")
	rootCmd.Flags().BoolP("version", "V", false, "print script version and exit")
	rootCmd.Flags().BoolP("help", "h", false, "print this help and exit")

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
	viper.SetDefault("help.include", true)
	viper.SetDefault("directories.mini_help", "migration.template.sql")
	viper.SetDefault("directories.migrations", "./migrations")
}

func loadConfigToConstants() {
	MiniHelpDir = viper.GetString("directories.mini_help")
	MigrationDir = viper.GetString("directories.migrations")
	ConcLimit = viper.GetInt("app.conc_limit")
	IncludeHelp = viper.GetBool("help.include")
}

func Ld(msg string) {
	if Debug {
		fmt.Printf("%s: %s\n",
			colorize("DEBUG", yellow+bold),
			colorize(msg, yellow),
		)
	}
}

func Lw(msg string) {
	fmt.Printf("%s: %s\n",
		colorize("WARNING", purple),
		colorize(msg, ""),
	)
}

func Le(msg string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n",
		colorize("ERROR", red+bold),
		colorize(msg, red),
	)
}

func colorize(s, color string) string {
	if NoColor {
		return s
	}
	return color + s + reset
}
