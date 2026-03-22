package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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
	if rslts.MissedFilesCnt != 0 {
		fmt.Printf("there are unregistered migration files pairs (%d), collect:", rslts.MissedFilesCnt)
		for _, meta := range rslts.MissedMigrations {
			upFileDir := filepath.Join(meta.Dir, meta.UpFileName)
			downFileDir := filepath.Join(meta.Dir, meta.DownFileName)
			if rslt, err := FindFileViaDir(upFileDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				// not sure how to both color error and call return maybe should just color it
				// le "BUG: there is no file ${file_dir}/${file_name_up}, something wrong"
				// exit 1
				// Le(fmt.Sprintf("BUG: there is no file %s, something wrong", upFileDir))
				return fmt.Errorf("BUG: there is no file %s, something wrong", upFileDir)
			}
			if rslt, err := FindFileViaDir(downFileDir); err != nil {
				return fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				// not sure how to both color error and call return maybe should just color it
				// le "BUG: there is no file ${file_dir}/${file_name_up}, something wrong"
				// exit 1
				// Le(fmt.Sprintf("BUG: there is no file %s, something wrong", downFileDir))
				return fmt.Errorf("BUG: there is no file %s, something wrong", downFileDir)
			}
			// before check name exists in module_migrations file_prefix=>project_migration_file_pair
			for _, moduleMeta := range rslts.ModuleMigrations {
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
					// head -1 ${migration_dir}/${migration_name_down} > ${migration_dir}/${migration_name_down}
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
					full, err := Describe(MigrationDir, "full")
					if err != nil {
						return fmt.Errorf("error Describe: %w", err)
					}
					_, lastMigration, err := FindLastMigrationInfo(MigrationDir, full)
					if err != nil {
						return fmt.Errorf("error FindLastMigrationInfo: %w", err)
					}

					pattern := regexp.MustCompile(fmt.Sprintf(`^(%s-\d+)\.up\(.sql)$`, regexp.QuoteMeta(lastMigration)))
					matches := pattern.FindStringSubmatch(lastMigration)
					if matches == nil {
						return fmt.Errorf("wrong file name '%s' expect  name-x[.y[.z][-r].up.ext", lastMigration)
					} else if len(matches) == 2 {
						prefix := matches[1]
						ext := matches[2]

						upFileName := lastMigration
						downFileName := fmt.Sprintf("%s-.down.-%s", prefix, ext)
						upFileDir := filepath.Join(MigrationDir, upFileName)
						downFileDir := filepath.Join(MigrationDir, downFileName)

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

						// compare includes and fill missed_includes or deleted includes
						for include, included := range originalIncludes {
							Ld(fmt.Sprintf("include file %s", include))

							md5, err := FileMD5(include)
							if err != nil {
								return fmt.Errorf("error FileMD5: %w", err)
							}
							// parts := strings.Split(include, "/")
							// includedRelativeFile := strings.Join(parts[1:], "/")
							Ld(fmt.Sprintf("md5 %x of include %s included by %s and check in migrationIncludes", md5, include, included))
							if _, exists := rslts.ProjectMD5Includes[md5]; !exists {
								if _, exists := rslts.MissedIncludes[include]; !exists {
									rslts.MissedIncludes[include] = included
									rslts.MissedIncludesCnt++
								}
							}
						}
						fmt.Printf("pair %s.up|down.%s save migration: %s.up|down.%s\n", meta.Prefix, meta.Ext, moduleMeta.Prefix, moduleMeta.Ext)
					}
					md5Up, err := FileMD5(upFileDir)
					if err != nil {
						return fmt.Errorf("error FileMD5: %w", err)
					}
					md5Down, err := FileMD5(downFileDir)
					if err != nil {
						return fmt.Errorf("error FileMD5: %w", err)
					}
					var md5UpDown [32]byte
					copy(md5UpDown[0:16], md5Up[:])
					copy(md5UpDown[16:32], md5Down[:])

					moduleUpFileDir := filepath.Join(moduleMeta.Dir, moduleMeta.UpFileName)
					moduleDownFileDir := filepath.Join(moduleMeta.Dir, moduleMeta.DownFileName)

					os.WriteFile(upFileDir, []byte(fmt.Sprintf("#migration: %s;%x", meta.UpFileName, md5UpDown)), 0644)
					data, err := os.ReadFile(upFileDir)
					if err != nil {
						return fmt.Errorf("error reading file: %w", err)
					}
					file, err := os.OpenFile(moduleUpFileDir, os.O_APPEND|os.O_WRONLY, 0644)
					if err != nil {
						return fmt.Errorf("error opening file: %w", err)
					}
					defer file.Close()
					file.Write(data)

					os.WriteFile(downFileDir, []byte(fmt.Sprintf("#migration: %s;%x", meta.DownFileName, md5UpDown)), 0644)
					data, err = os.ReadFile(downFileDir)
					if err != nil {
						return fmt.Errorf("error reading file: %w", err)
					}
					file, err = os.OpenFile(moduleDownFileDir, os.O_APPEND|os.O_WRONLY, 0644)
					if err != nil {
						return fmt.Errorf("error opening file: %w", err)
					}
					defer file.Close()
					file.Write(data)

					collectedCnt++
					collectedCnt++
				}
			}
		}
	}
	return nil
}
