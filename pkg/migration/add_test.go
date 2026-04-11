package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFindLastMigrationInfo(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) (string, string)
		wantMax      int
		wantLastFile string
		wantErr      bool
	}{
		{
			name: "normal",
			setup: func(t *testing.T) (string, string) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
				base := "test-func-0.1.0-1"
				for i := 1; i < 12; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
					os.WriteFile(fileUp, []byte(""), 0644)
					os.WriteFile(fileDown, []byte(""), 0644)
				}
				return base, tmpDir
			},
			wantMax:      11,
			wantLastFile: "test-func-0.1.0-1-11.up.sql",
			wantErr:      false,
		},
		{
			name: "wrong base",
			setup: func(t *testing.T) (string, string) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
				base := "test-func-0.1.0-1"
				wrongBase := "wrongTest-func-0.1.0-1"
				for i := 1; i < 6; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
					os.WriteFile(fileUp, []byte(""), 0644)
					os.WriteFile(fileDown, []byte(""), 0644)
				}
				for i := 6; i < 13; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", wrongBase, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", wrongBase, i))
					os.WriteFile(fileUp, []byte(""), 0644)
					os.WriteFile(fileDown, []byte(""), 0644)
				}
				return base, tmpDir
			},
			wantMax:      5,
			wantLastFile: "test-func-0.1.0-1-5.up.sql",
			wantErr:      false,
		},
		{
			name: "wrong base",
			setup: func(t *testing.T) (string, string) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
				base := "test-func-0.1.0-1"
				wrongBase := "wrongTest-func-0.1.0-1"
				for i := 1; i < 6; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
					os.WriteFile(fileUp, []byte(""), 0644)
					os.WriteFile(fileDown, []byte(""), 0644)
				}
				return wrongBase, tmpDir
			},
			wantMax:      0,
			wantLastFile: "",
			wantErr:      false,
		},
		{
			name: "empty Dir",
			setup: func(t *testing.T) (string, string) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(tmpDir)
				})
				base := "test-func-0.1.0-1"
				return base, tmpDir
			},
			wantMax:      0,
			wantLastFile: "",
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseName, tmpDir := tt.setup(t)

			foundMax, foundLast, err := findLastMigrationInfo(tmpDir, baseName)

			if (err != nil) != tt.wantErr {
				t.Errorf("findLastMigrationInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if foundMax != tt.wantMax {
				t.Errorf("findLastMigrationInfo() found = %v, want %v", foundMax, tt.wantMax)
			}
			if foundLast != tt.wantLastFile {
				t.Errorf("findLastMigrationInfo() found = %v, want %v", foundLast, tt.wantLastFile)
			}
		})
	}
}

// func TestCreateMigrationFiles(t *testing.T) {
// 	tests := []struct {
// 		name         string
// 		setup        func(t *testing.T) (string, string)
// 		wantMax      int
// 		wantLastFile string
// 		wantErr      bool
// 	}{
// 		{
// 			name: "normal",
// 			setup: func(t *testing.T) (string, string) {
// 				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
// 				if err != nil {
// 					t.Fatalf("Failed to create dir: %v", err)
// 				}
// 				t.Cleanup(func() {
// 					os.RemoveAll(tmpDir)
// 				})
// 				base := "test-func-0.1.0-1"
// 				for i := 1; i < 12; i++ {
// 					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
// 					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
// 					os.WriteFile(fileUp, []byte(""), 0644)
// 					os.WriteFile(fileDown, []byte(""), 0644)
// 				}
// 				return base, tmpDir
// 			},
// 			wantMax:      11,
// 			wantLastFile: "test-func-0.1.0-1-11.up.sql",
// 			wantErr:      false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			baseName, tmpDir := tt.setup(t)

// 			foundMax, foundLast, err := findLastMigrationInfo(tmpDir, baseName)

// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("findLastMigrationInfo() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}

// 			if foundMax != tt.wantMax {
// 				t.Errorf("findLastMigrationInfo() found = %v, want %v", foundMax, tt.wantMax)
// 			}
// 			if foundLast != tt.wantLastFile {
// 				t.Errorf("findLastMigrationInfo() found = %v, want %v", foundLast, tt.wantLastFile)
// 			}
// 		})
// 	}
// }
