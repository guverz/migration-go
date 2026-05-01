package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func MissedFiles(missedFiles map[string]MigrationInfo, moduleMigrations map[string]MigrationInfo) (int, error) {
	collectedCnt := 0

	targetUpPath := ""
	targetDownPath := ""
	for _, module := range missedFiles {
		fmt.Printf("%s.up|down.%s\n", module.Prefix, module.Ext)
		upPath := filepath.Join(module.Dir, module.UpFileName)
		downPath := filepath.Join(module.Dir, module.DownFileName)
		if rslt, err := FindFileViaDir(upPath); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if !rslt {
			// Lw(fmt.Sprintf("BUG: there is no file %s, something is wrong", upFileDir))
			// continue
			return collectedCnt, fmt.Errorf("BUG: there is no file %s, something is wrong", upPath)
		}
		if rslt, err := FindFileViaDir(downPath); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if !rslt {
			// Lw(fmt.Sprintf("BUG: there is no file %s, something is wrong", downFileDir))
			// continue
			return collectedCnt, fmt.Errorf("BUG: there is no file %s, something is wrong", downPath)
		}
		// before check if prefix exists in moduleMigrations == check if missing files are being present in project as copies
		// ngl this thing is probably impossible as we get rslts.MissedFiles only if module migration pair is not present in project
		if project, exists := moduleMigrations[module.Prefix]; exists {
			// if only the update of the migration file is required
			migrationUpFileDir := filepath.Join(project.Dir, project.UpFileName)
			migrationDownFileDir := filepath.Join(project.Dir, project.DownFileName)

			if rslt, err := FindFileViaDir(migrationUpFileDir); err != nil {
				return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				// Lw(fmt.Sprintf("BUG: there is no file %s, something wrong", migrationUpFileDir))
				// continue
				return collectedCnt, fmt.Errorf("BUG: there is no file %s, something wrong", migrationUpFileDir)
			}
			if rslt, err := FindFileViaDir(migrationDownFileDir); err != nil {
				return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				// Lw(fmt.Sprintf("BUG: there is no file %s, something wrong", migrationDownFileDir))
				// continue
				return collectedCnt, fmt.Errorf("BUG: there is no file %s, something wrong", migrationDownFileDir)
			}

			if err := os.Truncate(migrationUpFileDir, 0); err != nil {
				return collectedCnt, fmt.Errorf("error zeroing file: %w", err)
			}

			if err := os.Truncate(migrationDownFileDir, 0); err != nil {
				return collectedCnt, fmt.Errorf("error zeroing file: %w", err)
			}

			targetUpPath = migrationUpFileDir
			targetDownPath = migrationDownFileDir

			Lw(fmt.Sprintf("pair %s.up|down.%s update migration %s.up|down.%s", module.Prefix, module.Ext, project.Prefix, project.Ext))
		} else {
			// if new migration pair is required
			fmt.Println("creating new migration file")

			migrationTemplate, err := Add(false)
			if err != nil {
				return 0, fmt.Errorf("error Add: %w", err)
			}
			newUpFileName := fmt.Sprintf("%s.up.sql", migrationTemplate)
			newDownFileName := fmt.Sprintf("%s.down.sql", migrationTemplate)
			newUpFileDir := filepath.Join(MigrationDir, newUpFileName)
			newDownFileDir := filepath.Join(MigrationDir, newDownFileName)

			targetUpPath = newUpFileDir
			targetDownPath = newDownFileDir

			if rslt, err := FindFileViaDir(newUpFileDir); err != nil {
				return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				return collectedCnt, fmt.Errorf("BUG: there is no file %s, something wrong", newUpFileDir)
				// Lw(fmt.Sprintf("BUG: there is no file %s, something wrong", newUpFileDir))
				// continue
			}
			if rslt, err := FindFileViaDir(newDownFileDir); err != nil {
				return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				return collectedCnt, fmt.Errorf("BUG: there is no file %s, something wrong", newDownFileDir)
				// Lw(fmt.Sprintf("BUG: there is no file %s, something wrong", newDownFileDir))
				// continue
			}
			fmt.Printf("pair %s.up|down.%s updating migration pair: %s.up|down.sql\n", module.Prefix, module.Ext, migrationTemplate)
		}

		md5Up, err := FileMD5(upPath)
		if err != nil {
			return collectedCnt, fmt.Errorf("error FileMD5: %w", err)
		}
		md5Down, err := FileMD5(downPath)
		if err != nil {
			return collectedCnt, fmt.Errorf("error FileMD5: %w", err)
		}

		md5UpDown := md5Up + md5Down

		relativeUpFile := StripDir(upPath)
		relativeDownFile := StripDir(downPath)

		// fmt.Printf("writing into file %s information %s\n", newUpFileDir, fmt.Sprintf("#migration: %s;%s",
		// 	relativeUpFile,
		// 	md5UpDown),
		// )

		if err := appendToFrom(
			targetUpPath,
			upPath,
			fmt.Sprintf("#migration: %s;%s\n", relativeUpFile, md5UpDown),
		); err != nil {
			return collectedCnt, fmt.Errorf("error writing into new migration file: %w", err)
		}
		// fmt.Printf("copying from %s to %s\n", upFileDir, newUpFileDir)

		// fmt.Printf("writing into file %s information %s\n", newDownFileDir, fmt.Sprintf("#migration: %s;%s",
		// 	relativeDownFile,
		// 	md5UpDown),
		// )

		if err := appendToFrom(
			targetDownPath,
			downPath,
			fmt.Sprintf("#migration: %s;%s\n", relativeDownFile, md5UpDown),
		); err != nil {
			return collectedCnt, fmt.Errorf("error writing into new migration file: %w", err)
		}

		// fmt.Printf("copying from %s to %s\n", downFileDir, downFileDir)

		// updating a pair of migrations
		collectedCnt += 2
	}

	return collectedCnt, nil
}

