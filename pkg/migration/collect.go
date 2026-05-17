package migration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Collect() error {
	collectedCnt := 0
	errorSl := []string{}
	fsys := os.DirFS(".")

	rslts, err := migrationList(fsys, MigrationDir)
	if err != nil {
		return fmt.Errorf("migrationList failed: %w", err)
	}

	if len(rslts.MissedFiles) != 0 {
		ld(fmt.Sprintf("there are unregistered migration files pairs (%d), collecting:\n", len(rslts.MissedFiles)))
		collected, err := missedFiles(rslts.MissedFiles, realVersionGetter{})
		if err != nil {
			errorSl = append(errorSl, fmt.Sprintf("%v", err))
		}
		collectedCnt += collected
	}
	if len(rslts.MissedIncludes) != 0 {
		ld(fmt.Sprintf("the number of missed includes is (%d)\n", len(rslts.MissedIncludes)))
		collected, err := missedIncludes(rslts.MissedIncludes)
		if err != nil {
			errorSl = append(errorSl, fmt.Sprintf("%v", err))
		}
		collectedCnt += collected
	}
	if len(rslts.MissedPairs) != 0 {
		ld(fmt.Sprintf("the number of deleted files is %d", len(rslts.DeletedFiles)))
		collected, err := missedPairs(rslts.MissedPairs, rslts.ModuleMigrations, rslts.ProjectMigrations)
		if err != nil {
			errorSl = append(errorSl, fmt.Sprintf("%v", err))
		}
		collectedCnt += collected
	}
	if len(rslts.DeletedIncludes) != 0 {
		ld(fmt.Sprintf("the number of deleted includes is %d", len(rslts.DeletedIncludes)))
		collected, err := deletedIncludes(rslts.DeletedIncludes, rslts.ProjectIncludes, rslts.ModuleIncludes)
		if err != nil {
			errorSl = append(errorSl, fmt.Sprintf("%v", err))
		}
		collectedCnt += collected
	}
	if len(rslts.DeletedFiles) != 0 {
		ld(fmt.Sprintf("the number of  deleted files is %d", len(rslts.DeletedFiles)))
		collected, err := deletedFiles(rslts.DeletedFiles)
		if err != nil {
			errorSl = append(errorSl, fmt.Sprintf("%v", err))
		}
		collectedCnt += collected
	}

	// if err := migration.MigrationValidation(migration.MigrationDir); err != nil {
	// return fmt.Errorf("error MigrationValidation: %w", err)
	// }

	switch {
	case collectedCnt != 0:
		fmt.Printf("%s: %s\n",
			colorize("[OK]", green),
			colorize(fmt.Sprintf("collected %d file(s)", collectedCnt), reset),
		)
	case len(errorSl) != 0:
		for _, err := range errorSl {
			le(err)
		}
		return fmt.Errorf("error collecting files")
	default:
		fmt.Printf("%s: %s\n",
			colorize("[OK]", green),
			colorize("nothing to collect", reset),
		)
	}

	return nil
}

