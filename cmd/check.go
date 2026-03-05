package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

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
	if err := MigrationList(MigrationDir); err != nil {
		log.Printf("error migrationList: %s", err)
	}
	// if err := parseIncludeDir(MigrationDir); err != nil {
	// 	log.Printf("error: %s", err)
	// }
	// fullFileMD5, err := tempFindOriginalMigrations(MigrationDir)
	// if err != nil {
	// 	fmt.Println("error")
	// }
	// DirBase := make(map[string]string)
	// for el := range fullFileMD5 {
	// 	DirBase[filepath.Base(el)] = filepath.Join(TestDir, filepath.Dir(el))
	// }

	// for name, dir := range DirBase {
	// 	rslt, err := findFileViaPath(dir, name)
	// 	if err != nil {
	// 		log.Println(err)
	// 	}
	// 	if rslt {
	// 		fmt.Printf("down-file %s found in %s\n", name, dir)
	// 	} else {
	// 		fmt.Printf("down-file %s not found in %s\n", name, dir)
	// 	}
	// }

}

func MigrationList(dir string) error {
	// missedIncludesCnt := 0
	// deletedIncludesCnt := 0
	t0 := time.Now()
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

				include, err := parseIncludeFile(simPath, upName)
				if err != nil {
					return fmt.Errorf("parseIncludeFile error: %w", err)
				}
				if include != "" {
					log.Printf("Found include: '%s' in file '%s' via path: '%s'", include, upName, simPath)
				}

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
	fmt.Println(time.Since(t0))
	return nil
}

func parseIncludeFile(dir, base string) (string, error) {
	pattern := regexp.MustCompile(`^@([^;]+)`)

	filePath := filepath.Join(dir, base)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("File doesn't exist: %s", filePath)
		}
		return "", fmt.Errorf("can't get file info: %w", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("Warning: cannot open file %s: %w", filePath, err)
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
		// it can return multiple matches, maybe this fuction should have a pointer to slice in order to append matches to it
		// log.Printf("Found '%s' in file '%s'", matches[0], entry.Name())
		return matches[0], nil
	}

	return "", nil
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
