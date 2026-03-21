package cmd

import (
	"fmt"
	"path/filepath"

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
	rslt := &ListResults{}
	if err := MigrationList(MigrationDir, rslt); err != nil {
		return fmt.Errorf("error MigrationList: %w", err)
	}
	if rslt.MissedFilesCnt != 0 {
		fmt.Printf("there are unregistered migration files pairs (%d), collect:", rslt.MissedFilesCnt)
		for _, meta := range rslt.MissedMigrations {
			upFileDir := filepath.Join(meta.Dir, meta.UpFileName)
			downFileDir := filepath.Join(meta.Dir, meta.DownFileName)
			if rslt, err := FindFileViaDir(upFileDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				// not sure how to both color error and call return
				// le "BUG: there is no file ${file_dir}/${file_name_up}, something wrong"
				// exit 1
				// Le(fmt.Sprintf("BUG: there is no file %s, something wrong", upFileDir))
				return fmt.Errorf("BUG: there is no file %s, something wrong", upFileDir)
			}
			if rslt, err := FindFileViaDir(downFileDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				// not sure how to both color error and call return
				// le "BUG: there is no file ${file_dir}/${file_name_up}, something wrong"
				// exit 1
				// Le(fmt.Sprintf("BUG: there is no file %s, something wrong", downFileDir))
				return fmt.Errorf("BUG: there is no file %s, something wrong", downFileDir)
			}
			// before check name exists in module_migrations file_prefix=>project_migration_file_pair
			for _, moduleMeta := range rslt.ModuleMigrations {
				if moduleMeta.Prefix == meta.Prefix {
					moduleUpFileDir := filepath.Join(moduleMeta.Dir, moduleMeta.UpFileName)
					moduleDownFileDir := filepath.Join(moduleMeta.Dir, moduleMeta.DownFileName)
					if rslt, err := FindFileViaDir(moduleUpFileDir); err != nil {
						return fmt.Errorf("error FindFileViaDir: %w", err)
					} else if !rslt {
						return fmt.Errorf("BUG: there is no file %s, something wrong", moduleUpFileDir)
					}
					if rslt, err := FindFileViaDir(moduleDownFileDir); err != nil {
						return fmt.Errorf("error FindFileViaDir: %w", err)
					} else if !rslt {
						return fmt.Errorf("BUG: there is no file %s, something wrong", moduleDownFileDir)
					}
					// head -1 ${migration_dir}/${migration_name_up} > ${migration_dir}/${migration_name_up}

					Lw(fmt.Sprintf("pair %s.up|down.%s update migration %s.up|down.$%s", meta.Prefix, meta.Ext, moduleMeta.Prefix, moduleMeta.Ext))
					break
				} else {
					originalIncludes := make(map[string]string)
					state := make(map[string]int)
					if err := ParseIncludes(upFileDir, "", state, originalIncludes); err != nil {
						return fmt.Errorf("error ParseIncludes: %w", err)
					}
					if err := ParseIncludes(downFileDir, "", state, originalIncludes); err != nil {
						return fmt.Errorf("error ParseIncludes: %w", err)
					}
					if err := Add(false); err != nil {
						return fmt.Errorf("error Add: %w", err)
					}
					// full, err := Describe(MigrationDir, "full")
					// if err != nil {
					// 	return fmt.Errorf("error Describe: %w", err)
					// }
					// if _, lastMigration, err := FindLastMigrationInfo(MigrationDir, full); err != nil {
					// 	return fmt.Errorf("error FindLastMigrationInfo: %w", err)
					// }
				}
			}
		}
	}
	return nil
}