func missedFiles(missedFiles map[string]migrationInfo, getter versionGetter) (int, error) {
	collectedCnt := 0

	for key, module := range missedFiles {
		// checkMissedFiles registers both up and down names as keys for the same pair; process once.
		if key != module.UpFileName {
			continue
		}
		fmt.Printf("%s.up|down.%s\n", module.Prefix, module.Ext)
		// check if missedFiles exists
		upPath := filepath.Join(module.Dir, module.UpFileName)
		downPath := filepath.Join(module.Dir, module.DownFileName)
		if rslt, err := findFileViaDir(upPath); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if !rslt {
			return collectedCnt, fmt.Errorf("BUG: there is no file %s, something is wrong", upPath)
		}
		if rslt, err := findFileViaDir(downPath); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if !rslt {
			return collectedCnt, fmt.Errorf("BUG: there is no file %s, something is wrong", downPath)
		}
		// new project migration pair is required
		fmt.Println("creating new migration file")

		migrationTemplate, err := addMigrationPair(false, getter)
		if err != nil {
			return 0, fmt.Errorf("error Add: %w", err)
		}
		newUpFileName := fmt.Sprintf("%s.up.sql", migrationTemplate)
		newDownFileName := fmt.Sprintf("%s.down.sql", migrationTemplate)
		newUpFileDir := filepath.Join(MigrationDir, newUpFileName)
		newDownFileDir := filepath.Join(MigrationDir, newDownFileName)

		targetUpPath := newUpFileDir
		targetDownPath := newDownFileDir

		if rslt, err := findFileViaDir(newUpFileDir); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if !rslt {
			return collectedCnt, fmt.Errorf("BUG: there is no file %s, something wrong", newUpFileDir)
		}
		if rslt, err := findFileViaDir(newDownFileDir); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if !rslt {
			return collectedCnt, fmt.Errorf("BUG: there is no file %s, something wrong", newDownFileDir)
		}
		fmt.Printf("pair %s.up|down.%s updating migration pair: %s.up|down.sql\n", module.Prefix, module.Ext, migrationTemplate)

		md5UpDown, err := concatMD5(upPath, downPath)
		if err != nil {
			return collectedCnt, fmt.Errorf("error getting concat of md5 files: %w", err)
		}

		relativeUpFile := stripDir(upPath)
		relativeDownFile := stripDir(downPath)

		ld(fmt.Sprintf("writing into file %s information %s\n", targetUpPath, fmt.Sprintf("#migration: %s;%s",
			relativeUpFile,
			md5UpDown),
		))

		if err := appendToFrom(
			targetUpPath,
			upPath,
			fmt.Sprintf("#migration: %s;%s\n", relativeUpFile, md5UpDown),
		); err != nil {
			return collectedCnt, fmt.Errorf("error writing into new migration file: %w", err)
		}
		ld(fmt.Sprintf("copying from %s to %s\n", upPath, targetUpPath))

		ld(fmt.Sprintf("writing into file %s information %s\n", newDownFileDir, fmt.Sprintf("#migration: %s;%s",
			relativeDownFile,
			md5UpDown),
		))

		if err := appendToFrom(
			targetDownPath,
			downPath,
			fmt.Sprintf("#migration: %s;%s\n", relativeDownFile, md5UpDown),
		); err != nil {
			return collectedCnt, fmt.Errorf("error writing into new migration file: %w", err)
		}

		ld(fmt.Sprintf("copying from %s to %s\n", downPath, targetDownPath))

		// updating a pair of migrations
		collectedCnt += 2
	}

	return collectedCnt, nil
}

