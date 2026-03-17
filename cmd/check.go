package cmd

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
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
	MissedFilesCnt   int
	MissedMigrations map[string]Meta
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "short check description",
	Long:  `long check description`,
	Run: func(cmd *cobra.Command, args []string) {
		check()
	},
}

func check() {
	rslt := &ListResults{}
	if err := MigrationList(MigrationDir, rslt); err != nil {
		log.Printf("error migrationList: %s", err)
	}
	if rslt.MissedFilesCnt != 0 {
		log.Printf("there is unregistered migration files pairs %d, collect them and commit:", rslt.MissedFilesCnt)
		for _, file := range rslt.MissedMigrations {
			log.Print(file.Prefix + ".up|down." + file.Ext)
		}
		log.Print("do: scripts/migration collect")
	}
	if err := MigrationValidation(MigrationDir); err != nil {
		log.Printf("error migration validation: %s", err)
	}
}

func collect() {
	fmt.Println("collect_temp")
}

func MigrationList(dir string, rslt *ListResults) error {
	rslt.MissedFilesCnt = 0
	missedIncludesCnt := 0
	deletedIncludesCnt := 0

	missedIncludes := make(map[string]string)
	deletedIncludes := make(map[string]string)
	moduleIncludes := make(map[string]string)
	projectIncludes := make(map[string]string)

	rslt.MissedMigrations = make(map[string]Meta)
	projectMigrations := make(map[[32]byte]Meta)
	moduleMigrations := make(map[[32]byte]Meta)

	// t0 := time.Now()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", dir, err)
	}

	fileMap := make(map[string]bool)
	for _, entry := range entries {
		fileMap[entry.Name()] = true
	}

	for _, entry := range entries {
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
			// probably should return fmt.Errorf instead of log & continue (?)
			log.Printf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
			continue
		}

		fileDirUp := filepath.Join(dir, entry.Name())
		fileDirDown := filepath.Join(dir, downFileName)

		if err := parseIncludes(fileDirUp, "", state, migrationIncludes); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		if err := parseIncludes(fileDirDown, "", state, migrationIncludes); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		file, err := os.Open(fileDirUp)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		foundMetaFlag := false
		for scanner.Scan() {
			line := scanner.Text()
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
						return fmt.Errorf("Error decoding MD5 string in meta: %w", err)
					}
					copy(md5[:], decoded)
				}
				fileName := filepath.Base(pathFileName)
				path := filepath.Dir(pathFileName)
				// check for meta in the migration file
				matches := ListPattern.FindStringSubmatch(fileName)
				if matches == nil {
					// probably should return err instead of log & continue (?)
					log.Printf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext", file.Name())
					continue
				}

				ext := matches[2]
				prefix := matches[1]

				metaDir := filepath.Join(filepath.Dir(MigrationDir), path)

				metaUpName := prefix + ".up." + ext
				metaDownName := prefix + ".down." + ext
				metaFileDirUp := filepath.Join(metaDir, metaUpName)
				metaFileDirDown := filepath.Join(metaDir, metaDownName)
				// log.Printf("UP %s, DOWN %s refered by %s", metaFileDirUp, metaFileDirDown, file.Name())

				if rslt, err := findFileViaDir(metaFileDirDown); err != nil {
					return fmt.Errorf("findFileViaDir error: %w", err)
				} else if !rslt {
					if rslt, err = findFileViaDir(metaFileDirUp); err != nil {
						return fmt.Errorf("findFileViaDir error: %w", err)
					} else if !rslt {
						log.Printf("migration %s does not have based migration file %s", file.Name(), metaUpName)
						// WARNING dangerous operation, check fileDirUp is not zero and this is a regular file
						if fileInfo, err := os.Stat(fileDirUp); err == nil && fileInfo.Mode().IsRegular() {
							log.Printf("delete %s", fileDirUp)
							os.Remove(fileDirUp)
						}

						// WARNING dangerous operation, check fileDirDown is not zero and this is a regular file
						if fileInfo, err := os.Stat(fileDirDown); err == nil && fileInfo.Mode().IsRegular() {
							log.Printf("delete %s", fileDirDown)
							os.Remove(fileDirDown)
						}
						continue

					} else {
						return fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaUpName, metaDownName, metaDir)
					}
				} else {
					if rslt, err = findFileViaDir(metaFileDirUp); err != nil {
						return fmt.Errorf("findFileViaDir error: %w", err)
					} else if !rslt {
						return fmt.Errorf("BUG: file %s do not have counterpart file %s at '%s'", metaDownName, metaUpName, metaDir)
					}
				}

				projectMigrations[md5] = Meta{
					Prefix:       prefix,
					Ext:          ext,
					Dir:          metaDir,
					UpFileName:   metaUpName,
					DownFileName: metaDownName,
				}
				// if projectMigrations[md5].Dir == "test\\roam-cdr\\migrations" {
				// 	log.Printf("meta roam-cdr %s, referenced by %s", projectMigrations[md5].Prefix, file.Name())
				// 	// log.Printf("roam-cdr md5: %x, meta: %+v, referenced by %s", md5, projectMigrations[md5], file.Name())
				// } else {
				// 	// log.Printf("md5: %x, meta: %+v, referenced by %s", md5, projectMigrations[md5], file.Name())
				// 	log.Printf("meta index: %s, referenced by %s", projectMigrations[md5].Prefix, file.Name())
				// }

				moduleMigrations[md5] = Meta{
					Prefix:       filePrefix,
					Ext:          fileExt,
					Dir:          dir,
					UpFileName:   upFileName,
					DownFileName: downFileName,
				}

				if err := parseIncludes(metaFileDirDown, "", metaState, originalIncludes); err != nil {
					return fmt.Errorf("parseIncludes error: %w", err)
				}
				if err := parseIncludes(metaFileDirUp, "", metaState, originalIncludes); err != nil {
					return fmt.Errorf("parseIncludes error: %w", err)
				}
				migrationMD5Includes := make(map[[16]byte]string)
				projectMD5Includes := make(map[[16]byte]string)

				for include, included := range migrationIncludes {
					md5Include, err := FileMD5(include)
					if err != nil {
						return fmt.Errorf("FileMD5 error: %w", err)
					}
					migrationMD5Includes[md5Include] = include
					projectMD5Includes[md5Include] = include
					includeDir, err := filepath.Rel(filepath.Clean(dir), include)
					if err != nil {
						return err
					}
					metaInclude := filepath.Join(metaDir, includeDir)
					// log.Printf("created MD5 %x of include %s", md5Include, include)
					// log.Printf("md5 %x of include file %s included by %s and check in original includes at %s", md5Include, include, included, metaDir)
					if rslt, err := findFileViaDir(metaInclude); err != nil {
						return fmt.Errorf("findFileViaDir error: %w", err)
					} else if !rslt {
						log.Printf("include %s may be deleted from %s, check later", include, metaInclude)
						deletedIncludes[include] = included
						deletedIncludesCnt++
					}

				}

				for include, included := range originalIncludes {
					md5Include, err := FileMD5(include)
					if err != nil {
						return fmt.Errorf("FileMD5 error: %w", err)
					}
					if _, exists := migrationMD5Includes[md5Include]; !exists {
						if _, exists := missedIncludes[include]; !exists {
							log.Printf("MD5 %x of include %s is not present in migrationMD5Includes", md5Include, include)
							// log.Printf("include file %s is changed or not exists in migration includes", include)
							missedIncludesCnt++
							missedIncludes[include] = included
						}
					} else {
						moduleIncludes[include] = included
					}
				}

			}
		}
		// if meta is undefined, migration file is original file
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
			// log.Printf("NO META md5: %x, meta: %+v, referenced by %s", ogFileMD5UpDown, projectMigrations[ogFileMD5UpDown], file.Name())
			// log.Printf("NO META meta: %s, referenced by %s", projectMigrations[ogFileMD5UpDown].Prefix, file.Name())
			maps.Copy(projectIncludes, migrationIncludes)
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("Scanner error: %w", err)
		}
		// log.Printf("File %s has been processed", entry.Name())
	}
	// getting submodule dir
	submoduleDirSlice, err := getSubmoduleDir(MigrationDir)
	MigrationDirName := filepath.Base(MigrationDir)
	if err != nil {
		return err
	}
	for _, submoduleDir := range submoduleDirSlice {
		submoduleProject := ""
		submoduleMigration := filepath.Join(submoduleDir, MigrationDirName)
		entries, err := os.ReadDir(submoduleMigration)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			} else {
				return err
			}
		}
		submoduleProject, err = getProject(submoduleDir)
		if err != nil {
			return err
		}
		// log.Printf("That submodule %s has a name of %s", submodule, submoduleProject)

		fileMap := make(map[string]bool, len(entries))
		for _, entry := range entries {
			fileMap[entry.Name()] = true
		}

		for _, entry := range entries {
			matches := ListPattern.FindStringSubmatch(entry.Name())
			if len(matches) != 3 {
				continue
			}

			filePrefix, fileExt := matches[1], matches[2]

			if !strings.HasPrefix(entry.Name(), submoduleProject) {
				log.Printf("file not started with project name: %s", submoduleProject)
				filePrefix = strings.TrimSuffix(entry.Name(), ".up.sql")
			}

			upFileName := fmt.Sprintf("%s.up.%s", filePrefix, fileExt)
			downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)

			fileDirUp := filepath.Join(submoduleMigration, upFileName)
			fileDirDown := filepath.Join(submoduleMigration, downFileName)

			if _, exists := fileMap[downFileName]; !exists {
				// probably should return fmt.Errorf instead of log & continue (?)
				log.Printf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
				continue
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
			// log.Printf("MD5: %x, Prefix: %s, Ext: %s, Dir: %s, UpFile: %s, DownFile: %s", md5SubmoduleUpDown, filePrefix, fileExt, submoduleMigration, upFileName, downFileName)

			if _, exists := projectMigrations[md5SubmoduleUpDown]; !exists {
				rslt.MissedMigrations[upFileName] = Meta{
					Prefix:       filePrefix,
					Ext:          fileExt,
					Dir:          submoduleMigration,
					UpFileName:   upFileName,
					DownFileName: downFileName,
				}
				rslt.MissedFilesCnt++
				// log.Printf("Missed migrations file name: %s, Meta: %+v", upFileName, rslt.MissedMigrations[upFileName])
			}
		}
	}
	// for md5, meta := range projectMigrations {
	// 	if meta.Dir == "test\\roam-cdr\\migrations" {
	// 		log.Printf("MD5: %x, Meta: %+v", md5, meta)
	// 	}
	// }
	// fmt.Println(time.Since(t0))
	return nil
}