func MissedIncludes(missedIncludes map[string]string, projectIncludes map[string]string) (int, error) {
	collectedCnt := 0
	// for include, included := range rslts.MissedIncludes {
	// 	fmt.Printf("include %s included by %s", include, included)
	// }
	for include, included := range missedIncludes {
		Ld(fmt.Sprintf("include file %s", include))
		if exists, err := FindFileViaDir(include); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if exists {
			md5, err := FileMD5(include)
			if err != nil {
				return collectedCnt, fmt.Errorf("error FileMD5: %w", err)
			}
			Ld(fmt.Sprintf("md5 %x of include file %s included by %s and check in rslts.ProjectIncludes", md5, include, included))

			// getting project include path & name based on original include in module
			rawInclude := include
			marker := "migrations" + string(filepath.Separator)
			if idx := strings.Index(rawInclude, marker); idx != -1 {
				rawInclude = rawInclude[idx+len(marker):]
			}
			newIncludeFile := filepath.Join(MigrationDir, rawInclude)
			newIncludeDir := filepath.Dir(newIncludeFile)

			// if include dir doesn't exist, we create it
			if rslt, err := FindFileViaDir(newIncludeDir); err != nil {
				return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				if err := os.Mkdir(newIncludeDir, 0755); err != nil {
					return collectedCnt, fmt.Errorf("error creating dir: %w", err)
				}
			}
			// ngl this part seems to be inaccessible
			if _, exists := projectIncludes[newIncludeFile]; exists {
				projectMD5, err := FileMD5(newIncludeFile)
				if err != nil {
					return collectedCnt, fmt.Errorf("error FileMD5: %w", err)
				}
				if md5 != projectMD5 {
					Lw(fmt.Sprintf("%s there is in rslts.ProjectIncludes[%s], it was changed, replace it", newIncludeFile, newIncludeFile))
					if err := appendToFrom(newIncludeFile, include, ""); err != nil {
						return collectedCnt, fmt.Errorf("error updating include: %w", err)
					}
					collectedCnt++
				}
			} else {
				fmt.Printf("add include file %s\n", include)

				if exists, err := FindFileViaDir(newIncludeFile); err != nil {
					return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
				} else if !exists {
					if err := os.WriteFile(newIncludeFile, []byte{}, 0644); err != nil {
						return collectedCnt, fmt.Errorf("error creating file: %w", err)
					}
				} else if exists {
					if err := os.Truncate(newIncludeFile, 0); err != nil {
						return collectedCnt, fmt.Errorf("error zeroing file: %w", err)
					}
				}

				if err := appendToFrom(newIncludeFile, include, ""); err != nil {
					return collectedCnt, fmt.Errorf("error updating iclude: %w", err)
				}
				collectedCnt++
			}
		} else {
			// if module include is missing, then we can't use it to fulfill missing project include
			Le(fmt.Sprintf("module include %s is missing, can't collect missing project include", include))
			continue
		}
	}
	return collectedCnt, nil
}

func DeletedIncludes(deletedIncludes map[string]string, projectIncludes map[string]string, moduleIncludes map[string]string) (int, error) {
	collectedCnt := 0
	for include, included := range deletedIncludes {
		fmt.Printf("include file %s included by %s\n", include, included)
		// check if file exists in ProjectIncludes, if included by something else, then do not delete
		_, exists1 := projectIncludes[include]
		_, exists2 := moduleIncludes[include]
		if !exists1 && !exists2 {
			// WARNING: dangerous operation, check before delete
			if fileInfo, err := os.Stat(include); err == nil && fileInfo.Mode().IsRegular() {
				Lw(fmt.Sprintf("deleting include %s", include))
				if err := os.Remove(include); err != nil {
					return collectedCnt, fmt.Errorf("failed to delete: %w", err)
				}
				collectedCnt++
			}
		}
	}
	return collectedCnt, nil
}

func DeletedFiles(deletedFiles map[string]string) (int, error) {
	collectedCnt := 0
	for project := range deletedFiles {
		// WARNING dangerous operation, check if migration file is a regular file
		if fileInfo, err := os.Stat(project); err == nil && fileInfo.Mode().IsRegular() {
			Lw(fmt.Sprintf("deleting %s", project))
			if err := os.Remove(project); err != nil {
				return collectedCnt, fmt.Errorf("failed to remove file: %w", err)
			}
			collectedCnt++
		}
	}

	return collectedCnt, nil
}

