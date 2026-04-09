package cmd

import (
	"cobra-migration/pkg/migration"
	"fmt"

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
	rslts := &migration.ListResults{}
	collectedCnt := 0
	var listError error
	err := migration.MigrationList(migration.MigrationDir, rslts)
	if err != nil {
		listError = fmt.Errorf("migrationList failed: %w", err)
		// return fmt.Errorf("migrationList failed: %w", err)
	}

	if rslts.MissedFilesCnt != 0 {
		migration.Ld(fmt.Sprintf("there are unregistered migration files pairs (%d), collecting:\n", rslts.MissedFilesCnt))
		collected, err := migration.MissedFiles(rslts)
		if err != nil {
			return err
		}
		collectedCnt += collected
	}
	if rslts.MissedIncludesCnt != 0 {
		migration.Ld(fmt.Sprintf("the number of missed includes is (%d)\n", rslts.MissedIncludesCnt))
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
	if rslts.DeletedIncludesCnt != 0 {
		migration.Ld(fmt.Sprintf("the number of deleted includes is %d", rslts.DeletedIncludesCnt))
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
