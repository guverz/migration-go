package migration

import (
	"bufio"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	versiongo "github.com/AlexBurnes/version-go/pkg/version"
)

// GetModuleDir function
func GetModuleDir(fsys fs.FS, path string) ([]string, error) {
	rslt := []string{}
	outerDir := filepath.Dir(path)
	submoduleDir := filepath.Join(outerDir, ".gitmodules")
	submoduleDir = filepath.ToSlash(submoduleDir)
	f, err := fsys.Open(submoduleDir)
	if err != nil {
		return nil, fmt.Errorf("error opening .gitmodules: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "path ="); ok {
			path := strings.TrimSpace(after)
			rslt = append(rslt, filepath.Join(outerDir, path))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return rslt, nil
}

// Describe function
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
