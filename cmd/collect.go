package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	rslts := &ListResults{}
	collectedCnt := 0

	if err := MigrationList(MigrationDir, rslts); err != nil {
		return fmt.Errorf("error MigrationList: %w", err)
	}
	// fmt.Println(rslts.MissedFilesCnt, rslts.MissedMigrations)
	// fmt.Println("ModuleMigrations")
	// for originalPrefix, module := range rslts.ModuleMigrations {
	// 	fmt.Printf("Original prefix: %s, Prefix: %s, Dir: %s, UpFile: %s, DownFile: %s\n", originalPrefix, module.Prefix, module.Dir, module.UpFileName, module.DownFileName)
	// }
	// fmt.Println("MissedMigrations")
	// for UpFileName, module := range rslts.MissedMigrations {
	// 	fmt.Printf("UpFileName: %s, Prefix: %s, Dir: %s, UpFile: %s, DownFile: %s\n", UpFileName, module.Prefix, module.Dir, module.UpFileName, module.DownFileName)
	// }
	// fmt.Println("ProjectIncludes")
	// for include, included := range rslts.ProjectIncludes {
	// 	fmt.Printf("Include: %s, Included by: %s\n", include, included)
	// }

	if rslts.MissedFilesCnt != 0 {
		fmt.Printf("there are unregistered migration files pairs (%d), collect:\n", rslts.MissedFilesCnt)
		targetDirUp := ""
		targetDirDown := ""
		for _, module := range rslts.MissedMigrations {
			upFileDir := filepath.Join(module.Dir, module.UpFileName)
			downFileDir := filepath.Join(module.Dir, module.DownFileName)
			if rslt, err := FindFileViaDir(upFileDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				return fmt.Errorf("BUG: there is no file %s, something wrong", upFileDir)
			}
			if rslt, err := FindFileViaDir(downFileDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				return fmt.Errorf("BUG: there is no file %s, something wrong", downFileDir)
			}
			// before check name exists in module_migrations file_prefix=>project_migration_file_pair
			if migration, exists := rslts.ModuleMigrations[module.Prefix]; exists {
				// if only update of the migration file requires
				migrationUpFileDir := filepath.Join(migration.Dir, migration.UpFileName)
				migrationDownFileDir := filepath.Join(migration.Dir, migration.DownFileName)

				if rslt, err := FindFileViaDir(migrationUpFileDir); err != nil {
					return fmt.Errorf("error FindFileViaDir: %w", err)
				} else if !rslt {
					return fmt.Errorf("BUG: there is no file %s, something wrong", migrationUpFileDir)
				}
				if rslt, err := FindFileViaDir(migrationDownFileDir); err != nil {
					return fmt.Errorf("error FindFileViaDir: %w", err)
				} else if !rslt {
					return fmt.Errorf("BUG: there is no file %s, something wrong", migrationDownFileDir)
				}

				if err := os.Truncate(migrationUpFileDir, 0); err != nil {
					return fmt.Errorf("error zeroing file: %w", err)
				}

				if err := os.Truncate(migrationDownFileDir, 0); err != nil {
					return fmt.Errorf("error zeroing file: %w", err)
				}

				targetDirUp = migrationUpFileDir
				targetDirDown = migrationDownFileDir

				Lw(fmt.Sprintf("pair %s.up|down.%s update migration %s.up|down.%s", module.Prefix, module.Ext, migration.Prefix, migration.Ext))
			} else {
				// if new migration pair is required
				fmt.Println("creating new migration file")
				originalIncludes := make(map[string]string)
				state := make(map[string]int)

				if err := ParseIncludes(upFileDir, "", state, originalIncludes); err != nil {
					return fmt.Errorf("error ParseIncludes: %w", err)
				}
				if err := ParseIncludes(downFileDir, "", state, originalIncludes); err != nil {
					return fmt.Errorf("error ParseIncludes: %w", err)
				}

				migrationTemplate, err := Add(false)
				if err != nil {
					return fmt.Errorf("error Add: %w", err)
				}
				newUpFileName := fmt.Sprintf("%s.up.sql", migrationTemplate)
				newDownFileName := fmt.Sprintf("%s.down.sql", migrationTemplate)
				newUpFileDir := filepath.Join(MigrationDir, newUpFileName)
				newDownFileDir := filepath.Join(MigrationDir, newDownFileName)

				targetDirUp = newUpFileDir
				targetDirDown = newDownFileDir

				if rslt, err := FindFileViaDir(newUpFileDir); err != nil {
					return fmt.Errorf("error FindFileViaDir: %w", err)
				} else if !rslt {
					return fmt.Errorf("BUG: there is no file %s, something wrong", newUpFileDir)
				}
				if rslt, err := FindFileViaDir(newDownFileDir); err != nil {
					return fmt.Errorf("error FindFileViaDir: %w", err)
				} else if !rslt {
					return fmt.Errorf("BUG: there is no file %s, something wrong", newDownFileDir)
				}
				// compare includes and fill missed_includes or deleted includes
				for include, included := range originalIncludes {
					Ld(fmt.Sprintf("include file %s", include))

					md5, err := FileMD5(include)
					if err != nil {
						return fmt.Errorf("error FileMD5: %w", err)
					}
					Ld(fmt.Sprintf("md5 %s of include %s included by %s and check in migrationIncludes", md5, include, included))
					if _, exists := rslts.ProjectMD5Includes[md5]; !exists {
						if _, exists := rslts.MissedIncludes[include]; !exists {
							Ld(fmt.Sprintf("include file %s is changed or doesn't exist in migration includes", include))
							rslts.MissedIncludes[include] = included
							rslts.MissedIncludesCnt++
						}
					}
				}
				// strange behaviour: pair ClearingManager-roam-cdr-0.9.7~rc.5-1-1.up|down.sql save migration: .up|down.
				fmt.Printf("pair %s.up|down.%s save migration: %s.up|down.%s\n", module.Prefix, module.Ext, migration.Prefix, migration.Ext)
			}

			md5Up, err := FileMD5(upFileDir)
			if err != nil {
				return fmt.Errorf("error FileMD5: %w", err)
			}
			md5Down, err := FileMD5(downFileDir)
			if err != nil {
				return fmt.Errorf("error FileMD5: %w", err)
			}

			md5UpDown := md5Up + md5Down

			relativeUpFile := StripDir(upFileDir)
			relativeDownFile := StripDir(downFileDir)

			// fmt.Printf("writing into file %s information %s\n", newUpFileDir, fmt.Sprintf("#migration: %s;%s",
			// 	relativeUpFile,
			// 	md5UpDown),
			// )

			if err := updateMigration(
				targetDirUp,
				upFileDir,
				fmt.Sprintf("#migration: %s;%s\n", relativeUpFile, md5UpDown),
			); err != nil {
				return fmt.Errorf("error creating migration and writing in module: %w", err)
			}
			// fmt.Printf("copying from %s to %s\n", upFileDir, newUpFileDir)

			// fmt.Printf("writing into file %s information %s\n", newDownFileDir, fmt.Sprintf("#migration: %s;%s",
			// 	relativeDownFile,
			// 	md5UpDown),
			// )

			if err := updateMigration(
				targetDirDown,
				downFileDir,
				fmt.Sprintf("#migration: %s;%s\n", relativeDownFile, md5UpDown),
			); err != nil {
				return fmt.Errorf("error creating migration and writing in module: %w", err)
			}

			// fmt.Printf("copying from %s to %s\n", downFileDir, downFileDir)

			collectedCnt++
			collectedCnt++

		}
	}

	if rslts.MissedIncludesCnt != 0 {
		fmt.Printf("there is number of missed includes (%d)\n", rslts.MissedIncludesCnt)
		// for include, included := range rslts.MissedIncludes {
		// 	fmt.Printf("include %s included by %s", include, included)
		// }
		// if included file is not an original file
		// if include exists in migration dir but is absent in migration-file then we should update migration-file by copying original migration-file - unrealistic scenario
		// if include exists in migration dir and is being referenced by migration-file but it's changed so we should update include by copying original include - realistic scenario, works fine
		// if included file is an original file
		// if
		for include, included := range rslts.MissedIncludes {
			Ld(fmt.Sprintf("include file %s", include))
			md5, err := FileMD5(include)
			if err != nil {
				return fmt.Errorf("error FileMD5: %w", err)
			}
			Ld(fmt.Sprintf("md5 %x of include file %s included by %s and check in rslts.ProjectIncludes", md5, include, included))

			// includeRelativeFile := StripDir(include)
			// includeRelativePath := filepath.Dir(includeRelativeFile)

			rawInclude := include
			marker := "migrations" + string(filepath.Separator)
			if idx := strings.Index(rawInclude, marker); idx != -1 {
				rawInclude = rawInclude[idx+len(marker):]
			}
			newIncludeFile := filepath.Join(MigrationDir, rawInclude)
			newIncludeDir := filepath.Dir(newIncludeFile)

			if rslt, err := FindFileViaDir(newIncludeDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				if err := os.Mkdir(newIncludeDir, 0644); err != nil {
					return fmt.Errorf("error creating dir: %w", err)
				}
			}
			if _, exists := rslts.ProjectIncludes[newIncludeFile]; exists {
				projectMD5, err := FileMD5(newIncludeFile)
				if err != nil {
					return fmt.Errorf("error FileMD5: %w", err)
				}
				if md5 != projectMD5 {
					Lw(fmt.Sprintf("%s there is in rslts.ProjectIncludes[%s], it was changed, replace it", newIncludeFile, newIncludeFile))
					if err := updateMigration(newIncludeFile, include, ""); err != nil {
						return fmt.Errorf("error updating include: %w", err)
					}
					collectedCnt++
				}
			} else {
				fmt.Printf("add include file %s\n", include)

				if err := os.Truncate(newIncludeFile, 0); err != nil {
					return fmt.Errorf("error zeroing file: %w", err)
				}

				if err := updateMigration(newIncludeFile, include, ""); err != nil {
					return fmt.Errorf("error adding include: %w", err)
				}
				// project_includes[${included_relative_file}]=${included_file}
				// rslts.ProjectIncludes[newIncludeFile] = include
				collectedCnt++
			}
		}
	}
	if rslts.DeletedIncludesCnt != 0 {
		Ld(fmt.Sprintf("there is deleted includes %d", rslts.DeletedIncludesCnt))
		for include, included := range rslts.DeletedIncludes {
			fmt.Printf("include file %s included by %s\n", include, included)
			// check file exists in rslts.ProjectIncludes, if inlcuded by some one else, then do not delete
			_, exists1 := rslts.ProjectIncludes[include]
			_, exists2 := rslts.ModuleIncludes[include]
			if !exists1 && !exists2 {
				// WARNING: dangerous operation, check before delete
				if fileInfo, err := os.Stat(include); err == nil && fileInfo.Mode().IsRegular() {
					Le(fmt.Sprintf("delete include %s", include))
					if err := os.Remove(include); err != nil {
						return fmt.Errorf("failed to delete: %w", err)
					}
					collectedCnt++
				}
			}
		}
	}

	if err := MigrationValidation(MigrationDir); err != nil {
		return fmt.Errorf("error MigrationValidation: %w", err)
	}

	if collectedCnt != 0 {
		fmt.Printf("%s: %s\n",
			colorize("[OK]", green),
			colorize(fmt.Sprintf("collected %d file(s)", collectedCnt), reset),
		)
	} else {
		fmt.Printf("%s: %s\n",
			colorize("[OK]", green),
			colorize("nothing to collect", reset),
		)
	}

	return nil
}

func StripDir(fileDir string) string {
	temp := strings.Split(fileDir, string(filepath.Separator))
	return strings.Join(temp[1:], string(filepath.Separator))
}

func updateMigration(newFilePath, srcFilePath, header string) error {
	newFile, err := os.OpenFile(newFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer newFile.Close()

	if _, err = newFile.WriteString(header); err != nil {
		return err
	}

	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if _, err = io.Copy(newFile, srcFile); err != nil {
		return err
	}

	return nil
}
