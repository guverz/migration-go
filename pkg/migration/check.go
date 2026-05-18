package migration

import (
	"bufio"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sync/errgroup"
)

var (
	includePattern     = regexp.MustCompile(`^@([^;]+)`)
	migrationUpPattern = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.up\.([^\.]+)$`)
	migrationPattern   = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.(up|down)\.([^\.]+)$`)
)

// listResults struct stores errors regarding to migration directory and saves important information that can be used to fix some of those errors.
type listResults struct {
	ListWarnings []string // list of non-critical errors

	LostPairs       map[string]string        // key - missed migration file, value - existing pair
	MissedPairs     map[string]string        // key - missed migration file, value - existing pair
	DeletedFiles    map[string]string        // key - project migration, value - module migration (module file is missing)
	DeletedIncludes map[string]string        // key - include, value - included (include is being included; included includes)
	MissedIncludes  map[string]string        // key - include, value - included
	MissedFiles     map[string]migrationInfo // key - upName of module file, value - migrationInfo of that module file

	ProjectMigrations map[string]migrationInfo // key - MD5, value - migrationInfo of any original migration pair (map of original migration files (have no meta))
	ModuleMigrations  map[string]migrationInfo // key - prefix of module migration file, value - migrationInfo of project migration file that references module
	ProjectIncludes   map[string]string        // key - include, value - included; include of project migration file (migration file has no meta)
	ModuleIncludes    map[string]string        // key - include, value - included; include of module migration file
}

// migrationInfo struct is
type migrationInfo struct {
	Prefix       string
	Ext          string
	Dir          string
	UpFileName   string
	DownFileName string
}

// Meta struct is used solely for project migration files.
type meta struct {
	MetaInfo migrationInfo
	MD5      string
}

func Check() error {
	collect := false
	fsys := os.DirFS(".")
	rslt, err := migrationList(fsys, MigrationDir)
	if err != nil {
		return fmt.Errorf("migrationList failed: %w", err)
	}
	for _, error := range rslt.ListWarnings {
		collect = true
		lw(error)
	}
	if len(rslt.LostPairs) != 0 {
		le(fmt.Sprintf("there is number of incomplete pairs (%d), need to fix it by hand:", len(rslt.LostPairs)))
		for missed, existing := range rslt.LostPairs {
			le(fmt.Sprintf("file %s does not have counterpart %s", existing, missed))
		}
	}
	if len(rslt.MissedFiles) != 0 {
		lw(fmt.Sprintf("there are unregistered migration files (%d), collect them and commit:", len(rslt.MissedFiles)))
		collect = true
		for file := range rslt.MissedFiles {
			fmt.Printf("\t%s\n", file)
		}
	}
	if len(rslt.MissedIncludes) != 0 {
		lw(fmt.Sprintf("there is number of unregistered include files (%d), collect them and commit:", len(rslt.MissedIncludes)))
		collect = true
		for include, included := range rslt.MissedIncludes {
			fmt.Printf("\tinclude %s included by %s\n", include, included)
		}
	}
	if len(rslt.MissedPairs) != 0 {
		lw(fmt.Sprintf("there is number of incomplete pairs (%d), collect them and commit:", len(rslt.MissedPairs)))
		collect = true
		for missed, existing := range rslt.MissedPairs {
			fmt.Printf("\tfile %s does not have counterpart %s\n", existing, missed)
		}
	}
	if len(rslt.DeletedIncludes) != 0 {
		lw(fmt.Sprintf("there is number of obsolete includes (%d), collect them and commit:", len(rslt.DeletedIncludes)))
		collect = true
		for include, included := range rslt.DeletedIncludes {
			fmt.Printf("\tinclude %s included by %s\n", include, included)
		}
	}
	if len(rslt.DeletedFiles) != 0 {
		lw(fmt.Sprintf("there is number of obsolete migration files (%d), collect them and commit:", len(rslt.DeletedFiles)))
		collect = true
		for project, module := range rslt.DeletedFiles {
			fmt.Printf("\tmigration file %s missing original file %s\n", project, module)
		}
	}

	switch {
	case collect:
		return fmt.Errorf("use collect command")
	case len(rslt.LostPairs) != 0:
		return fmt.Errorf("only lost pairs left, fix it by hand")
	default:
		fmt.Printf("%s: No errors!\n",
			colorize("[OK]", green),
		)
	}

	return nil
}