func MissedPairs(missedPairs map[string]string, moduleMigrations map[string]MigrationInfo, projectMigrations map[string]MigrationInfo) (int, error) {
	collectedCnt := 0
	for missed := range missedPairs {
		matches := MigrationPattern.FindStringSubmatch(missed)
		if matches == nil {
			Le("wrong format of migration")
			continue
		}
		migrationType := matches[2]

		var (
			modulePrefix string
			moduleMD5    string
			moduleDir    string
		)

		for target, meta := range moduleMigrations {
			if meta.DownFileName == missed || meta.UpFileName == missed {
				modulePrefix = target
				break
			}
		}
		for md5, meta := range projectMigrations {
			if meta.Prefix == modulePrefix {
				moduleMD5 = md5
				moduleDir = meta.Dir
				break
			}
		}
		if modulePrefix == "" || moduleMD5 == "" {
			return collectedCnt, fmt.Errorf("couldn't find info necessary to collect missed pair %s", missed)
		}
		targetModuleFileName := fmt.Sprintf("%s.%s.sql", modulePrefix, migrationType)
		fullTargetModuleFile := filepath.Join(moduleDir, targetModuleFileName)
		missingProjectFile := filepath.Join(MigrationDir, missed)
		targetModuleFile := StripDir(fullTargetModuleFile)

		missedFileInfo := fmt.Sprintf("# %s", missed)                                   // info about migration file name
		missedFileMeta := fmt.Sprintf("#migration: %s;%s", targetModuleFile, moduleMD5) // info about original migration file pair & its md5

		if err := os.WriteFile(missingProjectFile, []byte{}, 0644); err != nil {
			return collectedCnt, fmt.Errorf("error creating file: %w", err)
		}
		// maybe migration files should be created using Add function (?)
		if err := appendToFrom(
			missingProjectFile,
			fullTargetModuleFile,
			fmt.Sprintf("%s\n%s\n", missedFileInfo, missedFileMeta),
		); err != nil {
			return collectedCnt, fmt.Errorf("error appending migration file: %w", err)
		}
		collectedCnt++
	}

	return collectedCnt, nil
}

func MigrationValidation(path string) error {
	wrongFilesCnt := 0
	migrations := make(map[string]string)
	projectContext := NewParseContext()

	var files []string

	err := filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking dir: %w", err)
		}
		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "README.md") || strings.HasSuffix(path, ".txt") {
			return nil
		}
		Ld(fmt.Sprintf("found file at %s", path))
		files = append(files, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking dir: %w", err)
	}

	for _, file := range files {
		fileName := filepath.Base(file)
		migrations[fileName] = file
		// ld "file ${file_name} check name is correct $file"
		Ld(fmt.Sprintf("file %s check if name is correct %s", fileName, file))
		if MigrationPattern.MatchString(fileName) {
			if err := ParseIncludes(projectContext, file, ""); err != nil {
				return fmt.Errorf("error parsing includes of %s, Error: %w", fileName, err)
			}
			if len(projectContext.MissingFiles) != 0 {
				Lw("deleted Includes:")
				for include := range projectContext.MissingFiles {
					fmt.Println(include)
					wrongFilesCnt++
					// rslts.DeletedIncludesCnt++
				}
			}
		}
	}

	for fileName, fileDir := range migrations {
		relative := strings.TrimPrefix(fileDir, path+"/")
		matches := MigrationPattern.FindStringSubmatch(fileName)
		if matches == nil {
			if _, exists := projectContext.Includes[relative]; !exists {
				// undefined includes crawl in here and activate Le; I think that's not how it's supposed to work
				Le(fmt.Sprintf("%s wrong file name suffix expect .up.sql or .down.sql", fileName))
				wrongFilesCnt++
				continue
			}
			continue
		}

		prefix := matches[1]
		suffix := matches[2]

		var counterpart string
		if suffix == "up" {
			counterpart = prefix + ".down.sql"
		} else {
			counterpart = prefix + ".up.sql"
		}
		Ld(fmt.Sprintf("file %s check if counterpart %s exists", fileName, counterpart))
		if _, exists := migrations[counterpart]; !exists {
			Le(fmt.Sprintf("%s counterpart %s not found", fileName, counterpart))
			wrongFilesCnt++
		}
	}
	if wrongFilesCnt != 0 {
		return fmt.Errorf("there are (%d) wrong files", wrongFilesCnt)
	}
	return nil
}

// firstly appends header, then appends srcFilePath text to newFilePath
func appendToFrom(newFilePath, srcFilePath, header string) error {
	newFile, err := os.OpenFile(newFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := newFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if _, err = newFile.WriteString(header); err != nil {
		return err
	}

	srcFile, err := os.Open(srcFilePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if _, err = io.Copy(newFile, srcFile); err != nil {
		return err
	}

	return nil
}
