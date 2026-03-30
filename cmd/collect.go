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
	// for originalPrefix, Meta := range rslts.ModuleMigrations {
	// 	fmt.Printf("Original prefix: %s, Prefix: %s, Dir: %s, UpFile: %s, DownFile: %s\n", originalPrefix, Meta.Prefix, Meta.Dir, Meta.UpFileName, Meta.DownFileName)
	// }
	// fmt.Println("MissedMigrations")
	// for UpFileName, Meta := range rslts.MissedMigrations {
	// 	fmt.Printf("UpFileName: %s, Prefix: %s, Dir: %s, UpFile: %s, DownFile: %s\n", UpFileName, Meta.Prefix, Meta.Dir, Meta.UpFileName, Meta.DownFileName)
	// }
	// fmt.Println("ProjectIncludes")
	// for include, included := range rslts.ProjectIncludes {
	// 	fmt.Printf("Include: %s, Included by: %s\n", include, included)
	// }

	if rslts.MissedFilesCnt != 0 {
		fmt.Printf("there are unregistered migration files pairs (%d), collect:\n", rslts.MissedFilesCnt)
		for _, meta := range rslts.MissedMigrations {
			upFileDir := filepath.Join(meta.Dir, meta.UpFileName)
			downFileDir := filepath.Join(meta.Dir, meta.DownFileName)
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
			if moduleMeta, exists := rslts.ModuleMigrations[meta.Prefix]; exists {
				// if only update of the migration file requires
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
				// question: not sure about what it should do, so instead of actually updating file i'll just clear outdated file so user updates it himself
				if err := os.Truncate(moduleUpFileDir, 0); err != nil {
					return fmt.Errorf("error zeroing file: %w", err)
				}
				if err := os.Truncate(moduleDownFileDir, 0); err != nil {
					return fmt.Errorf("error zeroing file: %w", err)
				}
				Lw(fmt.Sprintf("pair %s.up|down.%s update migration %s.up|down.$%s", meta.Prefix, meta.Ext, moduleMeta.Prefix, moduleMeta.Ext))
			} else {
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
				fmt.Printf("pair %s.up|down.%s save migration: %s.up|down.%s\n", meta.Prefix, meta.Ext, moduleMeta.Prefix, moduleMeta.Ext)

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
					newUpFileDir,
					upFileDir,
					fmt.Sprintf("#migration: %s;%s\n", relativeUpFile, md5UpDown),
				); err != nil {
					return fmt.Errorf("error creating migration and writing in meta: %w", err)
				}
				// fmt.Printf("copying from %s to %s\n", upFileDir, newUpFileDir)

				// fmt.Printf("writing into file %s information %s\n", newDownFileDir, fmt.Sprintf("#migration: %s;%s",
				// 	relativeDownFile,
				// 	md5UpDown),
				// )

				if err := updateMigration(
					newDownFileDir,
					downFileDir,
					fmt.Sprintf("#migration: %s;%s\n", relativeDownFile, md5UpDown),
				); err != nil {
					return fmt.Errorf("error creating migration and writing in meta: %w", err)
				}

				// fmt.Printf("copying from %s to %s\n", downFileDir, downFileDir)

				collectedCnt++
				collectedCnt++
			}
		}
	}

	if rslts.MissedIncludesCnt != 0 {
		fmt.Printf("there is number of missed includes (%d)\n", rslts.MissedIncludesCnt)
		// for include, included := range rslts.MissedIncludes {
		// 	fmt.Printf("include %s included by %s", include, included)
		// }
		// if included file is not an original file
		// if include exists in migration dir but is absent in migration-file then we should update migration-file by copying original migration-file
		// if include exists in migration dir and is being referenced by migration-file but it's changed so we should update include by copying original include
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
				fmt.Printf("add include file %s", include)
				if err := updateMigration(newIncludeFile, include, ""); err != nil {
					return fmt.Errorf("error adding include: %w", err)
				}
				// project_includes[${included_relative_file}]=${included_file}
				// rslts.ProjectIncludes[newIncludeFile] = include
				collectedCnt++
			}
		}
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
