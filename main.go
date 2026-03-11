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

var (
	unvisited = 0
	visiting  = 1
	done      = 2
)

type ProjectMigrations struct {
	Prefix       string
	MD5          string
	Ext          string
	Dir          string
	UpFileName   string
	DownFileName string
}

type ModuleMigrations struct {
	Prefix       string
	Ext          string
	Dir          string
	UpFileName   string
	DownFileName string
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
	// deletedIncludes := make(map[string]string)
	migrationIncludes := make(map[string]string)
	originalIncludes := make(map[string]string)
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
		state := make(map[string]int)

		matches := ListPattern.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		filePrefix, fileExt := matches[1], matches[2]
		downFileName := fmt.Sprintf("%s.down.%s", filePrefix, fileExt)
		// upFileName := entry.Name()

		if _, exists := fileMap[downFileName]; !exists {
			// probably should return fmt.Errorf instead of log & continue (?)
			log.Printf("file %s do not have counterpart file %s at '%s'", entry.Name(), downFileName, dir)
			continue
		}

		fileDirUp := filepath.Join(dir, entry.Name())
		fileDirDown := filepath.Join(dir, entry.Name())

		if err := parseIncludes(fileDirUp, state, migrationIncludes); err != nil {
			return fmt.Errorf("parseIncludes error: %w", err)
		}

		if err := parseIncludes(fileDirDown, state, migrationIncludes); err != nil {
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
							// os.Remove(fileDirUp)
						}

						// WARNING dangerous operation, check fileDirDown is not zero and this is a regular file
						if fileInfo, err := os.Stat(fileDirDown); err == nil && fileInfo.Mode().IsRegular() {
							log.Printf("delete %s", fileDirDown)
							// os.Remove(fileDirDown)
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

				// temp := ProjectMigrations{
				// 	Prefix: prefix,
				// 	MD5: md5,
				// 	Ext: ext,
				// 	Dir: metaDir,
				// 	UpFileName: metaUpName,
				// 	DownFileName: metaDownName,
				// }
				// temp2 := ModuleMigrations{
				// 	Prefix: filePrefix,
				// 	Ext: fileExt,
				// 	Dir: dir,
				// 	UpFileName: upFileName,
				// 	DownFileName: downFileName,
				// }
				if err := parseIncludes(metaFileDirDown, state, originalIncludes); err != nil {
					return fmt.Errorf("parseIncludes error: %w", err)
				}
				if err := parseIncludes(metaFileDirUp, state, originalIncludes); err != nil {
					return fmt.Errorf("parseIncludes error: %w", err)
				}
				// migrationMD5Includes := make(map[[16]byte]string)
				// projectMD5Includes := make(map[[16]byte]string)

				// for include, included := range migrationIncludes {
				// 	md5Include := md5.Sum([]byte(include))
				// 	migrationMD5Includes[md5Include] = include
				// 	projectMD5Includes[md5Include] = include
				// 	metaInclude := filepath.Join(metaDir, include)
				// 	// log.Printf("md5 %x of include file %s included by %s and check in original includes at %s", md5Include, include, included, metaDir)
				// 	if rslt, err := findFileViaDir(metaInclude); err != nil {
				// 		return fmt.Errorf("findFileViaDir error: %w", err)
				// 	} else if !rslt {
				// 		log.Printf("include %s may be deleted from %s, check later", include, metaInclude)
				// 		deletedIncludes[include] = included
				// 		deletedIncludesCnt++
				// 	}
				// }

				// for include, parent := range originalIncludes {
				// 	md5Parent := md5.Sum([]byte(parent))
				// 	tempSlice := strings.Split(parent, "/")
				// 	includedRelativeFile := tempSlice[len(tempSlice)-1]
				// }
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}

	}
	for include, included := range migrationIncludes {
		fmt.Printf("include %s, included by %s\n", include, included)
	}
	log.Printf("migrationINcludes ends, originalIncludes starts\n")
	for include, included := range originalIncludes {
		fmt.Printf("include %s, included by %s\n", include, included)
	}
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

func parseIncludes(fileDir string, state map[string]int, parent map[string]string) error {

	switch state[fileDir] {

	case visiting:
		prev := parent[fileDir]
		return fmt.Errorf("include loop detected %s included by %s already included by %s", fileDir, parent[fileDir], prev)

	case done:
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

		if state[includeDir] == visiting {
			prev := parent[includeName]
			return fmt.Errorf("include loop detected %s included by %s already included by %s", includeName, fileDir, prev)
		}

		parent[includeName] = fileDir

		if err := parseIncludes(includeDir, state, parent); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	state[fileDir] = done
	return nil
}
