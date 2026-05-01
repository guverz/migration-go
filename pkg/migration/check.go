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
)

var (
	IncludePattern     = regexp.MustCompile(`^@([^;]+)`)
	MigrationUpPattern = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.up\.([^\.]+)$`)
	MigrationPattern   = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.(up|down)\.([^\.]+)$`)
)

type ListResults struct {
	ListWarnings []string // list of non-critical errors

	LostPairs       map[string]string        // key - missed migration file, value - existing pair
	MissedPairs     map[string]string        // key - missed migration file, value - existing pair
	DeletedFiles    map[string]string        // key - project migration, value - module migration (module file is missing)
	DeletedIncludes map[string]string        // key - include, value - included (include is being included; included includes)
	MissedIncludes  map[string]string        // key - include, value - included
	MissedFiles     map[string]MigrationInfo // key - upName of module file, value - MigrationInfo of that module file

	ProjectMigrations map[string]MigrationInfo // key - MD5, value - MigrationInfo of any original migration pair (map of original migration files (have no meta))
	ModuleMigrations  map[string]MigrationInfo // key - prefix of original migration file, value - MigrationInfo of migration file that references original
	ProjectIncludes   map[string]string        // key - include, value - included; include of project migration file (migration file has no meta)
	ModuleIncludes    map[string]string        // key - include, value - included; include of module migration file
}

type MigrationInfo struct {
	Prefix       string
	Ext          string
	Dir          string
	UpFileName   string
	DownFileName string
}

type Meta struct {
	MetaInfo MigrationInfo
	MD5      string
}

