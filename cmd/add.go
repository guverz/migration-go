package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "temp",
	Long:  `temp`,
	Run: func(cmd *cobra.Command, args []string) {
		add()
	},
}

func findLastMigrationNumber(dir, baseName string) (int, error) {
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

func createMigrationFiles(dir, baseName string, includeHelp bool) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

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

func add() error {
	project, err := Describe(MigrationDir, "project")
	if err != nil {
		log.Fatal(err)
	}
	version, err := Describe(MigrationDir, "version")
	if err != nil {
		log.Fatal(err)
	}
	release, err := Describe(MigrationDir, "release")
	if err != nil {
		log.Fatal(err)
	}

	baseName := fmt.Sprintf("%s-%s-%s", project, version, release)

	fmt.Printf("Add migration script %s\n", baseName)

	increment, err := findLastMigrationNumber(MigrationDir, baseName)
	if err != nil {
		return fmt.Errorf("failed to find last migration: %v", err)
	}
	increment++

	migrationFile := fmt.Sprintf("%s-%d", baseName, increment)
	err = createMigrationFiles(MigrationDir, migrationFile, IncludeHelp)
	if err != nil {
		return fmt.Errorf("failed to create migration files: %v", err)
	}

	fmt.Printf("Created migration files:\n   %s/%s.up.sql\n   %s/%s.down.sql\n",
		MigrationDir, migrationFile, MigrationDir, migrationFile)

	return nil
}

func Describe(dir, arg string) (string, error) {
	cmd := exec.Command("version", arg)
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run version %v in %v: %w", dir, arg, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func minihelp() (string, error) {
	text, err := os.ReadFile(MiniHelpDir)
	if err != nil {
		return "", fmt.Errorf("Error reading MiniHelp: %v", err)
	}
	return string(text), nil
}
