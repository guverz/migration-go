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
	"sync"

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
	DeletedFilesCnt    int               //
	DeletedIncludesCnt int               //
	MissedIncludesCnt  int               //
	MissedFilesCnt     int               //
	DeletedFiles       map[string]string // key - project migration, value - module migration
	DeletedIncludes    map[string]string // key - include, value - included (include is being included; included includes)
	MissedIncludes     map[string]string // key - include, value - included
	MissedMigrations   map[string]Meta   // key - upFileName, value - Meta
	ModuleMigrations   map[string]Meta   // key - prefix of original migration file, value - Meta
	ProjectIncludes    map[string]string // key - include, value - included
	ModuleIncludes     map[string]string // key - include, value - included
	ProjectMD5Includes map[string]string // key - MD5, value - include
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
	collect := false

	if err := MigrationList(MigrationDir, rslt); err != nil {
		return fmt.Errorf("migrationList failed: %w", err)
	}

	if rslt.MissedFilesCnt != 0 {
		Lw(fmt.Sprintf("there are unregistered migration files (%d), collect them and commit:", rslt.MissedFilesCnt))
		collect = true
		for _, file := range rslt.MissedMigrations {
			fmt.Println(file.Prefix + ".up|down." + file.Ext)
		}
	}
	if rslt.MissedIncludesCnt != 0 {
		Lw(fmt.Sprintf("there is number of unregistered include files (%d), collect them and commit:", rslt.MissedFilesCnt))
		collect = true
		for include, included := range rslt.MissedIncludes {
			fmt.Printf("include %s inclded by %s\n", include, included)
		}
	}
	if rslt.DeletedIncludesCnt != 0 {
		Lw(fmt.Sprintf("there is number of obsolete includes (%d), collect them and commit:", rslt.DeletedIncludesCnt))
		collect = true
		for include, included := range rslt.DeletedIncludes {
			fmt.Printf("include %s included by %s\n", include, included)
		}
	}
	if rslt.DeletedFilesCnt != 0 {
		Lw(fmt.Sprintf("there is number of obsolete migration files (%d), collect them and commit:", rslt.DeletedFilesCnt))
		collect = true
		for project, module := range rslt.DeletedFiles {
			fmt.Printf("migration file %s missing original file %s\n", project, module)
		}
	}

	if collect {
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
	rslts.DeletedFiles = make(map[string]string)
	rslts.ProjectMD5Includes = make(map[string]string)

	projectMigrations := make(map[string]Meta) // seems to be used by sole function

	rslts.ModuleMigrations = make(map[string]Meta)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	fileMap := make(map[string]bool)
	for _, entry := range entries {
		fileMap[entry.Name()] = true
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		errOnce  sync.Once
		firstErr error
	)
	sem := make(chan struct{}, ConcLimit)

	setErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
		})
	}

	for _, entry := range entries {
		wg.Add(1)
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			Ld(fmt.Sprintf("found %s", entry.Name()))
			// declaring maps for parseIncludes func
			metaState := make(map[string]int)
			state := make(map[string]int)
			migrationIncludes := make(map[string]string)
			originalIncludes := make(map[string]string)

			migrationMD5Includes := make(map[string]string)

			matches := ListPattern.FindStringSubmatch(entry.Name())
			if len(matches) != 3 {
				return
			}

			filePrefix, fileExt := matches[1], matches[2]
			downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)
			upFileName := entry.Name()

			if _, exists := fileMap[downFileName]; !exists {
				setErr(fmt.Errorf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir))
				return
			}

			fileDirUp := filepath.Join(dir, entry.Name())
			fileDirDown := filepath.Join(dir, downFileName)

			if err := ParseIncludes(fileDirUp, "", state, migrationIncludes); err != nil {
				setErr(fmt.Errorf("parseIncludes error: %w", err))
				return
			}
			if err := ParseIncludes(fileDirDown, "", state, migrationIncludes); err != nil {
				setErr(fmt.Errorf("parseIncludes error: %w", err))
				return
			}

			file, err := os.Open(fileDirUp)
			if err != nil {
				setErr(fmt.Errorf("error opening dir: %w", err))
				return
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
					var md5 string
					if len(parts) == 2 {
						md5 = parts[1]
					}
					fileName := filepath.Base(pathFileName)
					path := filepath.Dir(pathFileName)
					// check for meta in the migration file
					matches := ListPattern.FindStringSubmatch(fileName)
					if matches == nil {
						setErr(fmt.Errorf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext", file.Name()))
						return
					}

					ext := matches[2]
					prefix := matches[1]

					metaDir := filepath.Join(filepath.Dir(MigrationDir), path)

					metaUpName := prefix + ".up." + ext
					metaDownName := prefix + ".down." + ext
					metaFileDirUp := filepath.Join(metaDir, metaUpName)
					metaFileDirDown := filepath.Join(metaDir, metaDownName)

					if rslt, err := FindFileViaDir(metaFileDirDown); err != nil {
						setErr(fmt.Errorf("findFileViaDir error: %w", err))
						return
					} else if !rslt {
						if rslt, err = FindFileViaDir(metaFileDirUp); err != nil {
							setErr(fmt.Errorf("findFileViaDir error: %w", err))
							return
						} else if !rslt {
							Lw(fmt.Sprintf("migration %s does not have based migration file %s", file.Name(), metaUpName))
							mu.Lock()
							rslts.DeletedFilesCnt = +2
							rslts.DeletedFiles[fileDirUp] = metaFileDirUp
							rslts.DeletedFiles[fileDirDown] = metaFileDirDown
							mu.Unlock()
							continue
						} else {
							setErr(fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaUpName, metaDownName, metaDir))
							return
						}
					} else {
						if rslt, err = FindFileViaDir(metaFileDirUp); err != nil {
							setErr(fmt.Errorf("findFileViaDir error: %w", err))
							return
						} else if !rslt {
							setErr(fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaDownName, metaUpName, metaDir))
							return
						}
					}

					Ld(fmt.Sprintf("MD5: %s, Prefix: %s, Ext: %s, Dir: %s, UpFileName: %s, DownFileName: %s",
						md5,
						prefix,
						ext,
						metaDir,
						metaUpName,
						metaDownName),
					)
					mu.Lock()
					// key of projectMigrations probably should actually be prefix:md5 because otherwise there can be different md5's with the same prefix
					projectMigrations[md5] = Meta{
						Prefix:       prefix,
						Ext:          ext,
						Dir:          metaDir,
						UpFileName:   metaUpName,
						DownFileName: metaDownName,
					}

					rslts.ModuleMigrations[prefix] = Meta{
						Prefix:       filePrefix,
						Ext:          fileExt,
						Dir:          dir,
						UpFileName:   upFileName,
						DownFileName: downFileName,
					}
					mu.Unlock()

					if err := ParseIncludes(metaFileDirDown, "", metaState, originalIncludes); err != nil {
						setErr(fmt.Errorf("parseIncludes error: %w", err))
						return
					}
					if err := ParseIncludes(metaFileDirUp, "", metaState, originalIncludes); err != nil {
						setErr(fmt.Errorf("parseIncludes error: %w", err))
						return
					}

					// build md5 includes from migrations includes
					for include, included := range migrationIncludes {
						md5Include, err := FileMD5(include)
						if err != nil {
							setErr(fmt.Errorf("FileMD5 error: %w", err))
							return
						}

						migrationMD5Includes[md5Include] = include
						mu.Lock()
						rslts.ProjectMD5Includes[md5Include] = include
						mu.Unlock()

						Ld(fmt.Sprintf("md5 %s of include file %s included by %s and check in original includes at %s",
							md5Include,
							include,
							included,
							metaDir),
						)

						includeDir, err := filepath.Rel(filepath.Clean(dir), include)
						if err != nil {
							setErr(fmt.Errorf("error getting relative path: %w", err))
							return
						}
						// fmt.Println(dir, include, includeDir)

						// check if file exists in directory of
						metaInclude := filepath.Join(metaDir, includeDir)
						if rslt, err := FindFileViaDir(metaInclude); err != nil {
							setErr(fmt.Errorf("findFileViaDir error: %w", err))
							return
						} else if !rslt {
							// ERROR: include test\migrations\tables\migration.sqlite.0.2.3.sql may be deleted from test\roam-support\migrations\tables\migration.sqlite.0.2.3.sql, check later
							// not sure if error should look that way
							Lw(fmt.Sprintf("include %s may be deleted from %s, check later", include, metaInclude))
							mu.Lock()
							rslts.DeletedIncludes[include] = included
							rslts.DeletedIncludesCnt++
							mu.Unlock()
						}

					}
					// compare includes and fill missed_includes or deleted includes
					for include, included := range originalIncludes {
						// relative path could be calculated here but it was found to be obsolete
						md5Include, err := FileMD5(include)
						if err != nil {
							setErr(fmt.Errorf("FileMD5 error: %w", err))
							return
						}
						Ld(fmt.Sprintf("md5 %s of include file %s included by %s and check in migrationIncludes", md5Include, include, included))
						if _, exists := migrationMD5Includes[md5Include]; !exists {
							mu.Lock()
							if _, exists := rslts.MissedIncludes[include]; !exists {
								Lw(fmt.Sprintf("include file %s is changed or does not exist in migrationIncludes", include))
								rslts.MissedIncludesCnt++
								rslts.MissedIncludes[include] = included
							}
							mu.Unlock()
						} else {
							mu.Lock()
							rslts.ModuleIncludes[include] = included
							mu.Unlock()
						}
					}

				}
			}
			// if meta is undefined, migration file is an original file
			if !foundMetaFlag {
				ogFileMD5Up, err := FileMD5(fileDirUp)
				if err != nil {
					setErr(fmt.Errorf("FileMD5 error: %w", err))
					return
				}
				ogFileMD5Down, err := FileMD5(fileDirDown)
				if err != nil {
					setErr(fmt.Errorf("FileMD5 error: %w", err))
					return
				}

				ogFileMD5UpDown := ogFileMD5Up + ogFileMD5Down

				mu.Lock()
				projectMigrations[ogFileMD5UpDown] = Meta{
					Prefix:       filePrefix,
					Ext:          fileExt,
					Dir:          dir,
					UpFileName:   upFileName,
					DownFileName: downFileName,
				}
				// fmt.Printf("MD5: %s, Prefix: %s, Ext: %s, Dir: %s, UpFileName: %s, DownFileName: %s\n",
				//  ogFileMD5UpDown,
				//  filePrefix,
				//  fileExt,
				//  dir,
				//  upFileName,
				//  downFileName)

				Ld(fmt.Sprintf("MD5: %s, Prefix: %s, Ext: %s, Dir: %s, UpFileName: %s, DownFileName: %s",
					ogFileMD5UpDown,
					filePrefix,
					fileExt,
					dir,
					upFileName,
					downFileName),
				)
				for include, included := range migrationIncludes {
					fmt.Printf("include: %s, included by: %s", include, included)
				}
				maps.Copy(rslts.ProjectIncludes, migrationIncludes)
				mu.Unlock()
			}
			if err := scanner.Err(); err != nil {
				setErr(fmt.Errorf("scanner error: %w", err))
				return
			}
		}()
	}

	wg.Wait()
	if firstErr != nil {
		return firstErr
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
		var (
			wgSub       sync.WaitGroup
			errOnceSub  sync.Once
			firstErrSub error
		)
		semSub := make(chan struct{}, ConcLimit)

		setErrSub := func(err error) {
			if err == nil {
				return
			}
			errOnceSub.Do(func() {
				firstErrSub = err
			})
		}

		for _, entry := range entries {
			wgSub.Add(1)
			semSub <- struct{}{}

			go func() {
				defer wgSub.Done()
				defer func() { <-semSub }()

				Ld(fmt.Sprintf("file name %s dir %s", entry.Name(), submoduleMigration))
				matches := ListPattern.FindStringSubmatch(entry.Name())
				if len(matches) != 3 {
					return
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
					setErrSub(fmt.Errorf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir))
					return
				}

				md5SubmoduleUp, err := FileMD5(fileDirUp)
				if err != nil {
					setErrSub(fmt.Errorf("FileMD5 error: %w", err))
					return
				}
				md5SubmoduleDown, err := FileMD5(fileDirDown)
				if err != nil {
					setErrSub(fmt.Errorf("FileMD5 error: %w", err))
					return
				}

				md5SubmoduleUpDown := md5SubmoduleUp + md5SubmoduleDown

				Ld(fmt.Sprintf("MD5: %s, Prefix: %s, Ext: %s, Dir: %s, UpFile: %s, DownFile: %s",
					md5SubmoduleUpDown,
					filePrefix,
					fileExt,
					submoduleMigration,
					upFileName,
					downFileName),
				)

				if _, exists := projectMigrations[md5SubmoduleUpDown]; !exists {
					mu.Lock()
					rslts.MissedMigrations[upFileName] = Meta{
						Prefix:       filePrefix,
						Ext:          fileExt,
						Dir:          submoduleMigration,
						UpFileName:   upFileName,
						DownFileName: downFileName,
					}
					// missed pair of migrations
					rslts.MissedFilesCnt = +2
					mu.Unlock()
				}
			}()
		}

		wgSub.Wait()
		if firstErrSub != nil {
			return firstErrSub
		}
	}
	return nil
}

func FindFileViaDir(fileDir string) (bool, error) {
	_, err := os.Stat(fileDir)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
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

func FileMD5(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
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
