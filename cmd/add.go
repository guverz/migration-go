package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	versiongo "github.com/AlexBurnes/version-go/pkg/version"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "temp",
	Long:  `temp`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := Add(IncludeHelp)
		return err
	},
}

func FindLastMigrationInfo(dir, baseName string) (int, string, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`^%s-(\d+)\.up\.sql$`, regexp.QuoteMeta(baseName)))
	var maxNum int
	var lastFile string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, "", fmt.Errorf("failed to read directory %s: %v", dir, err)
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
				lastFile = entry.Name()
			}
		}
	}

	return maxNum, lastFile, nil
}

func createMigrationFiles(dir, baseName string, includeHelp bool) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating dir: %w", err)
	}

	upContent := fmt.Sprintf("# %s.up.sql\n", baseName)
	GetMiniHelp, _ := minihelp()
	if includeHelp {
		upContent += GetMiniHelp + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, baseName+".up.sql"), []byte(upContent), 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	downContent := fmt.Sprintf("# %s.down.sql\n", baseName)
	if includeHelp {
		downContent += GetMiniHelp + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, baseName+".down.sql"), []byte(downContent), 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	return nil
}

func Add(includeFlag bool) (string, error) {
	project, err := Describe(MigrationDir, "project")
	if err != nil {
		return "", fmt.Errorf("error describing dir: %w", err)
	}
	version, err := Describe(MigrationDir, "version")
	if err != nil {
		return "", fmt.Errorf("error describing dir: %w", err)
	}
	release, err := Describe(MigrationDir, "release")
	if err != nil {
		return "", fmt.Errorf("error describing dir: %w", err)
	}

	baseName := fmt.Sprintf("%s-%s-%s", project, version, release)

	fmt.Printf("Add migration script %s\n", baseName)

	increment, _, err := FindLastMigrationInfo(MigrationDir, baseName)
	if err != nil {
		return "", fmt.Errorf("failed to find last migration: %v", err)
	}
	increment++

	migrationFile := fmt.Sprintf("%s-%d", baseName, increment)
	err = createMigrationFiles(MigrationDir, migrationFile, includeFlag)
	if err != nil {
		return "", fmt.Errorf("failed to create migration files: %v", err)
	}

	fmt.Printf("Created migration files:\n   %s/%s.up.sql\n   %s/%s.down.sql\n",
		MigrationDir, migrationFile, MigrationDir, migrationFile)

	return migrationFile, nil
}

func Describe(dir, arg string) (rslt string, err error) {
	switch arg {
	case "project":
		rslt, err = versiongo.GetProjectFromGit(dir)
	case "version":
		rslt, err = versiongo.GetVersion(dir)
	case "release":
		rslt = versiongo.GetRelease()
	case "full":
		rslt, err = versiongo.GetFull(dir)
	default:
		return "", fmt.Errorf("unknown argument")
	}
	if err != nil {
		return "", err
	}
	return rslt, nil
}

func minihelp() (string, error) {
	text, err := os.ReadFile(MiniHelpDir)
	if err != nil {
		return "", fmt.Errorf("Error reading MiniHelp: %v", err)
	}
	return string(text), nil
}
