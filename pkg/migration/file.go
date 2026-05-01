package migration

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func FindFileViaDir(fileDir string) (bool, error) {
	_, err := os.Stat(fileDir)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func FileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	h := md5.New()

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func StripDir(fileDir string) string {
	cleanDir := filepath.Clean(fileDir)
	dirSplit := strings.Split(cleanDir, string(filepath.Separator))
	return strings.Join(dirSplit[1:], string(filepath.Separator))
}

func getEntriesProjectMap(fsys fs.FS, dir string) (map[string]struct{}, error) {
	entriesProjectMap := make(map[string]struct{})
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("error reading dir: %w", err)
	}
	for _, entry := range entries {
		match := MigrationPattern.MatchString(entry.Name())
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
	moduleDirs, err := GetModuleDir(fsys, dir)
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
			match := MigrationPattern.MatchString(entry.Name())
			if !match {
				continue
			}
			modulePath := filepath.Join(moduleMigration, entry.Name())
			entriesModuleMap[modulePath] = struct{}{}
		}
	}

	return entriesModuleMap, nil
}
