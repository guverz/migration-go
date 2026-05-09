package migration

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FindFileViaDir function
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

// FileMD5 function
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

// StripDir function
func StripDir(fileDir string) string {
	cleanDir := filepath.Clean(fileDir)
	dirSplit := strings.Split(cleanDir, string(filepath.Separator))
	return strings.Join(dirSplit[1:], string(filepath.Separator))
}
