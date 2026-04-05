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

type ParseContext struct {
	State    map[string]int
	Includes map[string]string

	MissingFiles map[string]string // key - include; value - included
	Errors       []error
}

func NewParseContext() *ParseContext {
	return &ParseContext{
		State:        make(map[string]int),
		Includes:     make(map[string]string),
		MissingFiles: make(map[string]string),
	}
}

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
	MissedFiles        map[string]Meta   // key - upFileName, value - Meta
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
	var listError error
	err := MigrationList(MigrationDir, rslt)
	if err != nil {
		listError = fmt.Errorf("migrationList failed: %w", err)
		// return fmt.Errorf("migrationList failed: %w", err)
	}

	if rslt.MissedFilesCnt != 0 {
		Lw(fmt.Sprintf("there are unregistered migration files (%d), collect them and commit:", rslt.MissedFilesCnt))
		collect = true
		for _, file := range rslt.MissedFiles {
			fmt.Println(file.Prefix + ".up|down." + file.Ext)
		}
	}
	if rslt.MissedIncludesCnt != 0 {
		Lw(fmt.Sprintf("there is number of unregistered include files (%d), collect them and commit:", rslt.MissedIncludesCnt))
		collect = true
		for include, included := range rslt.MissedIncludes {
			fmt.Printf("include %s included by %s\n", include, included)
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
	// if err := MigrationValidation(MigrationDir); err != nil {
	// 	return fmt.Errorf("error migration validation: %w", err)
	// }
	if listError != nil {
		return listError
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
			moduleContext := NewParseContext()
			projectContext := NewParseContext()

			migrationMD5Includes := make(map[string]string)

			matches := ListPattern.FindStringSubmatch(entry.Name())
			if len(matches) != 3 {
				return
			}

			filePrefix, fileExt := matches[1], matches[2]
			downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)
			upFileName := entry.Name()

			if _, exists := fileMap[downFileName]; !exists {
				// Lw(fmt.Sprintf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir))
				setErr(fmt.Errorf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir))
				return
			}

			fileDirUp := filepath.Join(dir, entry.Name())
			fileDirDown := filepath.Join(dir, downFileName)

			if err := ParseIncludes(projectContext, fileDirUp, ""); err != nil {
				setErr(fmt.Errorf("parseIncludes error: %w", err))
				return
			}
			if err := ParseIncludes(projectContext, fileDirDown, ""); err != nil {
				setErr(fmt.Errorf("parseIncludes error: %w", err))
				return
			}
			if len(projectContext.Errors) != 0 {
				for _, e := range projectContext.Errors {
					Lw(fmt.Sprintf("non-critical error: %s", e))
				}
			}
			// project include is literally missing
			// for include, included := range projectContext.MissingFiles {
			// 	mu.Lock()
			// 	if _, exists := rslts.MissedIncludes[include]; !exists {
			// 		rslts.MissedIncludes[include] = included
			// 		rslts.MissedIncludesCnt++
			// 		mu.Unlock()
			// 		Lw(fmt.Sprintf("missed include: %s (included by %s)", include, included))
			// 	} else {
			// 		mu.Unlock()
			// 	}
			// }

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

					metaDir := filepath.Join(filepath.Dir(dir), path)

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
							// pair of migrations is missing
							rslts.DeletedFilesCnt += 2
							rslts.DeletedFiles[fileDirUp] = metaFileDirUp
							rslts.DeletedFiles[fileDirDown] = metaFileDirDown
							mu.Unlock()
							// continue
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

					if err := ParseIncludes(moduleContext, metaFileDirDown, ""); err != nil {
						setErr(fmt.Errorf("parseIncludes error: %w", err))
						return
					}
					if err := ParseIncludes(moduleContext, metaFileDirUp, ""); err != nil {
						setErr(fmt.Errorf("parseIncludes error: %w", err))
						return
					}
					if len(moduleContext.Errors) != 0 {
						for _, e := range moduleContext.Errors {
							Lw(fmt.Sprintf("non-critical error: %s", e))
						}
					}
					for include, included := range moduleContext.MissingFiles {
						// if module file is gone but it is still being referenced by project file. it causes Le with empty included field, so it just skips
						if included == "" {
							continue
						} else {
							Le(fmt.Sprintf("include file %s is missing in the module and it's being included by %s, need to fix it by hand", include, included))
						}
						// this is an error caused by developer, he should fix it by himself (or we can try to delete the @include line in the migration file of the module)
						// mu.Lock()
						// if _, exists := rslts.DeletedIncludes[include]; !exists {
						// 	rslts.DeletedIncludes[include] = included
						// 	rslts.DeletedIncludesCnt++
						// 	mu.Unlock()
						// 	Lw(fmt.Sprintf("deleted include: %s (included by %s)", include, included))
						// } else {
						// 	mu.Unlock()
						// }
					}
					// project include is literally missing
					for include, included := range projectContext.MissingFiles {
						includeDir, err := filepath.Rel(filepath.Clean(dir), include)
						if err != nil {
							setErr(fmt.Errorf("error getting relative path: %w", err))
							return
						}
						metaInclude := filepath.Join(metaDir, includeDir)
						mu.Lock()
						if _, exists := rslts.MissedIncludes[metaInclude]; !exists {
							if metaIncluded, exists := rslts.ModuleIncludes[metaInclude]; exists {
								rslts.MissedIncludes[metaInclude] = metaIncluded
								rslts.MissedIncludesCnt++
								mu.Unlock()
								Lw(fmt.Sprintf("missed include: %s (included by %s)", include, included))
							} else {
								rslts.MissedIncludes[metaInclude] = "unknown"
								rslts.MissedIncludesCnt++
								mu.Unlock()
								Lw(fmt.Sprintf("include %s is missing, should be recreated using %s", include, metaInclude))
								Lw(fmt.Sprintf("can't find the file that included %s yet", metaInclude))
								// setErr(fmt.Errorf("can't find the file that included %s", metaInclude))
								// return
							}
						} else {
							mu.Unlock()
						}
					}

					// build md5 includes from migrations includes
					for include, included := range projectContext.Includes {
						// fmt.Println(include, included)
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

						// check if original include exists in module
						metaInclude := filepath.Join(metaDir, includeDir)
						if rslt, err := FindFileViaDir(metaInclude); err != nil {
							setErr(fmt.Errorf("findFileViaDir error: %w", err))
							return
						} else if !rslt {
							mu.Lock()
							Lw(fmt.Sprintf("include %s may be deleted as %s is deleted", include, metaInclude))
							rslts.DeletedIncludes[include] = included
							rslts.DeletedIncludesCnt++
							mu.Unlock()
						}

					}
					// compare includes and fill missed_includes or deleted includes
					for include, included := range moduleContext.Includes {
						// relative path could be calculated here but it was found to be obsolete
						md5Include, err := FileMD5(include)
						if err != nil {
							setErr(fmt.Errorf("FileMD5 error: %w", err))
							return
						}
						Ld(fmt.Sprintf("md5 %s of include file %s included by %s and check in migrationIncludes", md5Include, include, included))
						if _, exists := migrationMD5Includes[md5Include]; !exists {
							// migrationInclude, err := moduleToMigration(dir, include)
							// if err != nil {
							// 	setErr(fmt.Errorf("error transforming module to migration include path: %w", err))
							// 	return
							// }
							mu.Lock()
							if _, exists := rslts.MissedIncludes[include]; !exists {
								// if migrationIncluded, exists := rslts.ModuleIncludes[migrationInclude]; exists {
								Lw(fmt.Sprintf("copy of include file %s is changed", include))
								rslts.MissedIncludesCnt++
								rslts.MissedIncludes[include] = included
								// } else {
								// 	Le(fmt.Sprintf("can't find the file that included %s", migrationInclude))
								// }
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
				// for include, included := range projectContext.Parent {
				// 	fmt.Printf("include: %s, included by: %s\n", include, included)
				// }
				maps.Copy(rslts.ProjectIncludes, projectContext.Includes)
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
	rslts.MissedFiles = make(map[string]Meta)

	// getting submodule dir
	submoduleDirSlice, err := getSubmoduleDir(dir)
	if err != nil {
		return fmt.Errorf("getSubmoduleDir failed: %w", err)
	}
	MigrationDirName := filepath.Base(dir)
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

				// seems like obsolete check
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
					rslts.MissedFiles[upFileName] = Meta{
						Prefix:       filePrefix,
						Ext:          fileExt,
						Dir:          submoduleMigration,
						UpFileName:   upFileName,
						DownFileName: downFileName,
					}
					// missed pair of migrations
					rslts.MissedFilesCnt += 2
					mu.Unlock()
				}
			}()
		}

		wgSub.Wait()
		if firstErrSub != nil {
			return firstErrSub
		}
	}

	// for include, included := range rslts.ModuleIncludes {
	// 	fmt.Println(include, included)
	// }
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

