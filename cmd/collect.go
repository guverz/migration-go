package cmd

import (
	"encoding/hex"
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

	// for originalPrefix, Meta := range rslts.ModuleMigrations {
	// 	fmt.Printf("Original prefix: %s, Prefix: %s, Dir: %s, UpFile: %s, DownFile: %s\n", originalPrefix, Meta.Prefix, Meta.Dir, Meta.UpFileName, Meta.DownFileName)
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
				// head -1 ${migration_dir}/${migration_name_up} > ${migration_dir}/${migration_name_up}
				// head -1 ${migration_dir}/${migration_name_down} > ${migration_dir}/${migration_name_down}
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
					Ld(fmt.Sprintf("md5 %x of include %s included by %s and check in migrationIncludes", md5, include, included))
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
				md5UpHex := hex.EncodeToString(md5Up[:])
				md5DownHex := hex.EncodeToString(md5Down[:])

				md5UpDown := md5UpHex + md5DownHex

				temp := strings.Split(upFileDir, string(filepath.Separator))
				relativeUpFile := strings.Join(temp[1:], string(filepath.Separator))

				temp = strings.Split(downFileDir, string(filepath.Separator))
				relativeDownFile := strings.Join(temp[1:], string(filepath.Separator))

				// fmt.Printf("writing into file %s information %s\n", newUpFileDir, fmt.Sprintf("#migration: %s;%s",
				// 	relativeUpFile,
				// 	md5UpDown),
				// )

				newUpFile, err := os.OpenFile(newUpFileDir, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer newUpFile.Close()

				_, err = fmt.Fprintf(newUpFile, "#migration: %s;%s\n", relativeUpFile, md5UpDown)
				if err != nil {
					return err
				}

				upFile, err := os.Open(upFileDir)
				if err != nil {
					return err
				}
				defer upFile.Close()

				// fmt.Printf("copying from %s to %s\n", upFileDir, newUpFileDir)
				_, err = io.Copy(newUpFile, upFile)
				if err != nil {
					return err
				}

				// fmt.Printf("writing into file %s information %s\n", newDownFileDir, fmt.Sprintf("#migration: %s;%s",
				// 	relativeDownFile,
				// 	md5UpDown),
				// )

				newDownFile, err := os.OpenFile(newDownFileDir, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer newDownFile.Close()

				_, err = fmt.Fprintf(newDownFile, "#migration: %s;%s\n", relativeDownFile, md5UpDown)
				if err != nil {
					return err
				}

				downFile, err := os.Open(downFileDir)
				if err != nil {
					return err
				}
				defer downFile.Close()

				// fmt.Printf("copying from %s to %s\n", downFileDir, downFileDir)
				_, err = io.Copy(newDownFile, downFile)
				if err != nil {
					return err
				}

				collectedCnt++
				collectedCnt++
			}
		}
	}

	// if rslts.MissedIncludesCnt != 0 {
	// 	fmt.Printf("there is number of missed includes (%d)\n", rslts.MissedIncludesCnt)
	// 	for include, included := range rslts.MissedIncludes {
	// 		Ld(fmt.Sprintf("include file %s", include))
	// 		md5, err := FileMD5(include)
	// 		if err != nil {
	// 			return fmt.Errorf("error FileMD5: %w", err)
	// 		}
	// 		Ld(fmt.Sprintf("md5 %x of include file %s included by %s and check in projectIncludes", md5, include, included))
	// 		parts := strings.Split(include, string(filepath.Separator))
	// 		includeRelativeFile := strings.Join(parts[1:], string(filepath.Separator))
	// 		if rslt, err := FindFileViaDir(includeRelativeFile); err != nil {
	// 			return fmt.Errorf("error FindFileViaDir: %w", err)
	// 		} else if !rslt {
	// 			if err := os.Mkdir(includeRelativeFile, 0644); err != nil {
	// 				return fmt.Errorf("error creating dir: %w", err)
	// 			}
	// 		}
	// 		if _, exists := rslts.ProjectIncludes[includeRelativeFile]; exists {
	// 			// originally md5 is being calculated using the same file, definitely a bug
	// 		}
	// 	}
	// }

	return nil
}