func MigrationList(fsys fs.FS, dir string) (*ListResults, error) {
	rslts := &ListResults{}
	rslts.MissedPairs = make(map[string]string)
	rslts.LostPairs = make(map[string]string)
	dir = filepath.ToSlash(filepath.Clean(dir))

	// getting project and module maps by reading migration directory
	projectEntriesMap, err := getEntriesProjectMap(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("error getting project entries map: %w", err)
	}
	moduleEntriesMap, err := getEntriesModuleMap(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("error getting module entries map: %w", err)
	}

	// getting map where key - project file, value - migration info of meta file (if md5 is an empty string then that project file is an original one)
	MetaMap, err := GetMetaMap(fsys, projectEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error getting map of projects: %w", err)
	}
	// getting map where key - concat md5, value - migration info of module file (only files with complete pairs)
	ModuleMap, err := GetModuleMap(moduleEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error getting map of modules: %w", err)
	}

	// FILLING IN MIGRATION FILE RELATED FIELDS OF LISTRESULTS STRUCT

	// ProjectMigrations
	rslts.ProjectMigrations, err = fillProjectMigrations(MetaMap)
	if err != nil {
		return nil, fmt.Errorf("error getting ProjectMigrations: %w", err)
	}

	// ModuleMigrations
	rslts.ModuleMigrations, err = fillModuleMigrations(MetaMap)
	if err != nil {
		return nil, fmt.Errorf("error getting ModuleMigrations: %w", err)
	}
	// CHECKING PAIRS OF MIGRATION FILES

	missingProjectPairs, err := checkPairs(projectEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error checking project pairs: %w", err)
	}
	missingModulePairs, err := checkPairs(moduleEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error checking module pairs: %w", err)
	}

	// missing pairs in modules' dirs can't be fixed, so it's being added to LostPairs immediately
	maps.Copy(rslts.LostPairs, missingModulePairs)

	// checking if missing pairs in project dir are original, if original - it can't be fixed, if based on module migration file - can be.
	lostPairs, missedPairs := processMissingProjectPairs(missingProjectPairs, MetaMap)
	maps.Copy(rslts.LostPairs, lostPairs)
	maps.Copy(rslts.MissedPairs, missedPairs)

	// CHECKING FOR DELETED OR MISSED MIGRATION FILES

	// deletedFiles
	rslts.DeletedFiles, err = checkDeletedFiles(MetaMap, moduleEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error checking for deleted files: %w", err)
	}
	// missedFiles
	rslts.MissedFiles = checkMissedFiles(rslts.ProjectMigrations, ModuleMap)

	// INCLUDES

	// filling in ParseContext for project, module and meta
	mapProjectIncludes, err := getProjectParseContext(projectEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error getting project includes information: %w", err)
	}
	mapModuleIncludes, err := getModuleParseContext(moduleEntriesMap)
	if err != nil {
		return nil, fmt.Errorf("error getting module includes information: %w", err)
	}
	mapMetaIncludes, err := getMetaParseContext(MetaMap)
	if err != nil {
		return nil, fmt.Errorf("error getting meta includes information: %w", err)
	}

	// ProjectMD5Includes map[string]string        // key - MD5, value - include; md5 of includes used in the project directory (used to be in ListResults)
	ProjectMD5Includes, err := getProjectMD5Includes(mapProjectIncludes)
	if err != nil {
		return nil, fmt.Errorf("error getting ProjectMD5Includes: %w", err)
	}

	// FILLING IN INCLUDES RELATED FIELDS OF LISTRESULTS STRUCT

	// ProjectIncludes
	rslts.ProjectIncludes = fillProjectIncludes(mapProjectIncludes, MetaMap)

	// ModuleIncludes
	rslts.ModuleIncludes, err = fillModuleIncludes(mapMetaIncludes, ProjectMD5Includes)
	if err != nil {
		return nil, fmt.Errorf("error getting ModuleIncludes: %w", err)
	}

	// DeletedIncludes
	rslts.DeletedIncludes, err = checkDeletedIncludes(mapProjectIncludes, MetaMap)
	if err != nil {
		return nil, fmt.Errorf("error checking migration directory for deleted includes: %w", err)
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

func processMissingProjectPairs(missingProjectPairs map[string]string, metaMap map[string]Meta) (map[string]string, map[string]string) {
	lostPairs := make(map[string]string)
	missedPairs := make(map[string]string)

	for missing, existing := range missingProjectPairs {
		meta := metaMap[existing]
		if meta.IsOriginal() {
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
			md5, err := FileMD5(include)
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

func checkMissedFilesForIncludes(missedFiles map[string]MigrationInfo) ([]string, map[string]map[string]string, error) {
	warnings := []string{}
	newMissedIncludes := make(map[string]map[string]string)
	for file, moduleInfo := range missedFiles {
		isUp := strings.HasSuffix(file, ".up.sql")
		if !isUp {
			continue
		}
		missedContext := NewParseContext()
		missedUpPath := filepath.Join(moduleInfo.Dir, moduleInfo.UpFileName)
		missedDownPath := filepath.Join(moduleInfo.Dir, moduleInfo.DownFileName)
		if err := ParseIncludes(missedContext, missedUpPath, ""); err != nil {
			return nil, nil, fmt.Errorf("error ParseIncludes: %w", err)
		}
		if err := ParseIncludes(missedContext, missedDownPath, ""); err != nil {
			return nil, nil, fmt.Errorf("error ParseIncludes: %w", err)
		}
		for include, included := range missedContext.MissingFiles {
			warnings = append(warnings, fmt.Sprintf("include file %s is missing in the module and it's being included by %s, need to fix it by hand", include, included))
		}
		newMissedIncludes[file] = missedContext.Includes
	}
	return warnings, newMissedIncludes, nil
}

func checkDeletedIncludes(mapProjectIncludes map[string]ParseContext, metaMap map[string]Meta) (map[string]string, error) {
	deletedIncludes := make(map[string]string)
	for upPath, projectContext := range mapProjectIncludes {
		meta := metaMap[upPath]
		dir := filepath.Dir(upPath)
		if meta.IsOriginal() {
			continue
		}
		migrationMD5Includes := make(map[string]string)
		for projectInclude, projectIncluded := range projectContext.Includes {
			md5, err := FileMD5(projectInclude)
			if err != nil {
				return nil, fmt.Errorf("error getting md5 of an include file: %w", err)
			}
			migrationMD5Includes[md5] = projectInclude

			includeDir, err := filepath.Rel(dir, projectInclude)
			if err != nil {
				return nil, fmt.Errorf("error getting relative path: %w", err)
			}
			metaIncludePath := filepath.Join(meta.MetaInfo.Dir, includeDir)
			if rslt, err := FindFileViaDir(metaIncludePath); err != nil {
				return nil, fmt.Errorf("findFileViaDir error: %w", err)
			} else if !rslt {
				deletedIncludes[projectInclude] = projectIncluded
			}
		}
	}
	return deletedIncludes, nil
}

func checkMissedIncludes(mapProjectIncludes map[string]ParseContext, metaMap map[string]Meta, mapModuleIncludes map[string]ParseContext) (map[string]string, error) {
	missedIncludes := make(map[string]string)
	for upFile, projectContext := range mapProjectIncludes {
		meta := metaMap[upFile]
		if meta.IsOriginal() {
			continue
		}
		migrationMD5Includes := make(map[string]string)
		for projectInclude := range projectContext.Includes {
			md5, err := FileMD5(projectInclude)
			if err != nil {
				return nil, fmt.Errorf("error getting md5 of an include file: %w", err)
			}
			migrationMD5Includes[md5] = projectInclude
		}
		moduleContext := mapModuleIncludes[filepath.Join(meta.MetaInfo.Dir, meta.MetaInfo.UpFileName)]

		for metaInclude, metaIncluded := range moduleContext.Includes {
			metaMD5, err := FileMD5(metaInclude)
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

func fillModuleMigrations(metaMap map[string]Meta) (map[string]MigrationInfo, error) {
	moduleMigrations := make(map[string]MigrationInfo)
	for file, meta := range metaMap {
		isUp := strings.HasSuffix(file, ".up.sql")
		if !isUp || meta.IsOriginal() {
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

func getProjectInfo(projectPath string) (MigrationInfo, error) {
	var (
		prefix   string
		ext      string
		dir      string
		upName   string
		downName string
	)
	dir = filepath.Dir(projectPath)
	matches := MigrationUpPattern.FindStringSubmatch(filepath.Base(projectPath))
	if matches == nil {
		return MigrationInfo{}, fmt.Errorf("wrong file name: %s", filepath.Base(projectPath))
	}
	prefix, ext = matches[1], matches[2]
	upName = prefix + ".up." + ext
	downName = prefix + ".down." + ext

	return MigrationInfo{
		Prefix:       prefix,
		Ext:          ext,
		Dir:          dir,
		UpFileName:   upName,
		DownFileName: downName,
	}, nil
}

func fillProjectMigrations(metaMap map[string]Meta) (map[string]MigrationInfo, error) {
	projectMigrations := make(map[string]MigrationInfo)
	for file, meta := range metaMap {
		fileName := filepath.Base(file)
		matches := MigrationUpPattern.FindStringSubmatch(fileName)
		if matches == nil {
			continue
		}
		// if meta.IsEmpty == true then that file has no meta, therefore migrationInfo needs to be calculated
		if meta.IsOriginal() {
			prefix, ext := matches[1], matches[2]
			dir := filepath.Dir(file)
			upName := prefix + ".up." + ext
			downName := prefix + ".down." + ext
			upPath := filepath.Join(dir, upName)
			downPath := filepath.Join(dir, downName)

			md5, err := ConcatMD5(upPath, downPath)
			if err != nil {
				return nil, fmt.Errorf("error getting concat of MD5: %w", err)
			}
			// md5 is empty if migration pair is incomplete
			if md5 == "" {
				continue
			}
			projectMigrations[md5] = MigrationInfo{
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

func fillProjectIncludes(projectContextMap map[string]ParseContext, metaMap map[string]Meta) map[string]string {
	projectIncludes := make(map[string]string)
	for upFile, projectContext := range projectContextMap {
		meta := metaMap[upFile]
		if meta.IsOriginal() {
			maps.Copy(projectIncludes, projectContext.Includes)
		}
	}
	return projectIncludes
}

// getting ListResults field - ModuleIncludes
func fillModuleIncludes(moduleContextMap map[string]ParseContext, projectMD5Includes map[string]string) (map[string]string, error) {
	moduleIncludes := make(map[string]string)
	for _, ModuleContext := range moduleContextMap {
		for include, included := range ModuleContext.Includes {
			md5, err := FileMD5(include)
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

func getProjectMD5Includes(projectContextMap map[string]ParseContext) (map[string]string, error) {
	projectMD5Includes := make(map[string]string)
	for _, ProjectContext := range projectContextMap {
		for include := range ProjectContext.Includes {
			md5, err := FileMD5(include)
			if err != nil {
				return nil, fmt.Errorf("error calculating md5 of include: %w", err)
			}
			projectMD5Includes[md5] = include
		}
	}
	return projectMD5Includes, nil
}

func getProjectParseContext(projectMap map[string]struct{}) (map[string]ParseContext, error) {
	projectParseContext := make(map[string]ParseContext)
	for upFile := range projectMap {
		projectContext := NewParseContext()
		isUp := strings.HasSuffix(upFile, ".up.sql")
		if !isUp {
			continue
		}
		downFile, err := switchMigrationType(upFile, "down")
		if err != nil {
			return nil, fmt.Errorf("error switching migration type to down: %w", err)
		}
		if err := ParseIncludes(projectContext, upFile, ""); err != nil {
			return nil, fmt.Errorf("error parsing project (up) for includes: %w", err)
		}
		if err := ParseIncludes(projectContext, downFile, ""); err != nil {
			return nil, fmt.Errorf("error parsing project (down) for includes: %w", err)
		}
		projectParseContext[upFile] = *projectContext
	}
	return projectParseContext, nil
}

func getModuleParseContext(moduleMap map[string]struct{}) (map[string]ParseContext, error) {
	moduleParseContext := make(map[string]ParseContext)
	for upFile := range moduleMap {
		moduleContext := NewParseContext()
		isUp := strings.HasSuffix(upFile, ".up.sql")
		if !isUp {
			continue
		}
		downFile, err := switchMigrationType(upFile, "down")
		if err != nil {
			return nil, fmt.Errorf("error switching migration type to down: %w", err)
		}
		if err := ParseIncludes(moduleContext, upFile, ""); err != nil {
			return nil, fmt.Errorf("error parsing module (up) for includes: %w", err)
		}
		if err := ParseIncludes(moduleContext, downFile, ""); err != nil {
			return nil, fmt.Errorf("error parsing module (down) for includes: %w", err)
		}
		moduleParseContext[upFile] = *moduleContext
	}
	return moduleParseContext, nil
}

func getMetaParseContext(metaMap map[string]Meta) (map[string]ParseContext, error) {
	metaParseContext := make(map[string]ParseContext)
	for upFile, meta := range metaMap {
		metaContext := NewParseContext()
		isUp := strings.HasSuffix(upFile, ".up.sql")
		if !isUp || meta.IsOriginal() {
			continue
		}
		upPath := filepath.Join(meta.MetaInfo.Dir, meta.MetaInfo.UpFileName)
		downPath := filepath.Join(meta.MetaInfo.Dir, meta.MetaInfo.DownFileName)
		if err := ParseIncludes(metaContext, upPath, ""); err != nil {
			return nil, fmt.Errorf("error parsing meta (up) for includes: %w, file: %s", err, upPath)
		}
		if err := ParseIncludes(metaContext, downPath, ""); err != nil {
			return nil, fmt.Errorf("error parsing meta (down) for includes: %w, file: %s", err, downPath)
		}
		metaParseContext[upFile] = *metaContext
	}
	return metaParseContext, nil
}

func switchMigrationType(filename, newDirection string) (string, error) {
	parts := strings.Split(filename, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("filename is wrong format: %s", filename)
	}
	parts[len(parts)-2] = newDirection
	return strings.Join(parts, "."), nil
}

func checkPairs(projectMap map[string]struct{}) (map[string]string, error) {
	incompletePairs := make(map[string]string)
	for entryPath := range projectMap {
		isUp := strings.HasSuffix(entryPath, ".up.sql")
		if isUp {
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
		} else {
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
		}
	}
	return incompletePairs, nil
}

func checkMissedFiles(projectMigrations map[string]MigrationInfo, moduleMap map[string]MigrationInfo) map[string]MigrationInfo {
	missedFiles := make(map[string]MigrationInfo)
	for md5, moduleInfo := range moduleMap {
		if _, exists := projectMigrations[md5]; !exists {
			missedFiles[moduleInfo.UpFileName] = moduleInfo
			missedFiles[moduleInfo.DownFileName] = moduleInfo
		}
	}
	return missedFiles
}

func checkDeletedFiles(metaMap map[string]Meta, moduleMap map[string]struct{}) (map[string]string, error) {
	deletedFiles := make(map[string]string)
	for projectPath, meta := range metaMap {
		matches := MigrationUpPattern.FindStringSubmatch(filepath.Base(projectPath))
		if matches == nil {
			continue
		}
		if meta.IsOriginal() {
			continue
		}
		dir := filepath.Dir(projectPath)
		prefix, ext := matches[1], matches[2]
		upProjectPath := filepath.Join(dir, prefix+".up."+ext)
		downProjectPath := filepath.Join(dir, prefix+".down."+ext)

		upPath := filepath.Join(meta.MetaInfo.Dir, fmt.Sprintf("%s.up.%s", meta.MetaInfo.Prefix, meta.MetaInfo.Ext))
		downPath := filepath.Join(meta.MetaInfo.Dir, fmt.Sprintf("%s.down.%s", meta.MetaInfo.Prefix, meta.MetaInfo.Ext))

		_, upExists := moduleMap[upPath]
		_, downExists := moduleMap[downPath]
		if !upExists && !downExists {
			deletedFiles[upProjectPath] = upPath
			deletedFiles[downProjectPath] = downPath
		}
	}
	return deletedFiles, nil
}

func GetModuleMap(moduleEntriesMap map[string]struct{}) (map[string]MigrationInfo, error) {
	moduleMap := make(map[string]MigrationInfo)
	for entry := range moduleEntriesMap {
		moduleInfo, md5, err := GetModule(entry)
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

func GetModule(entry string) (MigrationInfo, string, error) {
	dir := filepath.Dir(entry)
	entryName := filepath.Base(entry)
	matches := MigrationPattern.FindStringSubmatch(entryName)
	if matches == nil {
		return MigrationInfo{}, "", fmt.Errorf("wrong name of module migration %s", entryName)
	}

	prefix, ext := matches[1], matches[3]

	upName := prefix + ".up." + ext
	downName := prefix + ".down." + ext
	upPath := filepath.Join(dir, upName)
	downPath := filepath.Join(dir, downName)

	md5, err := ConcatMD5(upPath, downPath)
	if err != nil {
		return MigrationInfo{}, "", fmt.Errorf("error getting concat of MD5: %w", err)
	}
	return MigrationInfo{
		Prefix:       prefix,
		Ext:          ext,
		Dir:          dir,
		UpFileName:   upName,
		DownFileName: downName,
	}, md5, nil
}

func ConcatMD5(upPath, downPath string) (string, error) {
	md5Up, err := FileMD5(upPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("FileMD5 error: %w", err)
	}
	md5Down, err := FileMD5(downPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("FileMD5 error: %w", err)
	}

	return md5Up + md5Down, nil
}

func (m Meta) IsOriginal() bool {
	return m.MD5 == ""
}

func GetMetaMap(fsys fs.FS, projectMap map[string]struct{}) (map[string]Meta, error) {
	metaMap := make(map[string]Meta)
	for projectPath := range projectMap {
		projectPathTemp := filepath.ToSlash(projectPath)
		metaEntry, md5, err := getMetaInfo(fsys, projectPathTemp)
		if err != nil {
			return nil, fmt.Errorf("error getting project: %w", err)
		}
		metaMap[projectPath] = Meta{
			MetaInfo: metaEntry,
			MD5:      md5,
		}
	}
	return metaMap, nil
}

func getMetaInfo(fsys fs.FS, projectPath string) (MigrationInfo, string, error) {
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
		return MigrationInfo{}, "", fmt.Errorf("error opening file: %w", err)
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
				return MigrationInfo{}, "", fmt.Errorf("wrong meta field: %s", meta)
			}
			relPathFileName := parts[0]
			metaMD5 = parts[1]

			fileName := filepath.Base(relPathFileName)
			path = filepath.Join(filepath.Dir(projectDir), filepath.Dir(relPathFileName))
			// check for meta in the migration file
			matches := MigrationPattern.FindStringSubmatch(fileName)
			if matches == nil {
				return MigrationInfo{}, "", fmt.Errorf("wrong migration name: %s", fileName)
			}
			prefix = matches[1]
			ext = matches[3]
			upName = prefix + ".up." + ext
			downName = prefix + ".down." + ext
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return MigrationInfo{}, "", fmt.Errorf("scanner error: %w", err)
	}
	return MigrationInfo{
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
