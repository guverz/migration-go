package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFindLastMigrationInfo(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) (string, fs.FS)
		wantMax      int
		wantLastFile string
		wantErr      bool
	}{
		{
			name: "normal",
			setup: func(t *testing.T) (string, fs.FS) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to deleted temp dir: %v", err)
					}
				})
				base := "test-func-0.1.0-1"
				for i := 1; i < 12; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
					if err := os.WriteFile(fileUp, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
					if err := os.WriteFile(fileDown, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
				}
				fsys := os.DirFS(tmpDir)
				return base, fsys
			},
			wantMax:      11,
			wantLastFile: "test-func-0.1.0-1-11.up.sql",
			wantErr:      false,
		},
		{
			name: "wrong base",
			setup: func(t *testing.T) (string, fs.FS) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to deleted temp dir: %v", err)
					}
				})
				base := "test-func-0.1.0-1"
				wrongBase := "wrongTest-func-0.1.0-1"
				for i := 1; i < 6; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
					if err := os.WriteFile(fileUp, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
					if err := os.WriteFile(fileDown, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
				}
				for i := 6; i < 13; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", wrongBase, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", wrongBase, i))
					if err := os.WriteFile(fileUp, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
					if err := os.WriteFile(fileDown, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
				}
				fsys := os.DirFS(tmpDir)
				return base, fsys
			},
			wantMax:      5,
			wantLastFile: "test-func-0.1.0-1-5.up.sql",
			wantErr:      false,
		},
		{
			name: "wrong base",
			setup: func(t *testing.T) (string, fs.FS) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to deleted temp dir: %v", err)
					}
				})
				base := "test-func-0.1.0-1"
				wrongBase := "wrongTest-func-0.1.0-1"
				for i := 1; i < 6; i++ {
					fileUp := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.up.sql", base, i))
					fileDown := filepath.Join(tmpDir, fmt.Sprintf("%s-%v.down.sql", base, i))
					if err := os.WriteFile(fileUp, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
					if err := os.WriteFile(fileDown, []byte(""), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
				}
				fsys := os.DirFS(tmpDir)
				return wrongBase, fsys
			},
			wantMax:      0,
			wantLastFile: "",
			wantErr:      false,
		},
		{
			name: "empty Dir",
			setup: func(t *testing.T) (string, fs.FS) {
				tmpDir, err := os.MkdirTemp("", "test_find_last_migration_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to deleted temp dir: %v", err)
					}
				})
				fsys := os.DirFS(tmpDir)
				base := "test-func-0.1.0-1"
				return base, fsys
			},
			wantMax:      0,
			wantLastFile: "",
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseName, fsys := tt.setup(t)

			foundMax, foundLast, err := findLastMigrationInfo(fsys, ".", baseName)

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

func TestCreateMigrationFiles(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) (string, string, bool)
		wantExists bool
		wantErr    bool
	}{
		{
			name: "normal",
			setup: func(t *testing.T) (string, string, bool) {
				tmpDir, err := os.MkdirTemp("", "test_create_migration_files_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to deleted temp dir: %v", err)
					}
				})
				base := "test-func-0.1.0-1"
				includeHelp := false
				return base, tmpDir, includeHelp
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name: "no dir",
			setup: func(t *testing.T) (string, string, bool) {
				tmpDir := "noDir"
				base := "test-func-0.1.0-1"
				includeHelp := false
				return base, tmpDir, includeHelp
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name: "no IncludeHelp",
			setup: func(t *testing.T) (string, string, bool) {
				tmpDir, err := os.MkdirTemp("", "test_create_migration_files_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to deleted temp dir: %v", err)
					}
				})
				base := "test-func-0.1.0-1"
				includeHelp := true
				return base, tmpDir, includeHelp
			},
			wantExists: false,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseName, tmpDir, help := tt.setup(t)
			t.Cleanup(func() {
				if err := os.RemoveAll(tmpDir); err != nil {
					t.Fatalf("failed to deleted temp dir: %v", err)
				}
			})
			err := createMigrationFiles(tmpDir, baseName, help)
			if (err != nil) != tt.wantErr {
				t.Errorf("createMigrationFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			upFile := filepath.Join(tmpDir, fmt.Sprintf("%s.up.sql", baseName))
			downFile := filepath.Join(tmpDir, fmt.Sprintf("%s.down.sql", baseName))

			rsltUp, err := FindFileViaDir(upFile)
			if err != nil {
				t.Fatalf("error FindFileViaDir: %v", err)
			}
			rsltDown, err := FindFileViaDir(downFile)
			if err != nil {
				t.Fatalf("error FindFileViaDir: %v", err)
			}
			var result bool
			if rsltUp && rsltDown {
				result = true
			} else if !rsltUp && !rsltDown {
				result = false
			}

			if result != tt.wantExists {
				t.Errorf("createMigrationFiles() found newly created files = %v, want %v", result, tt.wantExists)
			}
		})
	}
}

func TestAdd(t *testing.T) {

}

func TestMiniHelp(t *testing.T) {

}
