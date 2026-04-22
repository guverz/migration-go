package migration

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	IncludePattern = regexp.MustCompile(`^@([^;]+)`)
	ListPattern    = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.up\.([^\.]+)$`)
)

var (
	visiting = 1
	done     = 2
)

type ParseContext struct {
	State    map[string]int
	Includes map[string]string

	MissingFiles map[string]string // key - include; value - included
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
	ListWarnings []string // list of non-critical errors

	LostPairs          map[string]string // key - missed migration file, value - existing pair
	MissedPairs        map[string]string // key - missed migration file, value - existing pair
	DeletedFilesCnt    int               // counters seems to be obsolete (?)
	DeletedIncludesCnt int               //
	MissedIncludesCnt  int               //
	MissedFilesCnt     int               //
	DeletedFiles       map[string]string // key - project migration, value - module migration
	DeletedIncludes    map[string]string // key - include, value - included (include is being included; included includes)
	MissedIncludes     map[string]string // key - include, value - included
	MissedFiles        map[string]Meta   // key - upFileName, value - Meta

	ProjectMigrations  map[string]Meta   // key - MD5, value - Meta of any original migration pair
	ModuleMigrations   map[string]Meta   // key - prefix of original migration file, value - Meta of migration copy
	ProjectIncludes    map[string]string // key - include, value - included; include of original project migration file
	ModuleIncludes     map[string]string // key - include, value - included; include of module migration file
	ProjectMD5Includes map[string]string // key - MD5, value - include; md5 of includes used in project migration
}

func MigrationList(dir string, rslts *ListResults) error {
	rslts.MissedPairs = make(map[string]string)
	rslts.LostPairs = make(map[string]string)

	rslts.MissedIncludes = make(map[string]string)
	rslts.DeletedIncludes = make(map[string]string)
	rslts.ModuleIncludes = make(map[string]string)
	rslts.ProjectIncludes = make(map[string]string)
	rslts.DeletedFiles = make(map[string]string)
	rslts.ProjectMD5Includes = make(map[string]string)

	rslts.ProjectMigrations = make(map[string]Meta)

	rslts.ModuleMigrations = make(map[string]Meta)

	// ConcLimit is 0 when config was not loaded (e.g. unit tests): an unbuffered chan would
	// block forever on the first "sem <- struct{}{}" before any goroutine can receive.
	concLimit := ConcLimit
	if concLimit < 1 {
		concLimit = 4
	}

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
	sem := make(chan struct{}, concLimit)

	setErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
		})
	}

	for _, entry := range entries {
		entry := entry
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
			upFileName := entry.Name()
			downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)

			fileDirUp := filepath.Join(dir, entry.Name())
			fileDirDown := filepath.Join(dir, downFileName)

			if _, exists := fileMap[downFileName]; !exists {
				mu.Lock()
				rslts.LostPairs[downFileName] = entry.Name()
				mu.Unlock()
			}
			if err := ParseIncludes(projectContext, fileDirUp, ""); err != nil {
				setErr(fmt.Errorf("ParseIncludes error: %w", err))
				return
			}
			if err := ParseIncludes(projectContext, fileDirDown, ""); err != nil {
				setErr(fmt.Errorf("ParseIncludes error: %w", err))
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
					var metaMD5 string
					if len(parts) == 2 {
						metaMD5 = parts[1]
					}
					fileName := filepath.Base(pathFileName)
					path := filepath.Dir(pathFileName)
					// check for meta in the migration file
					matches := ListPattern.FindStringSubmatch(fileName)
					if matches == nil {
						mu.Lock()
						rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext", file.Name()))
						mu.Unlock()
						continue
						// setErr(fmt.Errorf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext", file.Name()))
						// return
					}

					metaExt := matches[2]
					metaPrefix := matches[1]

					metaDir := filepath.Join(filepath.Dir(dir), path)

					metaUpName := metaPrefix + ".up." + metaExt
					metaDownName := metaPrefix + ".down." + metaExt
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
							// Lw(fmt.Sprintf("migration %s does not have based migration file %s", file.Name(), metaUpName))
							mu.Lock()
							rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("migration %s does not have original pair of migration files %s.up|down.%s", file.Name(), metaPrefix, metaExt))
							// pair of migrations is missing
							rslts.DeletedFilesCnt += 2
							rslts.DeletedFiles[fileDirUp] = metaFileDirUp
							rslts.DeletedFiles[fileDirDown] = metaFileDirDown
							mu.Unlock()
							// continue
						} else {
							mu.Lock()
							rslts.LostPairs[metaDownName] = metaUpName
							// rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("BUG: file %s do not have counterpart file %s at '%s'", metaUpName, metaDownName, metaDir))
							mu.Unlock()
							// setErr(fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaUpName, metaDownName, metaDir))
							// return
						}
					} else {
						if rslt, err = FindFileViaDir(metaFileDirUp); err != nil {
							setErr(fmt.Errorf("findFileViaDir error: %w", err))
							return
						} else if !rslt {
							mu.Lock()
							rslts.LostPairs[metaUpName] = metaDownName
							// rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("BUG: file %s do not have counterpart file %s at '%s'", metaDownName, metaUpName, metaDir))
							mu.Unlock()
							// setErr(fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaDownName, metaUpName, metaDir))
							// return
						}
					}

					Ld(fmt.Sprintf("MD5: %s, Prefix: %s, Ext: %s, Dir: %s, UpFileName: %s, DownFileName: %s",
						metaMD5,
						metaPrefix,
						metaExt,
						metaDir,
						metaUpName,
						metaDownName),
					)

					mu.Lock()
					rslts.ProjectMigrations[metaMD5] = Meta{
						Prefix:       metaPrefix,
						Ext:          metaExt,
						Dir:          metaDir,
						UpFileName:   metaUpName,
						DownFileName: metaDownName,
					}

					rslts.ModuleMigrations[metaPrefix] = Meta{
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
					for include, included := range moduleContext.MissingFiles {
						mu.Lock()
						rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("include file %s is missing in the module and it's being included by %s, need to fix it by hand", include, included))
						mu.Unlock()
						// Le(fmt.Sprintf("include file %s is missing in the module and it's being included by %s, need to fix it by hand", include, included))

						// this is an error caused by developer, he should fix it by himself (or we can try to delete the @include line in the migration file of the module)
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
								rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("missed include: %s (included by %s)", include, included))
								mu.Unlock()
								// Lw(fmt.Sprintf("missed include: %s (included by %s)", include, included))
							} else {
								rslts.MissedIncludes[metaInclude] = "unknown"
								rslts.MissedIncludesCnt++

								rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("include %s is missing, should be recreated using %s", include, metaInclude))
								rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("can't find the file that included %s yet", metaInclude))
								mu.Unlock()
								// Lw(fmt.Sprintf("include %s is missing, should be recreated using %s", include, metaInclude))
								// Lw(fmt.Sprintf("can't find the file that included %s yet", metaInclude))
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
							rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("include %s may be deleted as %s is missing", include, metaInclude))
							// Lw(fmt.Sprintf("include %s may be deleted as %s is deleted", include, metaInclude))
							rslts.DeletedIncludes[include] = included
							rslts.DeletedIncludesCnt++
							mu.Unlock()
						}

					}
					// compare includes and fill missed_includes or deleted includes
					for include, included := range moduleContext.Includes {
						md5Include, err := FileMD5(include)
						if err != nil {
							setErr(fmt.Errorf("FileMD5 error: %w", err))
							return
						}
						Ld(fmt.Sprintf("md5 %s of include file %s included by %s and check in migrationIncludes", md5Include, include, included))
						if _, exists := migrationMD5Includes[md5Include]; !exists {
							mu.Lock()
							if _, exists := rslts.MissedIncludes[include]; !exists {
								rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("copy of include file %s is changed", include))
								// Lw(fmt.Sprintf("copy of include file %s is changed", include))
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
			// lostPairFlag shows if project migration pair is complete (true - incomplete, false - complete)
			mu.Lock()
			existingPair, lostPairFlag := rslts.LostPairs[downFileName]
			mu.Unlock()
			// if meta is undefined, migration file is an original file
			if !foundMetaFlag && !lostPairFlag {
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
				rslts.ProjectMigrations[ogFileMD5UpDown] = Meta{
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
				// if missing migration pair is based on module migration pair then it can be fixed via collect command
			} else if foundMetaFlag && lostPairFlag {
				mu.Lock()
				rslts.MissedPairs[downFileName] = existingPair
				delete(rslts.LostPairs, downFileName)
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
	rslts.MissedFiles = make(map[string]Meta)

	// getting submodule dir
	moduleDirSlice, err := GetModuleDir(dir)
	if err != nil {
		return fmt.Errorf("getModuleDir failed: %w", err)
	}
	MigrationDirName := filepath.Base(dir)
	// for each git submodule
	for _, moduleDir := range moduleDirSlice {
		moduleProject := ""
		moduleMigration := filepath.Join(moduleDir, MigrationDirName)
		entries, err := os.ReadDir(moduleMigration)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading directory error: %w", err)
		}
		if filepath.Base(moduleDir) == "ddl" {
			baseFull, err := Describe(dir, "project")
			if err != nil {
				return fmt.Errorf("error describing dir: %w", err)
			}
			baseCut, _, _ := strings.Cut(baseFull, "-")
			moduleProject = fmt.Sprintf("%s-ddl", baseCut)
		} else {
			moduleProject, err = Describe(moduleDir, "project")
			if err != nil {
				return fmt.Errorf("describing module project failed: %w", err)
			}
		}
		Ld(fmt.Sprintf("module project %s", moduleProject))

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
		semSub := make(chan struct{}, concLimit)

		setErrSub := func(err error) {
			if err == nil {
				return
			}
			errOnceSub.Do(func() {
				firstErrSub = err
			})
		}

		for _, entry := range entries {
			entry := entry
			wgSub.Add(1)
			semSub <- struct{}{}

			go func() {
				defer wgSub.Done()
				defer func() { <-semSub }()

				Ld(fmt.Sprintf("file name %s dir %s", entry.Name(), moduleMigration))
				matches := ListPattern.FindStringSubmatch(entry.Name())
				if len(matches) != 3 {
					return
				}

				modulePrefix, moduleExt := matches[1], matches[2]

				if !strings.HasPrefix(entry.Name(), moduleProject) {
					Ld(fmt.Sprintf("file not started with project name: %s", moduleProject))
					modulePrefix = strings.TrimSuffix(entry.Name(), ".up.sql")
				}

				upFileName := fmt.Sprintf("%s.up.%s", modulePrefix, moduleExt)
				downFileName := fmt.Sprintf("%s.down.%s", modulePrefix, moduleExt)

				fileDirUp := filepath.Join(moduleMigration, upFileName)
				fileDirDown := filepath.Join(moduleMigration, downFileName)

				if _, exists := fileMap[downFileName]; !exists {
					mu.Lock()
					rslts.LostPairs[downFileName] = entry.Name()
					// rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir))
					mu.Unlock()
					return
					// setErrSub(fmt.Errorf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir))
					// return
				}

				md5ModuleUp, err := FileMD5(fileDirUp)
				if err != nil {
					setErrSub(fmt.Errorf("FileMD5 error: %w", err))
					return
				}
				md5ModuleDown, err := FileMD5(fileDirDown)
				if err != nil {
					setErrSub(fmt.Errorf("FileMD5 error: %w", err))
					return
				}

				md5ModuleUpDown := md5ModuleUp + md5ModuleDown

				Ld(fmt.Sprintf("MD5: %s, Prefix: %s, Ext: %s, Dir: %s, UpFile: %s, DownFile: %s",
					md5ModuleUpDown,
					modulePrefix,
					moduleExt,
					moduleMigration,
					upFileName,
					downFileName),
				)

				if _, exists := rslts.ProjectMigrations[md5ModuleUpDown]; !exists {
					mu.Lock()
					rslts.MissedFiles[upFileName] = Meta{
						Prefix:       modulePrefix,
						Ext:          moduleExt,
						Dir:          moduleMigration,
						UpFileName:   upFileName,
						DownFileName: downFileName,
					}
					// missed pair of migrations
					rslts.MissedFilesCnt += 2
					mu.Unlock()

					// checking includes in the missed module pair
					missedModuleContext := NewParseContext()

					if err := ParseIncludes(missedModuleContext, fileDirUp, ""); err != nil {
						setErrSub(fmt.Errorf("error ParseIncludes: %w", err))
						return
					}
					if err := ParseIncludes(missedModuleContext, fileDirDown, ""); err != nil {
						setErrSub(fmt.Errorf("error ParseIncludes: %w", err))
						return
					}
					for include, included := range missedModuleContext.MissingFiles {
						mu.Lock()
						rslts.ListWarnings = append(rslts.ListWarnings, fmt.Sprintf("include file %s is missing in the module and it's being included by %s, need to fix it by hand", include, included))
						mu.Unlock()
					}
					for include, included := range missedModuleContext.Includes {
						Ld(fmt.Sprintf("include file %s", include))

						md5, err := FileMD5(include)
						if err != nil {
							setErrSub(fmt.Errorf("error FileMD5: %w", err))
							return
						}
						Ld(fmt.Sprintf("md5 %s of include %s included by %s and check in migrationIncludes", md5, include, included))
						mu.Lock()
						if _, exists := rslts.ProjectMD5Includes[md5]; !exists {
							if _, exists := rslts.MissedIncludes[include]; !exists {
								Ld(fmt.Sprintf("include file %s is changed or doesn't exist in migration includes", include))
								rslts.MissedIncludes[include] = included
								rslts.MissedIncludesCnt++
							}
						}
						mu.Unlock()
					}
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
			// current = "" if fileDir is not an include
			if current == "" {
				return nil
			}
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