func ParseIncludes(ctx *ParseContext, fileDir string, current string) error {

	Ld(fmt.Sprintf("parse file on includes %s", fileDir))

	if ctx.State[fileDir] == visiting {
		return fmt.Errorf("include loop detected %s included by %s already included by %s",
			fileDir,
			current,
			ctx.Includes[fileDir],
		)
	}

	if ctx.State[fileDir] == done {
		return nil
	}

	ctx.State[fileDir] = visiting

	file, err := os.Open(fileDir)
	if err != nil {
		if os.IsNotExist(err) {
			delete(ctx.Includes, fileDir)
			ctx.MissingFiles[fileDir] = current
			return nil
		}
		return fmt.Errorf("open %s: %w", fileDir, err)
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

		if _, exists := ctx.Includes[includeDir]; !exists {
			ctx.Includes[includeDir] = fileDir
		}

		if err := ParseIncludes(ctx, includeDir, fileDir); err != nil {
			return fmt.Errorf("include %s -> %s: %w", fileDir, includeDir, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	ctx.State[fileDir] = done
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
		if ValidationPattern.MatchString(fileName) {
			if err := ParseIncludes(projectContext, file, ""); err != nil {
				return fmt.Errorf("error parsing includes of %s, Error: %w", fileName, err)
			}
			if len(projectContext.Errors) != 0 {
				for _, e := range projectContext.Errors {
					Lw(fmt.Sprintf("non-critical error: %s", e))
				}
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
		matches := ValidationPattern.FindStringSubmatch(fileName)
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

// func moduleToMigration(migrationDir, moduleInclude string) (string, error) {
// 	var ModuleDir string
// 	idx := strings.Index(moduleInclude, "migrations")
// 	if idx != -1 {
// 		ModuleDir = moduleInclude[:idx+len("migrations")]
// 	}
// 	includePathRel, err := filepath.Rel(filepath.Clean(ModuleDir), moduleInclude)
// 	if err != nil {
// 		return "", err
// 	}
// 	return filepath.Join(migrationDir, includePathRel), nil
// }
