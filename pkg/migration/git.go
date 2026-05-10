package migration

import (
	"bufio"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	versiongo "github.com/AlexBurnes/version-go/pkg/version"
)

// getModuleDir function
func getModuleDir(fsys fs.FS, path string) ([]string, error) {
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

type versionGetter interface {
	GetProjectFromGit(dir string) (string, error)
	GetVersion(dir string) (string, error)
	GetRelease() string
	GetFull(dir string) (string, error)
}

type realVersionGetter struct{}

func (r realVersionGetter) GetProjectFromGit(dir string) (string, error) {
	return versiongo.GetProjectFromGit(dir)
}

func (r realVersionGetter) GetVersion(dir string) (string, error) {
	return versiongo.GetVersion(dir)
}

func (r realVersionGetter) GetRelease() string {
	return versiongo.GetRelease()
}

func (r realVersionGetter) GetFull(dir string) (string, error) {
	return versiongo.GetFull(dir)
}

// describe function
func describe(dir, arg string, getter versionGetter) (rslt string, err error) {
	switch arg {
	case "project":
		rslt, err = getter.GetProjectFromGit(dir)
	case "version":
		rslt, err = getter.GetVersion(dir)
	case "release":
		rslt = getter.GetRelease()
	case "full":
		rslt, err = getter.GetFull(dir)
	default:
		return "", fmt.Errorf("unknown argument")
	}
	if err != nil {
		return "", err
	}
	return rslt, nil
}
