package migration

import (
	"crypto/md5"
	"encoding/hex"
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
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

func StripDir(fileDir string) string {
	cleanDir := filepath.Clean(fileDir)
	dirSplit := strings.Split(cleanDir, string(filepath.Separator))
	return strings.Join(dirSplit[1:], string(filepath.Separator))
}
