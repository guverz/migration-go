package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Help = `migration helper to create migrations scripts
usage: migration [-h|--help] [-V|--version] add
options:
        -h|--help      print this help and exit
        -V|--version   print script version and exit
commands:
        add            add new migrations script with properly defined name
        collect        collect migrations on submodules between commits into migrations catalog
        check          check unregtistered migrations files at submodules`
	MiniHelpDir   = "migration.template.sql"
	MigrationDir  = "./test/migrations"
	IncludeHelp   = true
	TestDir       = "./test"
	checkIncludes = "./test/ddl/migrations"
)

type MigrationMeta struct {
	Prefix          string
	Ext             string
	Dir             string
	UpFile          string
	DownFile        string
	IsFromSubmodule bool
	OriginalPath    string
	MD5             string
	ProjectName     string
}

type MigrationPair struct {
	Prefix      string
	Ext         string
	Dir         string
	UpFile      string
	DownFile    string
	ProjectName string
	ModulePath  string // Путь к модулю, если это подмодуль
}

type IncludeInfo struct {
	IncludingFile string
	IncludedFile  string
	MD5           string
}

func main() {

	var (
		helpFlag    bool
		versionFlag bool
	)

	flag.Usage = func() {}

	flag.BoolVar(&helpFlag, "h", false, "print help and exit")
	flag.BoolVar(&helpFlag, "help", false, "print help and exit")
	flag.BoolVar(&versionFlag, "V", false, "print script version and exit")
	flag.BoolVar(&versionFlag, "version", false, "print script version and exit")

	err := flag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Unknown flag provided\n")
		os.Exit(1)
	}

	switch {
	case helpFlag:
		help()
		os.Exit(0)
	case versionFlag:
		version()
		os.Exit(0)
	}

	if flag.NArg() == 0 && flag.NFlag() > 0 {
		fmt.Fprintf(os.Stderr, "Error: Unknown flag provided\n")
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		os.Exit(0)
	}

	switch args[0] {
	case "add":
		add()
		os.Exit(0)
	case "collect":
		collect()
		os.Exit(0)
	case "check":
		check()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command '%s'\n", args[0])
		os.Exit(0)
	}
}

func help() {
	fmt.Println(Help)
}

func minihelp() (string, error) {
	text, err := os.ReadFile(MiniHelpDir)
	if err != nil {
		return "", fmt.Errorf("Error reading MiniHelp: %v", err)
	}
	return string(text), nil
}

func version() {
	Version := "0.1"
	fmt.Println(Version)
}

func describe(arg string) (string, error) {
	cmd := exec.Command("version", arg)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run version %v, %v", arg, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func add() error {
	baseName, err := describe("full")
	if err != nil {
		return fmt.Errorf("failed to get git repo information")
	}

	if err := os.MkdirAll(MigrationDir, 0755); err != nil {
		return err
	}

	increment, err := FindLastMigrationNumber(MigrationDir, baseName)
	if err != nil {
		return fmt.Errorf("failed to find last migration: %v", err)
	}
	increment++

	migrationFile := fmt.Sprintf("%s-%d", baseName, increment)
	err = CreateMigrationFiles(MigrationDir, migrationFile, IncludeHelp)
	if err != nil {
		return fmt.Errorf("failed to create migration files: %v", err)
	}

	fmt.Printf("Created migration files:\n   %s/%s.up.sql\n   %s/%s.down.sql\n",
		MigrationDir, migrationFile, MigrationDir, migrationFile)

	return nil
}

func FindLastMigrationNumber(dir, baseName string) (int, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`^%s-(\d+)\.(up|down)\.sql$`, regexp.QuoteMeta(baseName)))
	var maxNum int

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory %s: %v", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := pattern.FindStringSubmatch(entry.Name())
		if len(matches) > 1 {
			num, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}
			if num > maxNum {
				maxNum = num
			}
		}
	}

	return maxNum, nil
}

func CreateMigrationFiles(dir, baseName string, includeHelp bool) error {
	upContent := fmt.Sprintf("# %s.up.sql\n", baseName)
	GetMiniHelp, _ := minihelp()
	if includeHelp {
		upContent += GetMiniHelp + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, baseName+".up.sql"), []byte(upContent), 0644); err != nil {
		return err
	}

	downContent := fmt.Sprintf("# %s.down.sql\n", baseName)
	if includeHelp {
		downContent += GetMiniHelp + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, baseName+".down.sql"), []byte(downContent), 0644); err != nil {
		return err
	}

	return nil
}

func check() {
	if err := migrationList(MigrationDir); err != nil {
		log.Printf("error migrationList: %s", err)
	}
}

func collect() {
	fmt.Println("collect_temp")
}