// migrationList checks migration and submodules directories for errors.
// Those errors are being added to listResults struct.
func migrationList(fsys fs.FS, projectMigrationsDir string) (*listResults, error) {
	rslts := &listResults{}
	rslts.MissedPairs = make(map[string]string)
	rslts.LostPairs = make(map[string]string)
	projectMigrationsDir = filepath.ToSlash(filepath.Clean(projectMigrationsDir))

	// getting project and module maps by reading migration directory
	var (
		projectEntriesMap map[string]struct{}
		moduleEntriesMap  map[string]struct{}
		err               error
	)
	g := new(errgroup.Group)
	g.Go(func() error {
		entries, localErr := getEntriesProjectMap(fsys, projectMigrationsDir)
		if localErr != nil {
			return fmt.Errorf("error getting map of project entries: %w", localErr)
		}
		projectEntriesMap = entries
		return nil
	})
	g.Go(func() error {
		entries, localErr := getEntriesModuleMap(fsys, projectMigrationsDir)
		if localErr != nil {
			return fmt.Errorf("error getting map of module entries: %w", localErr)
		}
		moduleEntriesMap = entries
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// getting map where key - project file, value - migration info of meta file (if md5 is an empty string then that project file is an original one)
	var (
		MetaMap   map[string]meta
		ModuleMap map[string]migrationInfo
	)
	g = new(errgroup.Group)
	g.Go(func() error {
		metaMap, localErr := getMetaMap(fsys, projectEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error getting map of projects: %w", localErr)
		}
		MetaMap = metaMap
		return nil
	})
	// getting map where key - concat md5, value - migration info of module file (only files with complete pairs)
	g.Go(func() error {
		moduleMap, localErr := getModuleMap(moduleEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error getting map of modules: %w", localErr)
		}
		ModuleMap = moduleMap
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	// FILLING IN MIGRATION FILE RELATED FIELDS OF LISTRESULTS STRUCT
	// ALSO CHECKING FOR DELETED MIGRATION FILES

	// ProjectMigrations, ModuleMigrations and DeletedFiles
	g = new(errgroup.Group)
	g.Go(func() error {
		projectMigrations, localErr := fillProjectMigrations(MetaMap)
		if localErr != nil {
			return fmt.Errorf("error filling ProjectMigrations: %w", localErr)
		}
		rslts.ProjectMigrations = projectMigrations
		return nil
	})
	g.Go(func() error {
		moduleMigrations, localErr := fillModuleMigrations(MetaMap)
		if localErr != nil {
			return fmt.Errorf("error filling ModuleMigrations: %w", localErr)
		}
		rslts.ModuleMigrations = moduleMigrations
		return nil
	})
	g.Go(func() error {
		deletedFiles, localErr := checkDeletedFiles(MetaMap, moduleEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error checking for deleted files: %w", localErr)
		}
		rslts.DeletedFiles = deletedFiles
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	// CHECKING PAIRS OF MIGRATION FILES

	var (
		missingProjectPairs map[string]string
		missingModulePairs  map[string]string
	)
	g = new(errgroup.Group)
	g.Go(func() error {
		pairs, localErr := checkPairs(projectEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error checking project pairs: %w", localErr)
		}
		missingProjectPairs = pairs
		return nil
	})
	g.Go(func() error {
		pairs, localErr := checkPairs(moduleEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error checking module pairs: %w", localErr)
		}
		missingModulePairs = pairs
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// missing pairs in modules' dirs can't be fixed, so it's being added to LostPairs immediately
	maps.Copy(rslts.LostPairs, missingModulePairs)

	// checking if missing pairs in project dir are original, if original - it can't be fixed, if based on module migration file - can be.
	lostPairs, missedPairs := processMissingProjectPairs(missingProjectPairs, MetaMap)
	maps.Copy(rslts.LostPairs, lostPairs)
	maps.Copy(rslts.MissedPairs, missedPairs)

	// CHECKING FOR MISSED MIGRATION FILES

	// missedFiles
	missedFiles := checkMissedFiles(rslts.ProjectMigrations, ModuleMap)

	rslts.MissedFiles = fillMissedFiles(missedFiles, rslts.ModuleMigrations, rslts.MissedPairs)
	// INCLUDES

	// filling in ParseContext for project, module and meta
	var (
		mapProjectIncludes map[string]parseContext
		mapModuleIncludes  map[string]parseContext
		mapMetaIncludes    map[string]parseContext
	)
	g = new(errgroup.Group)
	g.Go(func() error {
		projectIncludes, localErr := getMapParseContext(projectEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error getting project includes information: %w", localErr)
		}
		mapProjectIncludes = projectIncludes
		return nil
	})
	g.Go(func() error {
		moduleIncludes, localErr := getMapParseContext(moduleEntriesMap)
		if localErr != nil {
			return fmt.Errorf("error getting module includes information: %w", localErr)
		}
		mapModuleIncludes = moduleIncludes
		return nil
	})
	g.Go(func() error {
		metaIncludes, localErr := getMetaParseContext(MetaMap)
		if localErr != nil {
			return fmt.Errorf("error getting meta includes information: %w", localErr)
		}
		mapMetaIncludes = metaIncludes
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// ProjectMD5Includes map[string]string        // key - MD5, value - include; md5 of includes used in the project directory (used to be in listResults)
	var ProjectMD5Includes map[string]string

	// FILLING IN INCLUDES RELATED FIELDS OF LISTRESULTS STRUCT

	g = new(errgroup.Group)
	g.Go(func() error {
		projectMD5Includes, localErr := getProjectMD5Includes(mapProjectIncludes)
		if localErr != nil {
			return fmt.Errorf("error getting ProjectMD5Includes: %w", localErr)
		}
		ProjectMD5Includes = projectMD5Includes
		return nil
	})
	g.Go(func() error {
		rslts.ProjectIncludes = fillProjectIncludes(mapProjectIncludes, MetaMap)
		return nil
	})
	g.Go(func() error {
		deletedIncludes, localErr := checkDeletedIncludes(mapProjectIncludes, MetaMap)
		if localErr != nil {
			return fmt.Errorf("error checking migration directory for deleted includes: %w", localErr)
		}
		rslts.DeletedIncludes = deletedIncludes
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// ModuleIncludes
	rslts.ModuleIncludes, err = fillModuleIncludes(mapMetaIncludes, ProjectMD5Includes)
	if err != nil {
		return nil, fmt.Errorf("error getting ModuleIncludes: %w", err)
	}

	// MissedIncludes
	rslts.MissedIncludes, err = checkMissedIncludes(mapProjectIncludes, MetaMap, mapModuleIncludes)
	if err != nil {
		return nil, fmt.Errorf("error checking migration directory for missed includes: %w", err)
	}

	// checking missed files for includes
	warnings, newMissedIncludes, err := checkMissedFilesForIncludes(rslts.MissedFiles)
	if err != nil {
		return nil, fmt.Errorf("error checking missed files for unknown includes: %w", err)
	}
	// found that were not found parsing includes can't be automatically collected, so only warnings appear
	rslts.ListWarnings = append(rslts.ListWarnings, warnings...)
	// newly found includes for missed files are being added to missedIncludes
	missedIncludes, err := processMissedFilesIncludes(ProjectMD5Includes, rslts.MissedIncludes, newMissedIncludes)
	if err != nil {
		return nil, fmt.Errorf("error processing includes from MissedFiles: %w", err)
	}
	maps.Copy(rslts.MissedIncludes, missedIncludes)

	return rslts, nil
}

func fillMissedFiles(rawMissedFiles map[string]migrationInfo, moduleMigrations map[string]migrationInfo, missedPairs map[string]string) map[string]migrationInfo {
	missedFiles := make(map[string]migrationInfo)
	for modulePath, moduleInfo := range rawMissedFiles {
		projectInfo := moduleMigrations[moduleInfo.Prefix]
		upPath := filepath.Join(projectInfo.Dir, projectInfo.UpFileName)
		downPath := filepath.Join(projectInfo.Dir, projectInfo.DownFileName)
		_, exists1 := missedPairs[upPath]
		_, exists2 := missedPairs[downPath]
		if !exists1 && !exists2 {
			missedFiles[modulePath] = moduleInfo
		}
	}
	return missedFiles
}

func processMissingProjectPairs(missingProjectPairs map[string]string, metaMap map[string]meta) (map[string]string, map[string]string) {
	lostPairs := make(map[string]string)
	missedPairs := make(map[string]string)

	for missing, existing := range missingProjectPairs {
		meta := metaMap[existing]
		if meta.isOriginal() {
			lostPairs[missing] = existing
		} else {
			missedPairs[missing] = existing
		}
	}
	return lostPairs, missedPairs
}

func processMissedFilesIncludes(projectMD5Includes map[string]string, foundMissedIncludes map[string]string, missedFilesIncludesMap map[string]map[string]string) (map[string]string, error) {
	newMissedIncludes := make(map[string]string)
	for _, missedFileIncludes := range missedFilesIncludesMap {
		for include, included := range missedFileIncludes {
			md5, err := fileMD5(include)
			if err != nil {
				return nil, fmt.Errorf("error calculating MD5 of include: %w", err)
			}
			// check if include is already included in project directory
			_, existsProject := projectMD5Includes[md5]
			// check if include is already in MissedIncludes map
			_, existsMissed := foundMissedIncludes[include]
			if !existsProject && !existsMissed {
				newMissedIncludes[include] = included
			}
		}
	}
	return newMissedIncludes, nil
}

func checkMissedFilesForIncludes(missedFiles map[string]migrationInfo) ([]string, map[string]map[string]string, error) {
	warnings := []string{}
	newMissedIncludes := make(map[string]map[string]string)
	for file, moduleInfo := range missedFiles {
		isUp := strings.HasSuffix(file, ".up.sql")
		if !isUp {
			continue
		}
		missedContext := newParseContext()
		missedUpPath := filepath.Join(moduleInfo.Dir, moduleInfo.UpFileName)
		missedDownPath := filepath.Join(moduleInfo.Dir, moduleInfo.DownFileName)
		if err := parseIncludes(missedContext, missedUpPath, ""); err != nil {
			return nil, nil, fmt.Errorf("error parseIncludes: %w", err)
		}
		if err := parseIncludes(missedContext, missedDownPath, ""); err != nil {
			return nil, nil, fmt.Errorf("error parseIncludes: %w", err)
		}
		for include, included := range missedContext.MissingFiles {
			warnings = append(warnings, fmt.Sprintf("include file %s is missing in the module and it's being included by %s, need to fix it by hand", include, included))
		}
		newMissedIncludes[file] = missedContext.Includes
	}
	return warnings, newMissedIncludes, nil
}

func checkDeletedIncludes(mapProjectIncludes map[string]parseContext, metaMap map[string]meta) (map[string]string, error) {
	deletedIncludes := make(map[string]string)
	for upPath, projectContext := range mapProjectIncludes {
		meta := metaMap[upPath]
		dir := filepath.Dir(upPath)
		if meta.isOriginal() {
			continue
		}
		for projectInclude, projectIncluded := range projectContext.Includes {
			includeDir, err := filepath.Rel(dir, projectInclude)
			if err != nil {
				return nil, fmt.Errorf("error getting relative path: %w", err)
			}
			metaIncludePath := filepath.Join(meta.MetaInfo.Dir, includeDir)
			if rslt, err := findFileViaDir(metaIncludePath); err != nil {
				return nil, fmt.Errorf("findFileViaDir error: %w", err)
			} else if !rslt {
				deletedIncludes[projectInclude] = projectIncluded
			}
		}
	}
	return deletedIncludes, nil
}

func checkMissedIncludes(mapProjectIncludes map[string]parseContext, metaMap map[string]meta, mapModuleIncludes map[string]parseContext) (map[string]string, error) {
	missedIncludes := make(map[string]string)
	for upFile, projectContext := range mapProjectIncludes {
		meta := metaMap[upFile]
		if meta.isOriginal() {
			continue
		}
		migrationMD5Includes := make(map[string]string)
		for projectInclude := range projectContext.Includes {
			md5, err := fileMD5(projectInclude)
			if err != nil {
				return nil, fmt.Errorf("error getting md5 of an include file: %w", err)
			}
			migrationMD5Includes[md5] = projectInclude
		}
		moduleUpPath := filepath.Join(meta.MetaInfo.Dir, meta.MetaInfo.UpFileName)
		moduleContext := mapModuleIncludes[moduleUpPath]

		for metaInclude, metaIncluded := range moduleContext.Includes {
			metaMD5, err := fileMD5(metaInclude)
			if err != nil {
				return nil, fmt.Errorf("error getting md5 of an include file: %w", err)
			}
			if _, exists := migrationMD5Includes[metaMD5]; !exists {
				missedIncludes[metaInclude] = metaIncluded
			}
		}
	}
	return missedIncludes, nil
}

// shouldn't be only up files as if up file is missing, can't recreate pair
func fillModuleMigrations(metaMap map[string]meta) (map[string]migrationInfo, error) {
	moduleMigrations := make(map[string]migrationInfo)
	for file, meta := range metaMap {
		if meta.isOriginal() {
			continue
		}
		projectInfo, err := getProjectInfo(file)
		if err != nil {
			return nil, fmt.Errorf("error getting projectInfo: %w", err)
		}
		moduleMigrations[meta.MetaInfo.Prefix] = projectInfo
	}
	return moduleMigrations, nil
}

func getProjectInfo(projectPath string) (migrationInfo, error) {
	var (
		prefix   string
		ext      string
		dir      string
		upName   string
		downName string
	)
	dir = filepath.Dir(projectPath)
	matches := migrationPattern.FindStringSubmatch(filepath.Base(projectPath))
	if matches == nil {
		return migrationInfo{}, fmt.Errorf("wrong file name: %s", filepath.Base(projectPath))
	}
	prefix, ext = matches[1], matches[3]
	upName = prefix + ".up." + ext
	downName = prefix + ".down." + ext

	return migrationInfo{
		Prefix:       prefix,
		Ext:          ext,
		Dir:          dir,
		UpFileName:   upName,
		DownFileName: downName,
	}, nil
}

func fillProjectMigrations(metaMap map[string]meta) (map[string]migrationInfo, error) {
	projectMigrations := make(map[string]migrationInfo)
	for file, meta := range metaMap {
		fileName := filepath.Base(file)
		matches := migrationPattern.FindStringSubmatch(fileName)
		if matches == nil {
			continue
		}
		// if meta.isOriginal == true then that file has no meta, therefore migrationInfo needs to be calculated
		if meta.isOriginal() {
			prefix, ext := matches[1], matches[3]
			dir := filepath.Dir(file)
			upName := prefix + ".up." + ext
			downName := prefix + ".down." + ext
			upPath := filepath.Join(dir, upName)
			downPath := filepath.Join(dir, downName)

			md5, err := concatMD5(upPath, downPath)
			if err != nil {
				return nil, fmt.Errorf("error getting concat of MD5: %w", err)
			}
			// md5 is empty if migration pair is incomplete
			if md5 == "" {
				continue
			}
			projectMigrations[md5] = migrationInfo{
				Prefix:       prefix,
				Ext:          ext,
				Dir:          dir,
				UpFileName:   upName,
				DownFileName: downName,
			}
		} else {
			projectMigrations[meta.MD5] = meta.MetaInfo
		}
	}
	return projectMigrations, nil
}

func fillProjectIncludes(projectContextMap map[string]parseContext, metaMap map[string]meta) map[string]string {
	projectIncludes := make(map[string]string)
	for upFile, projectContext := range projectContextMap {
		meta := metaMap[upFile]
		if meta.isOriginal() {
			maps.Copy(projectIncludes, projectContext.Includes)
		}
	}
	return projectIncludes
}

// filling listResults field - ModuleIncludes
func fillModuleIncludes(metaContextMap map[string]parseContext, projectMD5Includes map[string]string) (map[string]string, error) {
	moduleIncludes := make(map[string]string)
	for _, MetaContext := range metaContextMap {
		for include, included := range MetaContext.Includes {
			md5, err := fileMD5(include)
			if err != nil {
				return nil, fmt.Errorf("error calculating md5 of include: %w", err)
			}
			if _, exists := projectMD5Includes[md5]; exists {
				moduleIncludes[include] = included
			}
		}
	}
	return moduleIncludes, nil
}

func getProjectMD5Includes(projectContextMap map[string]parseContext) (map[string]string, error) {
	projectMD5Includes := make(map[string]string)
	for _, ProjectContext := range projectContextMap {
		for include := range ProjectContext.Includes {
			md5, err := fileMD5(include)
			if err != nil {
				return nil, fmt.Errorf("error calculating md5 of include: %w", err)
			}
			projectMD5Includes[md5] = include
		}
	}
	return projectMD5Includes, nil
}

func getMapParseContext(entriesMap map[string]struct{}) (map[string]parseContext, error) {
	entryParseContext := make(map[string]parseContext)
	for upFile := range entriesMap {
		entryContext := newParseContext()
		isUp := strings.HasSuffix(upFile, ".up.sql")
		if !isUp {
			continue
		}
		downFile, err := switchMigrationType(upFile, "down")
		if err != nil {
			return nil, fmt.Errorf("error switching migration type to down: %w", err)
		}
		if err := parseIncludes(entryContext, upFile, ""); err != nil {
			return nil, fmt.Errorf("error parsing migration file (up) for includes: %w", err)
		}
		if err := parseIncludes(entryContext, downFile, ""); err != nil {
			return nil, fmt.Errorf("error parsing migration file (down) for includes: %w", err)
		}
		entryParseContext[upFile] = *entryContext
	}
	return entryParseContext, nil
}

func getMetaParseContext(metaMap map[string]meta) (map[string]parseContext, error) {
	metaParseContext := make(map[string]parseContext)
	for upFile, meta := range metaMap {
		metaContext := newParseContext()
		isUp := strings.HasSuffix(upFile, ".up.sql")
		if !isUp || meta.isOriginal() {
			continue
		}
		upPath := filepath.Join(meta.MetaInfo.Dir, meta.MetaInfo.UpFileName)
		downPath := filepath.Join(meta.MetaInfo.Dir, meta.MetaInfo.DownFileName)
		if err := parseIncludes(metaContext, upPath, ""); err != nil {
			return nil, fmt.Errorf("error parsing meta (up) for includes: %w, file: %s", err, upPath)
		}
		if err := parseIncludes(metaContext, downPath, ""); err != nil {
			return nil, fmt.Errorf("error parsing meta (down) for includes: %w, file: %s", err, downPath)
		}
		metaParseContext[upFile] = *metaContext
	}
	return metaParseContext, nil
}

// switchMigrationType return changed filename with newDirection.
func switchMigrationType(filename, newDirection string) (string, error) {
	parts := strings.Split(filename, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("filename is wrong format: %s", filename)
	}
	if newDirection == "" {
		return "", fmt.Errorf("empty direction string")
	}
	parts[len(parts)-2] = newDirection
	return strings.Join(parts, "."), nil
}

func checkPairs(projectMap map[string]struct{}) (map[string]string, error) {
	incompletePairs := make(map[string]string)
	for entryPath := range projectMap {
		isUp := strings.HasSuffix(entryPath, ".up.sql")
		isDown := strings.HasSuffix(entryPath, ".down.sql")
		switch {
		case isUp:
			entryName := filepath.Base(entryPath)
			entryDir := filepath.Dir(entryPath)
			downName, err := switchMigrationType(entryName, "down")
			if err != nil {
				return nil, fmt.Errorf("error switching migration type to down: %w", err)
			}
			downPath := filepath.Join(entryDir, downName)
			if _, exists := projectMap[downPath]; !exists {
				incompletePairs[downPath] = entryPath
			}
		case isDown:
			entryName := filepath.Base(entryPath)
			entryDir := filepath.Dir(entryPath)
			upName, err := switchMigrationType(entryName, "up")
			if err != nil {
				return nil, fmt.Errorf("error switching migration type to up: %w", err)
			}
			upPath := filepath.Join(entryDir, upName)
			if _, exists := projectMap[upPath]; !exists {
				incompletePairs[upPath] = entryPath
			}
		default:
			return nil, fmt.Errorf("error file type: %s", filepath.Base(entryPath))
		}
	}
	return incompletePairs, nil
}

func checkMissedFiles(projectMigrations map[string]migrationInfo, moduleMap map[string]migrationInfo) map[string]migrationInfo {
	missedFiles := make(map[string]migrationInfo)
	for md5, moduleInfo := range moduleMap {
		if _, exists := projectMigrations[md5]; !exists {
			missedFiles[moduleInfo.UpFileName] = moduleInfo
			missedFiles[moduleInfo.DownFileName] = moduleInfo
		}
	}
	return missedFiles
}

func checkDeletedFiles(metaMap map[string]meta, moduleEntriesMap map[string]struct{}) (map[string]string, error) {
	deletedFiles := make(map[string]string)
	for projectPath, meta := range metaMap {
		matches := migrationUpPattern.FindStringSubmatch(filepath.Base(projectPath))
		if matches == nil {
			continue
		}
		if meta.isOriginal() {
			continue
		}
		dir := filepath.Dir(projectPath)
		prefix, ext := matches[1], matches[2]
		upProjectPath := filepath.Join(dir, prefix+".up."+ext)
		downProjectPath := filepath.Join(dir, prefix+".down."+ext)

		upPath := filepath.Join(meta.MetaInfo.Dir, fmt.Sprintf("%s.up.%s", meta.MetaInfo.Prefix, meta.MetaInfo.Ext))
		downPath := filepath.Join(meta.MetaInfo.Dir, fmt.Sprintf("%s.down.%s", meta.MetaInfo.Prefix, meta.MetaInfo.Ext))

		_, upExists := moduleEntriesMap[upPath]
		_, downExists := moduleEntriesMap[downPath]
		if !upExists && !downExists {
			deletedFiles[upProjectPath] = upPath
			deletedFiles[downProjectPath] = downPath
		}
	}
	return deletedFiles, nil
}

// getModuleMap returns the map of module files, where key - concatenated md5 of module migration pair and value - migrationInfo struct.
func getModuleMap(moduleEntriesMap map[string]struct{}) (map[string]migrationInfo, error) {
	moduleMap := make(map[string]migrationInfo)
	for entry := range moduleEntriesMap {
		moduleInfo, md5, err := getModule(entry)
		if err != nil {
			return nil, fmt.Errorf("error getting module info: %w", err)
		}
		// empty md5 means that pair of migrations is incomplete
		if md5 == "" {
			continue
		}
		moduleMap[md5] = moduleInfo
	}
	return moduleMap, nil
}

// getModule reads file name and if it fits MigrationPattern then MD5 of migration pair is being calculated.
// The function returns migrationInfo struct and MD5 of that migration pair.
func getModule(entry string) (migrationInfo, string, error) {
	dir := filepath.Dir(entry)
	entryName := filepath.Base(entry)
	matches := migrationPattern.FindStringSubmatch(entryName)
	if matches == nil {
		return migrationInfo{}, "", fmt.Errorf("wrong name of module migration %s", entryName)
	}

	prefix, ext := matches[1], matches[3]

	upName := prefix + ".up." + ext
	downName := prefix + ".down." + ext
	upPath := filepath.Join(dir, upName)
	downPath := filepath.Join(dir, downName)

	md5, err := concatMD5(upPath, downPath)
	if err != nil {
		return migrationInfo{}, "", fmt.Errorf("error getting concat of MD5: %w", err)
	}
	return migrationInfo{
		Prefix:       prefix,
		Ext:          ext,
		Dir:          dir,
		UpFileName:   upName,
		DownFileName: downName,
	}, md5, nil
}

// isOriginal is being used to differentiate files that have meta or do not.
func (m meta) isOriginal() bool {
	return m.MD5 == ""
}

// getMetaMap reads projectEntryMap and sends every file's path to getMetaInfo to get migrationInfo.
// The result is a map where key - file name and value - meta information that's being stored in Meta struct.
// If migrationInfo field MD5 is empty string, then this file has no meta.
func getMetaMap(fsys fs.FS, projectEntriesMap map[string]struct{}) (map[string]meta, error) {
	metaMap := make(map[string]meta)
	for projectPath := range projectEntriesMap {
		// fsys works with pathes where separator is '/'
		projectPathTemp := filepath.ToSlash(projectPath)
		metaEntry, md5, err := getMetaInfo(fsys, projectPathTemp)
		if err != nil {
			return nil, fmt.Errorf("error getting project: %w", err)
		}
		metaMap[projectPath] = meta{
			MetaInfo: metaEntry,
			MD5:      md5,
		}
	}
	return metaMap, nil
}

// getMetaInfo opens file and reads it to find line that starts with "#migration:". Then this line is being used to return filled migrationInfo struct.
func getMetaInfo(fsys fs.FS, projectPath string) (migrationInfo, string, error) {
	var (
		prefix   string
		ext      string
		path     string
		metaMD5  string
		upName   string
		downName string
	)
	projectDir := filepath.Dir(projectPath)
	file, err := fsys.Open(projectPath)
	if err != nil {
		return migrationInfo{}, "", fmt.Errorf("error opening file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// if meta is defined
		if meta, ok := strings.CutPrefix(line, "#migration:"); ok {
			meta = strings.TrimSpace(meta)
			parts := strings.SplitN(meta, ";", 2)
			if len(parts) != 2 {
				return migrationInfo{}, "", fmt.Errorf("wrong meta field: %s", meta)
			}
			relPathFileName := parts[0]
			metaMD5 = parts[1]

			fileName := filepath.Base(relPathFileName)
			path = filepath.Join(filepath.Dir(projectDir), filepath.Dir(relPathFileName))
			// check for meta in the migration file
			matches := migrationPattern.FindStringSubmatch(fileName)
			if matches == nil {
				return migrationInfo{}, "", fmt.Errorf("wrong migration name: %s", fileName)
			}
			prefix = matches[1]
			ext = matches[3]
			upName = prefix + ".up." + ext
			downName = prefix + ".down." + ext
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return migrationInfo{}, "", fmt.Errorf("scanner error: %w", err)
	}
	return migrationInfo{
		Prefix:       prefix,
		Ext:          ext,
		Dir:          path,
		UpFileName:   upName,
		DownFileName: downName,
	}, metaMD5, nil
}

var (
	visiting = 1
	done     = 2
)

// parseContext struct is used by parseIncludes function.
// State field is used to check for Include Loops. Key - file, value - file's state (ranges from 1 - 2, 1 - visiting file, 2 - done visiting file).
// Includes field is used to save includes. Key - include file, value - the file that includes include file.
// MissingFiles field is used to save nonexistent files. Key - include file, value - the file that includes include file.
type parseContext struct {
	State    map[string]int
	Includes map[string]string

	MissingFiles map[string]string // key - include; value - included
}

// newParseContext function initializes maps of parseContext struct.
func newParseContext() *parseContext {
	return &parseContext{
		State:        make(map[string]int),
		Includes:     make(map[string]string),
		MissingFiles: make(map[string]string),
	}
}

// parseIncludes checks file for @includes and appends it to parseContext field Includes.
// If parseIncludes stumbles upon nonexistent file that is being included, that file is being appended to parseContext field MissingFiles.
func parseIncludes(ctx *parseContext, fileDir string, current string) error {

	ld(fmt.Sprintf("parse file on includes %s", fileDir))

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
			// current = "nil" if fileDir is not an include
			if current == "" {
				return nil
			}
			delete(ctx.Includes, fileDir)
			ctx.MissingFiles[fileDir] = current
			return nil
		}
		return fmt.Errorf("open %s: %w", fileDir, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	dir := filepath.Dir(fileDir)
	scanner := bufio.NewScanner(file)

	ld(fmt.Sprintf("parse file on includes %s", fileDir))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "@") {
			continue
		}

		m := includePattern.FindStringSubmatch(line)
		if m == nil {
			le(fmt.Sprintf("wrong include line in %s: %s", fileDir, line))
			// le("wrong include")
			continue
		}
		includeName := m[1]
		includeDir := filepath.Join(dir, includeName)

		ld(fmt.Sprintf("%s include %s dir %s", fileDir, includeName, dir))
		// ld("file include include dir")

		if _, exists := ctx.Includes[includeDir]; !exists {
			ctx.Includes[includeDir] = fileDir
		}

		if err := parseIncludes(ctx, includeDir, fileDir); err != nil {
			return fmt.Errorf("include %s -> %s: %w", fileDir, includeDir, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	ctx.State[fileDir] = done
	return nil
}

func getEntriesProjectMap(fsys fs.FS, dir string) (map[string]struct{}, error) {
	entriesProjectMap := make(map[string]struct{})
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("error reading dir: %w", err)
	}
	for _, entry := range entries {
		match := migrationPattern.MatchString(entry.Name())
		if !match {
			continue
		}
		projectPath := filepath.Join(dir, entry.Name())
		entriesProjectMap[projectPath] = struct{}{}
	}
	return entriesProjectMap, nil
}

func getEntriesModuleMap(fsys fs.FS, dir string) (map[string]struct{}, error) {
	entriesModuleMap := make(map[string]struct{})
	moduleDirs, err := getModuleDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("error getting info on modules' path: %w", err)
	}
	for _, moduleDir := range moduleDirs {
		moduleDir = filepath.ToSlash(moduleDir)
		moduleMigration := filepath.Join(moduleDir, "migrations")
		moduleMigration = filepath.ToSlash(moduleMigration)
		entries, err := fs.ReadDir(fsys, moduleMigration)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("error reading directory: %w", err)
		}
		for _, entry := range entries {
			match := migrationPattern.MatchString(entry.Name())
			if !match {
				continue
			}
			modulePath := filepath.Join(moduleMigration, entry.Name())
			entriesModuleMap[modulePath] = struct{}{}
		}
	}

	return entriesModuleMap, nil
}
