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

// still a poor function but kinda made it more versatile
func GetProject(repoPath, dir string) (string, error) {
	configPath := filepath.Join(repoPath, ".git", "config")

	f, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("error opening file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	inOrigin := false
	var url string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}

		if strings.HasPrefix(line, "[") {
			inOrigin = false
		}

		if inOrigin && strings.HasPrefix(line, "url") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				url = strings.TrimSpace(parts[1])
				break
			}
		}
	}

	if url == "" {
		submoduleName := filepath.Base(repoPath)
		baseFull, err := Describe(dir, "project")
		if err != nil {
			return "", fmt.Errorf("error describing dir: %w", err)
		}
		baseCut, _, _ := strings.Cut(baseFull, "-")
		baseSubmoduleName := fmt.Sprintf("%s-%s", baseCut, submoduleName)
		return baseSubmoduleName, nil
	}

	if i := strings.Index(url, ":"); i != -1 {
		url = url[i+1:]
	}

	url = strings.ReplaceAll(url, "/", "-")
	url = strings.TrimSuffix(url, ".git")

	return url, nil
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
