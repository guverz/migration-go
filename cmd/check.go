package cmd

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	IncludePattern    = regexp.MustCompile(`^@([^;]+)`)
	ListPattern       = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.up\.([^\.]+)$`)
	ValidationPattern = regexp.MustCompile(`^(.+)\.(up|down)\.sql$`)
)

var (
	visiting = 1
	done     = 2
)

type Meta struct {
	Prefix       string
	Ext          string
	Dir          string
	UpFileName   string
	DownFileName string
}

type ListResults struct {
	DeletedIncludesCnt int
	MissedIncludesCnt  int
	MissedFilesCnt     int
	MissedMigrations   map[string]Meta
	ModuleMigrations   map[[32]byte]Meta
	ProjectIncludes    map[string]string
	ModuleIncludes     map[string]string
	MissedIncludes     map[string]string
	DeletedIncludes    map[string]string
	ProjectMD5Includes map[[16]byte]string
}

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
	rslt := &ListResults{}

	if err := MigrationList(MigrationDir, rslt); err != nil {
		return fmt.Errorf("migrationList failed: %w", err)
	}

	if rslt.MissedFilesCnt != 0 {
		Le(fmt.Sprintf("there are unregistered migration files pairs (%d), collect them and commit:", rslt.MissedFilesCnt))
		for _, file := range rslt.MissedMigrations {
			fmt.Println(file.Prefix + ".up|down." + file.Ext)
		}
		fmt.Println("do: scripts/migration collect")
	}
	if err := MigrationValidation(MigrationDir); err != nil {
		return fmt.Errorf("error migration validation: %w", err)
	}
	return nil
}

func MigrationList(dir string, rslts *ListResults) error {
	rslts.MissedIncludesCnt = 0
	rslts.DeletedIncludesCnt = 0

	rslts.MissedIncludes = make(map[string]string)
	rslts.DeletedIncludes = make(map[string]string)
	rslts.ModuleIncludes = make(map[string]string)
	rslts.ProjectIncludes = make(map[string]string)

	projectMigrations := make(map[[32]byte]Meta) // seems to be used by sole function

	rslts.ModuleMigrations = make(map[[32]byte]Meta)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	fileMap := make(map[string]bool)
	for _, entry := range entries {
		fileMap[entry.Name()] = true
	}

	for _, entry := range entries {
		Ld(fmt.Sprintf("found %s", entry.Name()))
		// declaring maps for parseIncludes func
		metaState := make(map[string]int)
		state := make(map[string]int)
		migrationIncludes := make(map[string]string)
		originalIncludes := make(map[string]string)

		matches := ListPattern.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		filePrefix, fileExt := matches[1], matches[2]
		downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)
		upFileName := entry.Name()

		if _, exists := fileMap[downFileName]; !exists {
			// assumption #1: might be bold to assume but it seems that program should still work after having found no counterpart for some mere migration file
			// log.Printf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
			// continue
			return fmt.Errorf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
		}

		fileDirUp := filepath.Join(dir, entry.Name())
		fileDirDown := filepath.Join(dir, downFileName)

		if err := ParseIncludes(fileDirUp, "", state, migrationIncludes); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		if err := ParseIncludes(fileDirDown, "", state, migrationIncludes); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		file, err := os.Open(fileDirUp)
		if err != nil {
			return fmt.Errorf("error opening dir: %w", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		foundMetaFlag := false
		for scanner.Scan() {
			line := scanner.Text()
			// if meta is defined
			if meta, ok := strings.CutPrefix(line, "#migration:"); ok {
				foundMetaFlag = true
				meta = strings.TrimSpace(meta)

				parts := strings.SplitN(meta, ";", 2)
				if len(parts) == 0 {
					continue
				}
				pathFileName := parts[0]
				md5 := [32]byte{}
				if len(parts) == 2 {
					decoded, err := hex.DecodeString(parts[1])
					if err != nil {
						return fmt.Errorf("error decoding MD5 string in meta: %w", err)
					}
					copy(md5[:], decoded)
				}
				fileName := filepath.Base(pathFileName)
				path := filepath.Dir(pathFileName)
				// check for meta in the migration file
				matches := ListPattern.FindStringSubmatch(fileName)
				if matches == nil {
					// assumption #1
					// log.Printf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext", file.Name())
					// continue
					return fmt.Errorf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext", file.Name())
				}

				ext := matches[2]
				prefix := matches[1]

				metaDir := filepath.Join(filepath.Dir(MigrationDir), path)

				metaUpName := prefix + ".up." + ext
				metaDownName := prefix + ".down." + ext
				metaFileDirUp := filepath.Join(metaDir, metaUpName)
				metaFileDirDown := filepath.Join(metaDir, metaDownName)
				// log.Printf("UP %s, DOWN %s refered by %s", metaFileDirUp, metaFileDirDown, file.Name())

				if rslt, err := FindFileViaDir(metaFileDirDown); err != nil {
					return fmt.Errorf("findFileViaDir error: %w", err)
				} else if !rslt {
					if rslt, err = FindFileViaDir(metaFileDirUp); err != nil {
						return fmt.Errorf("findFileViaDir error: %w", err)
					} else if !rslt {
						Le(fmt.Sprintf("migration %s does not have based migration file %s", file.Name(), metaUpName))
						// WARNING dangerous operation, check fileDirUp is not zero and this is a regular file
						if fileInfo, err := os.Stat(fileDirUp); err == nil && fileInfo.Mode().IsRegular() {
							Le(fmt.Sprintf("delete %s", fileDirUp))
							os.Remove(fileDirUp)
						}

						// WARNING dangerous operation, check fileDirDown is not zero and this is a regular file
						if fileInfo, err := os.Stat(fileDirDown); err == nil && fileInfo.Mode().IsRegular() {
							Le(fmt.Sprintf("delete %s", fileDirDown))
							os.Remove(fileDirDown)
						}
						continue

					} else {
						return fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaUpName, metaDownName, metaDir)
					}
				} else {
					if rslt, err = FindFileViaDir(metaFileDirUp); err != nil {
						return fmt.Errorf("findFileViaDir error: %w", err)
					} else if !rslt {
						return fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaDownName, metaUpName, metaDir)
					}
				}
				Ld(fmt.Sprintf("MD5: %x, Prefix: %s, Ext: %s, Dir: %s, UpFileName: %s, DownFileName: %s",
					md5,
					prefix,
					ext,
					metaDir,
					metaUpName,
					metaDownName),
				)
				// key of projectMigrations probably should actually be prefix:md5 because otherwise there can be different md5's with the same prefix
				projectMigrations[md5] = Meta{
					Prefix:       prefix,
					Ext:          ext,
					Dir:          metaDir,
					UpFileName:   metaUpName,
					DownFileName: metaDownName,
				}

				rslts.ModuleMigrations[md5] = Meta{
					Prefix:       filePrefix,
					Ext:          fileExt,
					Dir:          dir,
					UpFileName:   upFileName,
					DownFileName: downFileName,
				}

				if err := ParseIncludes(metaFileDirDown, "", metaState, originalIncludes); err != nil {
					return fmt.Errorf("parseIncludes error: %w", err)
				}
				if err := ParseIncludes(metaFileDirUp, "", metaState, originalIncludes); err != nil {
					return fmt.Errorf("parseIncludes error: %w", err)
				}

				migrationMD5Includes := make(map[[16]byte]string)
				rslts.ProjectMD5Includes = make(map[[16]byte]string)

				// build md5 includes from migrations includes
				for include, included := range migrationIncludes {
					md5Include, err := FileMD5(include)
					if err != nil {
						return fmt.Errorf("FileMD5 error: %w", err)
					}
					migrationMD5Includes[md5Include] = include
					rslts.ProjectMD5Includes[md5Include] = include
					includeDir, err := filepath.Rel(filepath.Clean(dir), include)
					if err != nil {
						return fmt.Errorf("error getting relative path: %w", err)
					}

					Ld(fmt.Sprintf("md5 %x of include file %s included by %s and check in original includes at %s",
						md5Include,
						include,
						included,
						metaDir),
					)
					// check if file exists in directory of
					metaInclude := filepath.Join(metaDir, includeDir)
					if rslt, err := FindFileViaDir(metaInclude); err != nil {
						return fmt.Errorf("findFileViaDir error: %w", err)
					} else if !rslt {
						Le(fmt.Sprintf("include %s may be deleted from %s, check later", include, metaInclude))
						rslts.DeletedIncludes[include] = included
						rslts.DeletedIncludesCnt++
					}

				}
				// compare includes and fill missed_includes or deleted includes
				for include, included := range originalIncludes {
					md5Include, err := FileMD5(include)
					if err != nil {
						return fmt.Errorf("FileMD5 error: %w", err)
					}
					Ld(fmt.Sprintf("md5 %x of include file %s included by %s and check in migration_includes", md5Include, include, included))
					if _, exists := migrationMD5Includes[md5Include]; !exists {
						if _, exists := rslts.MissedIncludes[include]; !exists {
							Lw(fmt.Sprintf("include file %s is changed or not exists in migration includes", include))
							rslts.MissedIncludesCnt++
							rslts.MissedIncludes[include] = included
						}
					} else {
						rslts.ModuleIncludes[include] = included
					}
				}

			}
		}
		// if meta is undefined, migration file is an original file
		if !foundMetaFlag {
			ogFileMD5Up, err := FileMD5(fileDirUp)
			if err != nil {
				return fmt.Errorf("FileMD5 error: %w", err)
			}
			ogFileMD5Down, err := FileMD5(fileDirDown)
			if err != nil {
				return fmt.Errorf("FileMD5 error: %w", err)
			}

			var ogFileMD5UpDown [32]byte
			copy(ogFileMD5UpDown[0:16], ogFileMD5Up[:])
			copy(ogFileMD5UpDown[16:32], ogFileMD5Down[:])

			projectMigrations[ogFileMD5UpDown] = Meta{
				Prefix:       filePrefix,
				Ext:          fileExt,
				Dir:          dir,
				UpFileName:   upFileName,
				DownFileName: downFileName,
			}

			Ld(fmt.Sprintf("MD5: %x, Prefix: %s, Ext: %s, Dir: %s, UpFileName: %s, DownFileName: %s",
				ogFileMD5UpDown,
				filePrefix,
				fileExt,
				dir,
				upFileName,
				downFileName),
			)

			maps.Copy(rslts.ProjectIncludes, migrationIncludes)
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scanner error: %w", err)
		}
	}

	// list submodules migrations to find uncollected or changed files

	rslts.MissedFilesCnt = 0
	rslts.MissedMigrations = make(map[string]Meta)

	// getting submodule dir
	submoduleDirSlice, err := getSubmoduleDir(MigrationDir)
	if err != nil {
		return fmt.Errorf("getSubmoduleDir failed: %w", err)
	}
	MigrationDirName := filepath.Base(MigrationDir)
	// for each git submodule
	for _, submoduleDir := range submoduleDirSlice {
		submoduleProject := ""
		submoduleMigration := filepath.Join(submoduleDir, MigrationDirName)
		entries, err := os.ReadDir(submoduleMigration)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			} else {
				return fmt.Errorf("reading directory error: %w", err)
			}
		}
		submoduleProject, err = getProject(submoduleDir)
		if err != nil {
			return fmt.Errorf("getProject failed: %w", err)
		}
		Ld(fmt.Sprintf("submodule project %s", submoduleProject))

		fileMap := make(map[string]bool, len(entries))
		for _, entry := range entries {
			fileMap[entry.Name()] = true
		}
		// for each file in submodule migrations directory
		for _, entry := range entries {
			Ld(fmt.Sprintf("file name %s dir %s", entry.Name(), submoduleMigration))
			matches := ListPattern.FindStringSubmatch(entry.Name())
			if len(matches) != 3 {
				continue
			}

			filePrefix, fileExt := matches[1], matches[2]

			if !strings.HasPrefix(entry.Name(), submoduleProject) {
				Ld(fmt.Sprintf("file not started with project name: %s", submoduleProject))
				filePrefix = strings.TrimSuffix(entry.Name(), ".up.sql")
			}

			upFileName := fmt.Sprintf("%s.up.%s", filePrefix, fileExt)
			downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)

			fileDirUp := filepath.Join(submoduleMigration, upFileName)
			fileDirDown := filepath.Join(submoduleMigration, downFileName)

			if _, exists := fileMap[downFileName]; !exists {
				// assumption #1
				// log.Printf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
				// continue
				return fmt.Errorf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
			}

			md5SubmoduleUp, err := FileMD5(fileDirUp)
			if err != nil {
				return fmt.Errorf("FileMD5 error: %w", err)
			}
			md5SubmoduleDown, err := FileMD5(fileDirDown)
			if err != nil {
				return fmt.Errorf("FileMD5 error: %w", err)
			}
			var md5SubmoduleUpDown [32]byte
			copy(md5SubmoduleUpDown[0:16], md5SubmoduleUp[:])
			copy(md5SubmoduleUpDown[16:32], md5SubmoduleDown[:])

			Ld(fmt.Sprintf("MD5: %x, Prefix: %s, Ext: %s, Dir: %s, UpFile: %s, DownFile: %s",
				md5SubmoduleUpDown,
				filePrefix,
				fileExt,
				submoduleMigration,
				upFileName,
				downFileName),
			)

			if _, exists := projectMigrations[md5SubmoduleUpDown]; !exists {
				rslts.MissedMigrations[upFileName] = Meta{
					Prefix:       filePrefix,
					Ext:          fileExt,
					Dir:          submoduleMigration,
					UpFileName:   upFileName,
					DownFileName: downFileName,
				}
				rslts.MissedFilesCnt++
			}
		}
	}

	return nil
}

func FindFileViaDir(fileDir string) (bool, error) {
	path := filepath.Dir(fileDir)
	base := filepath.Base(fileDir)

	entries, err := os.ReadDir(path)
	if err != nil {
		return false, fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	for _, entry := range entries {
		if entry.Name() == base {
			return true, nil
		}
	}
	return false, nil
}

func ParseIncludes(fileDir string, current string, state map[string]int, parent map[string]string) error {

	Ld(fmt.Sprintf("parse file on includes %s", fileDir))

	if state[fileDir] == visiting {
		return fmt.Errorf("include loop detected %s included by %s already included by %s",
			fileDir,
			current,
			parent[fileDir],
		)
	}

	if state[fileDir] == done {
		return nil
	}

	state[fileDir] = visiting

	file, err := os.Open(fileDir)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", fileDir, err)
	}
	defer file.Close()

	dir := filepath.Dir(fileDir)
	scanner := bufio.NewScanner(file)

	Ld(fmt.Sprintf("parse file on includes %s", fileDir))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "@") {
			continue
		}

		m := IncludePattern.FindStringSubmatch(line)
		if m == nil {
			Le(fmt.Sprintf("wrong include line in %s: %s", fileDir, line))
			// le("wrong include")
			continue
		}
		includeName := m[1]
		includeDir := filepath.Join(dir, includeName)

		Ld(fmt.Sprintf("%s include %s dir %s", fileDir, includeName, dir))
		// ld("file include include dir")

		if _, exists := parent[includeDir]; !exists {
			parent[includeDir] = fileDir
		}

		if err := ParseIncludes(includeDir, fileDir, state, parent); err != nil {
			return fmt.Errorf("include %s -> %s: %w", fileDir, includeDir, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	state[fileDir] = done
	return nil
}

func FileMD5(path string) ([16]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return [16]byte{}, err
	}
	return md5.Sum(data), nil
}

func getSubmoduleDir(path string) ([]string, error) {
	rslt := []string{}
	outerDir := filepath.Dir(path)
	submoduleDir := filepath.Join(outerDir, ".gitmodules")
	f, err := os.Open(submoduleDir)
	if err != nil {
		return nil, fmt.Errorf("error opening .gitmodules: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "path ="); ok {
			path := strings.TrimSpace(after)
			rslt = append(rslt, filepath.Join(outerDir, path))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return rslt, nil
}

// still a poor function but kinda made it more versatile
func getProject(repoPath string) (string, error) {
	configPath := filepath.Join(repoPath, ".git", "config")

	f, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	inOrigin := false
	var url string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}

		if strings.HasPrefix(line, "[") {
			inOrigin = false
		}

		if inOrigin && strings.HasPrefix(line, "url") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				url = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if url == "" {
		submoduleName := filepath.Base(repoPath)
		baseFull, err := Describe(MigrationDir, "project")
		if err != nil {
			return "", fmt.Errorf("error describing dir: %w", err)
		}
		baseCut, _, _ := strings.Cut(baseFull, "-")
		baseSubmoduleName := fmt.Sprintf("%s-%s", baseCut, submoduleName)
		return baseSubmoduleName, nil
	}

	if i := strings.Index(url, ":"); i != -1 {
		url = url[i+1:]
	}

	url = strings.ReplaceAll(url, "/", "-")
	url = strings.TrimSuffix(url, ".git")

	return url, nil
}

func MigrationValidation(path string) error {
	wrongFilesCnt := 0
	migrations := make(map[string]string)
	migrationIncludes := make(map[string]string)
	state := make(map[string]int)

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
		if ValidationPattern.MatchString(fileName) {
			if err := ParseIncludes(file, "", state, migrationIncludes); err != nil {
				return fmt.Errorf("error parsing Includes of %s, Error: %w", fileName, err)
			}
		}
	}

	for fileName, fileDir := range migrations {
		relative := strings.TrimPrefix(fileDir, path+"/")
		matches := ValidationPattern.FindStringSubmatch(fileName)
		if matches == nil {
			if _, exists := migrationIncludes[relative]; !exists {
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
			Le(fmt.Sprintf("ERROR: %s counterpart %s not found\n", fileName, counterpart))
			wrongFilesCnt++
		}
		if wrongFilesCnt > 0 {
			return fmt.Errorf("there are %d wrong files", wrongFilesCnt)
		}
	}
	return nil
}
