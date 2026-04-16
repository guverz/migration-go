package migration

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	versiongo "github.com/AlexBurnes/version-go/pkg/version"
)

func GetModuleDir(path string) ([]string, error) {
	rslt := []string{}
	outerDir := filepath.Dir(path)
	submoduleDir := filepath.Join(outerDir, ".gitmodules")
	f, err := os.Open(submoduleDir)
	if err != nil {
		return nil, fmt.Errorf("error opening .gitmodules: %w", err)
	}
	defer f.Close()

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