func migrationList(dir string) error {
	// missedIncludesCnt := 0
	// deletedIncludesCnt := 0
	t0 := time.Now()
	migrationIncludes := make(map[string]string)
	// pathFileMd5 := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", dir, err)
	}

	fileMap := make(map[string]bool)
	for _, entry := range entries {
		fileMap[entry.Name()] = true
	}

	pattern := regexp.MustCompile(`(.+\-[0-9\.\-]+)\.up\.([^\.]+)$`)
	for _, entry := range entries {
		matches := pattern.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		prefix, ext := matches[1], matches[2]
		downFileName := fmt.Sprintf("%s.down.%s", prefix, ext)

		if _, exists := fileMap[downFileName]; !exists {
			log.Printf("missing down file for up file: %s)", entry.Name())
		}

		filePath := filepath.Join(dir, entry.Name())
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if meta, ok := strings.CutPrefix(line, "#migration:"); ok {
				meta = strings.TrimSpace(meta)

				parts := strings.SplitN(meta, ";", 2)
				if len(parts) == 0 {
					continue
				}
				pathFileName := parts[0]
				// md5 := ""
				// if len(parts) == 2 {
				// 	md5 = parts[1]
				// }
				fileName := filepath.Base(pathFileName)
				path := filepath.Dir(pathFileName)

				matches := pattern.FindStringSubmatch(fileName)
				if matches == nil {
					log.Printf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext \n", file.Name())
					continue
				}

				ext := matches[2]
				prefix := matches[1]

				simPath := filepath.Join(TestDir, path)

				// upPath := filepath.Join(simPath, prefix+".up."+ext)
				// downPath := filepath.Join(simPath, prefix+".down."+ext)

				upName := prefix + ".up." + ext
				downName := prefix + ".down." + ext
				filePathUp := filepath.Join(simPath, upName)
				filePathDown := filepath.Join(simPath, downName)
				// include, err := parseIncludeFile(simPath, upName)
				// if err != nil {
				// 	return fmt.Errorf("parseIncludeFile error: %w", err)
				// }

				// for _, el := range include {
				// 	if _, exists := migrationIncludes[el]; !exists {
				// 		migrationIncludes[el] = true
				// 	}
				// }
				if err := parseIncludeFileMap(filePathUp, migrationIncludes); err != nil {
					return fmt.Errorf("parseIncludeFile error: %w", err)
				}
				if err := parseIncludeFileMap(filePathDown, migrationIncludes); err != nil {
					return fmt.Errorf("parseIncludeFile error: %w", err)
				}

				// include, err = parseIncludeFile(simPath, downName)
				// if err != nil {
				// 	return fmt.Errorf("parseIncludeFile error: %w", err)
				// }
				// for _, el := range include {
				// 	if _, exists := migrationIncludes[el]; !exists {
				// 		migrationIncludes[el] = true
				// 	}
				// }

				rslt, err := findFileViaPath(simPath, downName)
				if err != nil {
					log.Printf("findFileViaPath error: %s", err)
				}
				if !rslt {
					fmt.Printf("down-file %s not found in %s\n", downName, simPath)
					rslt, err = findFileViaPath(simPath, upName)
					if err != nil {
						log.Printf("findFileViaPath error: %s", err)
					}
					if !rslt {
						fileInfo, err := os.Stat(simPath)
						if err != nil {
							if os.IsNotExist(err) {
								return fmt.Errorf("file doesn't exist: %s", simPath)
							}
							return fmt.Errorf("can't get file info: %w", err)
						}

						if !fileInfo.Mode().IsRegular() {
							return fmt.Errorf("%s is not a regual file (it is %s)", simPath, fileInfo.Mode())
						}
						// also add check if name argument is a regular file
						// os.Remove(filepath.Join(simPath, upName))
					}
				}
				break
			}
		}

	}
	for el := range migrationIncludes {
		fmt.Println(el)
	}
	fmt.Println(time.Since(t0))
	return nil
}

func parseIncludeFile(dir, base string) ([]string, error) {
	rslt := []string{}
	included := make(map[string]bool)
	pattern := regexp.MustCompile(`^@([^;]+)`)

	filePath := filepath.Join(dir, base)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return rslt, fmt.Errorf("File doesn't exist: %s", filePath)
		}
		return rslt, fmt.Errorf("can't get file info: %w", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return rslt, fmt.Errorf("Warning: cannot open file %s: %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := pattern.FindStringSubmatch(line)
		if matches == nil {
			// log.Printf("Wrong line: '%s' in the file '%s' in the dir '%s'.", line, file.Name(), dir)
			continue
		}
		// checking if new include has already added to map, if not it adds new incldue
		if _, exists := included[matches[0]]; !exists {

			included[matches[0]] = true
			// log.Printf("Found '%s' in file '%s'", matches[0][1:], file.Name())
			rslt = append(rslt, matches[0][1:])
		}
		// it can return multiple matches, maybe this fuction should have a pointer to slice in order to append matches to it

		// kinda expecting to do it recursive the same way the bash script did
		// parseIncludeFile(filepath.Dir(matches[0][1:]), filepath.Base(matches[0][1:]))
	}

	return rslt, nil
}

func parseIncludeFileMap(filePath string, included map[string]string) error {
	pattern := regexp.MustCompile(`^@([^;]+)`)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file doesn't exist: %s", filePath)
		}
		return fmt.Errorf("cannot open file %s: %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "@") {
			continue
		}

		matches := pattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		include := matches[1] //getting rid of @
		// checking if new include has already added to map, if not it adds new incldue
		if parent, exists := included[include]; exists && filePath != parent {
			return fmt.Errorf("include loop detected %s included by %s already included by %s", include, filePath, parent)
		} else {
			included[include] = filePath
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading file %s: %w", filePath, err)
	}

	return nil
}

func findFileViaPath(path string, fileName string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, fmt.Errorf("failed to read directory %s: %v", path, err)
	}

	for _, entry := range entries {
		if entry.Name() == fileName {
			return true, nil
		}
	}
	return false, nil
}