func missedIncludes(missedIncludes map[string]string) (int, error) {
	collectedCnt := 0
	for include, included := range missedIncludes {
		ld(fmt.Sprintf("include file %s", include))
		if exists, err := findFileViaDir(include); err != nil {
			return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
		} else if exists {
			md5, err := fileMD5(include)
			if err != nil {
				return collectedCnt, fmt.Errorf("error FileMD5: %w", err)
			}
			ld(fmt.Sprintf("md5 %x of include file %s included by %s and check in rslts.ProjectIncludes", md5, include, included))

			// getting project include path & name based on original include in module
			rawInclude := include
			marker := "migrations" + string(filepath.Separator)
			if idx := strings.Index(rawInclude, marker); idx != -1 {
				rawInclude = rawInclude[idx+len(marker):]
			}
			newIncludeFile := filepath.Join(MigrationDir, rawInclude)
			newIncludeDir := filepath.Dir(newIncludeFile)

			// if include dir doesn't exist, we create it
			if rslt, err := findFileViaDir(newIncludeDir); err != nil {
				return collectedCnt, fmt.Errorf("error FindFileViaDir: %w", err)
			} else if !rslt {
				if err := os.Mkdir(newIncludeDir, 0755); err != nil {
					return collectedCnt, fmt.Errorf("error creating dir: %w", err)
				}
			}
			fmt.Printf("add include file %s\n", include)

			if exists, err := findFileViaDir(newIncludeFile); err != nil {
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
		} else {
			// if module include is missing, then we can't use it to fulfill missing project include
			le(fmt.Sprintf("module include %s is missing, can't collect missing project include", include))
			continue
		}
	}
	return collectedCnt, nil
}

func deletedIncludes(deletedIncludes, projectIncludes, moduleIncludes map[string]string) (int, error) {
	collectedCnt := 0
	for include, included := range deletedIncludes {
		fmt.Printf("include file %s included by %s\n", include, included)
		// check if file exists in ProjectIncludes, if included by something else, then do not delete
		_, exists1 := projectIncludes[include]
		_, exists2 := moduleIncludes[include]
		if !exists1 && !exists2 {
			// WARNING: dangerous operation, check before delete
			if fileInfo, err := os.Stat(include); err == nil && fileInfo.Mode().IsRegular() {
				lw(fmt.Sprintf("deleting include %s", include))
				if err := os.Remove(include); err != nil {
					return collectedCnt, fmt.Errorf("failed to delete: %w", err)
				}
				collectedCnt++
			}
		}
	}
	return collectedCnt, nil
}

func deletedFiles(deletedFiles map[string]string) (int, error) {
	collectedCnt := 0
	for project := range deletedFiles {
		// WARNING dangerous operation, check if migration file is a regular file
		if fileInfo, err := os.Stat(project); err == nil && fileInfo.Mode().IsRegular() {
			lw(fmt.Sprintf("deleting %s", project))
			if err := os.Remove(project); err != nil {
				return collectedCnt, fmt.Errorf("failed to remove file: %w", err)
			}
			collectedCnt++
		}
	}

	return collectedCnt, nil
}

func missedPairs(missedPairs map[string]string, moduleMigrations map[string]migrationInfo, projectMigrations map[string]migrationInfo) (int, error) {
	collectedCnt := 0
	for missedPath := range missedPairs {
		missed := filepath.Base(missedPath)
		matches := migrationPattern.FindStringSubmatch(missed)
		if matches == nil {
			le("wrong format of migration")
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
		targetModuleFile := stripDir(fullTargetModuleFile)

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

func migrationValidation(path string) error {
	wrongFilesCnt := 0
	migrations := make(map[string]string)
	projectContext := newParseContext()

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
		ld(fmt.Sprintf("found file at %s", path))
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
		ld(fmt.Sprintf("file %s check if name is correct %s", fileName, file))
		if migrationPattern.MatchString(fileName) {
			if err := parseIncludes(projectContext, file, ""); err != nil {
				return fmt.Errorf("error parsing includes of %s, Error: %w", fileName, err)
			}
			if len(projectContext.MissingFiles) != 0 {
				lw("deleted Includes:")
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
		matches := migrationPattern.FindStringSubmatch(fileName)
		if matches == nil {
			if _, exists := projectContext.Includes[relative]; !exists {
				// undefined includes crawl in here and activate Le; I think that's not how it's supposed to work
				le(fmt.Sprintf("%s wrong file name suffix expect .up.sql or .down.sql", fileName))
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
		ld(fmt.Sprintf("file %s check if counterpart %s exists", fileName, counterpart))
		if _, exists := migrations[counterpart]; !exists {
			le(fmt.Sprintf("%s counterpart %s not found", fileName, counterpart))
			wrongFilesCnt++
		}
	}
	if wrongFilesCnt != 0 {
		return fmt.Errorf("there are (%d) wrong files", wrongFilesCnt)
	}
	return nil
}

// appendToFrom function firstly appends header, then appends srcFilePath text to newFilePath
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

	if _, err := os.Stat(srcFilePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("src file does not exist: %s", srcFilePath)
		}
		return err
	}

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
