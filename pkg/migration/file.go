package migration

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// findFileViaDir function
func findFileViaDir(fileDir string) (bool, error) {
	_, err := os.Stat(fileDir)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// fileMD5 function
func fileMD5(path string) (string, error) {
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

// stripDir function
func stripDir(fileDir string) string {
	cleanDir := filepath.Clean(fileDir)
	dirSplit := strings.Split(cleanDir, string(filepath.Separator))
	return strings.Join(dirSplit[1:], string(filepath.Separator))
}

// concatMD5 is used to both calculate and concatenate migration pair.
func concatMD5(upPath, downPath string) (string, error) {
	md5Up, err := fileMD5(upPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("FileMD5 error: %w", err)
	}
	md5Down, err := fileMD5(downPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("FileMD5 error: %w", err)
	}

	return md5Up + md5Down, nil
}
