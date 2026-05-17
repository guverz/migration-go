package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

func Add() error {
	if _, err := addMigrationPair(IncludeHelp, realVersionGetter{}); err != nil {
		return fmt.Errorf("error adding migration pair: %w", err)
	}
	return nil
}

func findLastMigrationInfo(fsys fs.FS, dir string, baseName string) (int, string, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`^%s-(\d+)\.up\.sql$`, regexp.QuoteMeta(baseName)))
	var (
		maxNum   int
		lastFile string
	)

	// io/fs paths must satisfy fs.ValidPath (no "."/".." segments); match migrationList normalization.
	dir = filepath.ToSlash(filepath.Clean(dir))
	entries, err := fs.ReadDir(fsys, dir)
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

func createMigrationFiles(dir string, baseName string, includeHelp bool) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating dir: %w", err)
	}

	upContent := fmt.Sprintf("# %s.up.sql\n", baseName)
	getMiniHelp := ""
	if includeHelp {
		var err error
		getMiniHelp, err = minihelp()
		if err != nil {
			return fmt.Errorf("error getting minihelp: %w", err)
		}
	}

	if includeHelp {
		upContent += getMiniHelp + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, baseName+".up.sql"), []byte(upContent), 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	downContent := fmt.Sprintf("# %s.down.sql\n", baseName)
	if includeHelp {
		downContent += getMiniHelp + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, baseName+".down.sql"), []byte(downContent), 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	return nil
}

func addMigrationPair(includeFlag bool, getter versionGetter) (string, error) {
	project, err := describe(MigrationDir, "project", getter)
	if err != nil {
		return "", fmt.Errorf("error describing dir: %w", err)
	}
	version, err := describe(MigrationDir, "version", getter)
	if err != nil {
		return "", fmt.Errorf("error describing dir: %w", err)
	}
	release, err := describe(MigrationDir, "release", getter)
	if err != nil {
		return "", fmt.Errorf("error describing dir: %w", err)
	}

	baseName := fmt.Sprintf("%s-%s-%s", project, version, release)

	fmt.Printf("Add migration script %s\n", baseName)

	fsys := os.DirFS(".")

	increment, _, err := findLastMigrationInfo(fsys, MigrationDir, baseName)
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

func minihelp() (string, error) {
	text, err := os.ReadFile(MiniHelpDir)
	if err != nil {
		return "", fmt.Errorf("error reading MiniHelp: %v", err)
	}
	return string(text), nil
}