func findFileViaDir(fileDir string) (bool, error) {
	path := filepath.Dir(fileDir)
	base := filepath.Base(fileDir)

	entries, err := os.ReadDir(path)
	if err != nil {
		return false, fmt.Errorf("failed to read directory %s: %v", path, err)
	}

	for _, entry := range entries {
		if entry.Name() == base {
			return true, nil
		}
	}
	return false, nil
}

func parseIncludes(fileDir string, current string, state map[string]int, parent map[string]string) error {
	if state[fileDir] == visiting {
		return fmt.Errorf("include loop detected %s included by %s already included by %s", fileDir, current, parent[fileDir])
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

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "@") {
			continue
		}

		m := IncludePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		includeName := m[1]
		includeDir := filepath.Join(dir, includeName)
		if _, exists := parent[includeDir]; !exists {
			parent[includeDir] = fileDir
		}

		if err := parseIncludes(includeDir, fileDir, state, parent); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
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
		return nil, fmt.Errorf("Error opening .gitmodules: %w", err)
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
		return nil, fmt.Errorf("Scanner error: %w", err)
	}
	return rslt, nil
}

// bad function no clue how to parse otherwise ddl directory, also not sure about describe function using version-go
func getProject(repoPath string) (string, error) {
	if rslt := strings.Contains(repoPath, "ddl"); rslt {
		baseDescribe, err := Describe(MigrationDir, "project")
		if err != nil {
			return "", err
		}
		baseSplit := strings.SplitN(baseDescribe, "-", 2)
		if len(baseSplit) != 2 {
			return "", fmt.Errorf("Can't work with ddl directory")
		}
		base := baseSplit[0]
		stringRslt := (base + "-ddl")
		return stringRslt, nil
	}
	configPath := filepath.Join(repoPath, ".git", "config")

	f, err := os.Open(configPath)
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("origin url not found at path %s", repoPath)
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
			return err
		}
		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, "README.md") || strings.HasSuffix(path, ".txt") {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return err
	}
	for _, file := range files {
		fileName := filepath.Base(file)
		migrations[fileName] = file

		if ValidationPattern.MatchString(fileName) {
			if err := parseIncludes(file, "", state, migrationIncludes); err != nil {
				return fmt.Errorf("Error parsing Includes of %s, Error: %w", fileName, err)
			}
		}
	}

	for fileName, fileDir := range migrations {
		relative := strings.TrimPrefix(fileDir, path+"/")
		matches := ValidationPattern.FindStringSubmatch(fileName)
		if matches == nil {
			if _, exists := migrationIncludes[relative]; !exists {
				fmt.Printf("ERROR: %s wrong suffix\n", fileName)
				wrongFilesCnt++
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
		if _, exists := migrations[counterpart]; !exists {
			log.Printf("ERROR: %s counterpart %s not found\n", fileName, counterpart)
			wrongFilesCnt++
		}
		if wrongFilesCnt > 0 {
			return fmt.Errorf("there are %d wrong files", wrongFilesCnt)
		}
	}
	return nil
}
