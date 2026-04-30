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
	collect := false

	rslt, err := migration.MigrationList(migration.MigrationDir)
	if err != nil {
		return fmt.Errorf("migrationList failed: %w", err)
	}
	for _, error := range rslt.ListWarnings {
		collect = true
		migration.Lw(error)
	}
	// fmt.Println()
	if len(rslt.LostPairs) != 0 {
		migration.Le(fmt.Sprintf("there is number of incomplete pairs (%d), need to fix it by hand:", len(rslt.LostPairs)))
		for missed, existing := range rslt.LostPairs {
			migration.Le(fmt.Sprintf("file %s do not have counterpart %s", existing, missed))
		}
	}
	if len(rslt.MissedFiles) != 0 {
		migration.Lw(fmt.Sprintf("there are unregistered migration files (%d), collect them and commit:", len(rslt.MissedFiles)))
		collect = true
		for file := range rslt.MissedFiles {
			fmt.Printf("\t%s\n", file)
		}
	}
	if len(rslt.MissedIncludes) != 0 {
		migration.Lw(fmt.Sprintf("there is number of unregistered include files (%d), collect them and commit:", len(rslt.MissedIncludes)))
		collect = true
		for include, included := range rslt.MissedIncludes {
			fmt.Printf("\tinclude %s included by %s\n", include, included)
		}
	}
	if len(rslt.MissedPairs) != 0 {
		migration.Lw(fmt.Sprintf("there is number of incomplete pairs (%d), collect them and commit:", len(rslt.MissedPairs)))
		collect = true
		for missed, existing := range rslt.MissedPairs {
			fmt.Printf("file %s do not have counterpart %s\n", existing, missed)
		}
	}
	if len(rslt.DeletedIncludes) != 0 {
		migration.Lw(fmt.Sprintf("there is number of obsolete includes (%d), collect them and commit:", len(rslt.DeletedIncludes)))
		collect = true
		for include, included := range rslt.DeletedIncludes {
			fmt.Printf("\tinclude %s included by %s\n", include, included)
		}
	}
	if len(rslt.DeletedFiles) != 0 {
		migration.Lw(fmt.Sprintf("there is number of obsolete migration files (%d), collect them and commit:", len(rslt.DeletedFiles)))
		collect = true
		for project, module := range rslt.DeletedFiles {
			fmt.Printf("\tmigration file %s missing original file %s\n", project, module)
		}
	}

	switch {
	case collect:
		return fmt.Errorf("use collect command")
		// fmt.Println("do: scripts/migration collect")
	case len(rslt.LostPairs) != 0:
		return fmt.Errorf("only lost pairs left, fix it by hand")
	default:
		fmt.Printf("%s: No errors!\n",
			migration.Colorize("[OK]", migration.Green),
		)
	}

	return nil
}
