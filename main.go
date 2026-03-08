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
        check          check unregistered migrations files at submodules`
	MiniHelpDir  = "migration.template.sql"
	MigrationDir = "./test/migrations"
	IncludeHelp  = true
)

var (
	IncludePattern = regexp.MustCompile(`^@([^;]+)`)
	ListPattern    = regexp.MustCompile(`(.+\-[0-9\.\-]+)\.up\.([^\.]+)$`)
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
	parent := make(map[string]string)
	// missedIncludesCnt := 0
	// deletedIncludesCnt := 0
	t0 := time.Now()

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
		parsed := make(map[string]bool)
		parsing := make(map[string]bool)

		matches := ListPattern.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		prefix, ext := matches[1], matches[2]
		downFileName := fmt.Sprintf("%s.down.%s", prefix, ext)

		if _, exists := fileMap[downFileName]; !exists {
			// probably should return fmt.Errorf instead of log & continue (?)
			log.Printf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
			continue
		}

		fileDirUp := filepath.Join(dir, entry.Name())
		fileDirDown := filepath.Join(dir, entry.Name())

		if err := parseIncludes(fileDirUp, parsed, parsing, parent); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		if err := parseIncludes(fileDirDown, parsed, parsing, parent); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		file, err := os.Open(fileDirUp)
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
				// check for meta in the migration file
				matches := ListPattern.FindStringSubmatch(fileName)
				if matches == nil {
					// probably should return err instead of log & continue (?)
					log.Printf("in file %s wrong meta #migration expect at name-x[.y[.z][-r].up.ext \n", file.Name())
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
						fileInfo, err := os.Stat(fileDirUp)
						if err != nil {
							if os.IsNotExist(err) {
								return fmt.Errorf("file doesn't exist: %s", fileDirUp)
							}
							return fmt.Errorf("can't get file info: %w", err)
						}

						if !fileInfo.Mode().IsRegular() {
							return fmt.Errorf("%s is not a regual file (it is %s)", fileDirDown, fileInfo.Mode())
						}
						log.Printf("delete %s\n", fileDirUp)
						// os.Remove(fileDirUp)

						fileInfo, err = os.Stat(fileDirDown)
						if err != nil {
							if os.IsNotExist(err) {
								return fmt.Errorf("file doesn't exist: %s", fileDirDown)
							}
							return fmt.Errorf("can't get file info: %w", err)
						}

						if !fileInfo.Mode().IsRegular() {
							return fmt.Errorf("%s is not a regual file (it is %s)", fileDirDown, fileInfo.Mode())
						}
						log.Printf("delete %s\n", fileDirDown)
						// os.Remove(fileDirDown)

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
				break
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}

	}
	// for include, included := range parent {
	// 	fmt.Printf("include %s, included by %s\n", include, included)
	// }
	fmt.Println(time.Since(t0))
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

func parseIncludes(fileDir string, visited, stack map[string]bool, parent map[string]string) error {
	if stack[fileDir] {
		return fmt.Errorf("include loop detected at %s", fileDir)
	}

	if visited[fileDir] {
		return nil
	}

	stack[fileDir] = true

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

		include := filepath.Join(dir, m[1])

		if stack[include] {
			prev := parent[include]
			return fmt.Errorf("include loop detected %s included by %s already included by %s", include, fileDir, prev)
		}

		parent[include] = fileDir

		err := parseIncludes(include, visited, stack, parent)
		if err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	delete(stack, fileDir)
	visited[fileDir] = true

	return nil
}
