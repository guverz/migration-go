package cmd

import (
	"fmt"

	"github.com/guverz/migration-go/pkg/migration"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(collectCmd)
}

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "short collect info",
	Long:  `long collect info`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return collect()
	},
}

func collect() error {
	collectedCnt := 0
	var listError error
	rslts, err := migration.MigrationList(migration.MigrationDir)
	if err != nil {
		listError = fmt.Errorf("migrationList failed: %w", err)
		// return fmt.Errorf("migrationList failed: %w", err)
	}

	if len(rslts.MissedFiles) != 0 {
		migration.Ld(fmt.Sprintf("there are unregistered migration files pairs (%d), collecting:\n", len(rslts.MissedFiles)))
		collected, err := migration.MissedFiles(rslts)
		if err != nil {
			return err
		}
		collectedCnt += collected
	}
	if len(rslts.MissedIncludes) != 0 {
		migration.Ld(fmt.Sprintf("the number of missed includes is (%d)\n", len(rslts.MissedIncludes)))
		collected, err := migration.MissedIncludes(rslts)
		if err != nil {
			return err
		}
		collectedCnt += collected
	}
	if len(rslts.MissedPairs) != 0 {
		migration.Ld(fmt.Sprintf("the number of deleted files is %d", len(rslts.DeletedFiles)))
		collected, err := migration.MissedPairs(rslts)
		if err != nil {
			return err
		}
		collectedCnt += collected
	}
	if len(rslts.DeletedIncludes) != 0 {
		migration.Ld(fmt.Sprintf("the number of deleted includes is %d", len(rslts.DeletedIncludes)))
		collected, err := migration.DeletedIncludes(rslts)
		if err != nil {
			return err
		}
		collectedCnt += collected
	}
	if len(rslts.DeletedFiles) != 0 {
		migration.Ld(fmt.Sprintf("the number of  deleted files is %d", len(rslts.DeletedFiles)))
		collected, err := migration.DeletedFiles(rslts)
		if err != nil {
			return err
		}
		collectedCnt += collected
	}

	// if err := migration.MigrationValidation(migration.MigrationDir); err != nil {
	// return fmt.Errorf("error MigrationValidation: %w", err)
	// }

	if collectedCnt != 0 {
		fmt.Printf("%s: %s\n",
			migration.Colorize("[OK]", migration.Green),
			migration.Colorize(fmt.Sprintf("collected %d file(s)", collectedCnt), migration.Reset),
		)
	} else {
		fmt.Printf("%s: %s\n",
			migration.Colorize("[OK]", migration.Green),
			migration.Colorize("nothing to collect", migration.Reset),
		)
	}

	if listError != nil {
		return listError
	}

	return nil
}
