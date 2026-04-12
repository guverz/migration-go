package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendToFrom(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) (string, string, string)
		wantExists bool
		wantHeader string
		wantErr    bool
	}{
		{
			name: "normal",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
				includeDirPath := filepath.Join(tmpDir, "includes")
				os.Mkdir(includeDirPath, 0755)
				includedFilePath := filepath.Join(tmpDir, "included.txt")
				baseIncludeName := "include"
				os.WriteFile(includedFilePath, []byte(fmt.Sprintf("@includes/%s_0.txt", baseIncludeName)), 0644)
				for i := 0; i < 5; i++ {
					includePath := filepath.Join(includeDirPath, fmt.Sprintf("%s_%v.txt", baseIncludeName, i))
					if i != 4 {
						os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i+1)), 0644)
					} else {
						os.WriteFile(includePath, []byte(""), 0644)
					}
				}
				return includedFilePath, "", ""
			},
			wantExists: true,
			wantHeader: "1234567890abcABC",
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetFile, srcFile, header := tt.setup(t)

			err := appendToFrom(targetFile, srcFile, header)
			if (err != nil) != tt.wantErr {
				t.Errorf("appendToFrom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fileHeader, _ := os.ReadFile(targetFile)
			var existsFlag bool
			_, tempErr := os.Stat(targetFile)
			if tempErr != nil {
				existsFlag = true
			}
			if os.IsNotExist(tempErr) {
				existsFlag = false
			}

			if existsFlag != tt.wantExists {
				t.Errorf("appendToFrom() found = %v, want %v", existsFlag, tt.wantExists)
			}
			if string(fileHeader) != tt.wantHeader {
				t.Errorf("appendToFrom() found = %v, want %v", string(fileHeader), tt.wantHeader)
			}
			if string(fileHeader) != tt.wantHeader {
				t.Errorf("appendToFrom() found = %v, want %v", string(fileHeader), tt.wantHeader)
			}
		})
	}
}
