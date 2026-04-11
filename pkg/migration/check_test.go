package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseIncludes(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(t *testing.T) string
		wantMissingFiles int
		wantIncludes     int
		wantErr          bool
	}{
		{
			name: "normal",
			setup: func(t *testing.T) string {
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
				return includedFilePath
			},
			wantMissingFiles: 0,
			wantIncludes:     5,
			wantErr:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			includedFile := tt.setup(t)
			ctx := NewParseContext()
			err := ParseIncludes(ctx, includedFile, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInclude() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(ctx.MissingFiles) != tt.wantMissingFiles {
				t.Errorf("ParseInclude() found = %v, want %v", len(ctx.MissingFiles), tt.wantMissingFiles)
			}
			if len(ctx.Includes) != tt.wantIncludes {
				t.Errorf("ParseInclude() found = %v, want %v", len(ctx.Includes), tt.wantIncludes)
			}
		})
	}
}
