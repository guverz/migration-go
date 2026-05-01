package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestFindFileViaDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_find_file_*")
	if err != nil {
		t.Errorf("Failed to created dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name      string
		setup     func(string) string
		wantFound bool
		wantErr   bool
	}{
		{
			name: "existing directory",
			setup: func(tmpDir string) string {
				dir := filepath.Join(tmpDir, "existing")
				os.Mkdir(dir, 0755)
				return dir
			},
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "existing file",
			setup: func(tmpDir string) string {
				file := filepath.Join(tmpDir, "test.txt")
				os.WriteFile(file, []byte("test"), 0644)
				return file
			},
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "non-existent path",
			setup: func(tmpDir string) string {
				return filepath.Join(tmpDir, "does_not_exist")
			},
			wantFound: false,
			wantErr:   false,
		},
		{
			name: "empty string",
			setup: func(tmpDir string) string {
				return ""
			},
			wantFound: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPath := tt.setup(tmpDir)

			found, err := FindFileViaDir(testPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("FindFileViaDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if found != tt.wantFound {
				t.Errorf("FindFileViaDir() found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestFileMD5(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
	if err != nil {
		t.Errorf("Failed to created dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		setup   func(string) string
		wantMD5 string
		wantErr bool
	}{
		{
			name: "lorem impsum 100",
			setup: func(tmpDir string) string {
				text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Curabitur felis dolor, fringilla id vulputate eget, volutpat eleifend nisi. In hac habitasse platea dictumst. Maecenas sit amet felis eleifend, blandit nunc et, venenatis turpis. Etiam scelerisque nec arcu ac euismod. Proin maximus est in velit mollis mattis. Ut risus tortor, porttitor eget gravida a, consectetur non nisi. Proin volutpat congue convallis. Sed consectetur fermentum pulvinar. Pellentesque rutrum rutrum maximus. Quisque rhoncus, justo ac gravida auctor, turpis ex pharetra augue, faucibus dignissim dolor leo ut turpis. Maecenas et sem vitae nunc molestie sagittis. Aliquam non tincidunt felis. Nam quis ornare arcu. Maecenas."
				file := filepath.Join(tmpDir, "test.txt")
				os.WriteFile(file, []byte(text), 0644)
				return file
			},
			wantMD5: "fecdaaa968e07e70c5e2cdae6e03a836",
			wantErr: false,
		},
		{
			name: "empty file",
			setup: func(tmpDir string) string {
				file := filepath.Join(tmpDir, "test.txt")
				os.WriteFile(file, []byte(""), 0644)
				return file
			},
			wantMD5: "d41d8cd98f00b204e9800998ecf8427e",
			wantErr: false,
		},
		{
			name: "no file",
			setup: func(tmpDir string) string {
				file := filepath.Join(tmpDir, "none")
				return file
			},
			wantMD5: "",
			wantErr: true,
		},
		{
			name: "dir",
			setup: func(tmpDir string) string {
				dir := filepath.Join(tmpDir, "dir")
				os.Mkdir(dir, 0755)
				return dir
			},
			wantMD5: "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPath := tt.setup(tmpDir)

			resultMD5, err := FileMD5(testPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("FileMD5() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if resultMD5 != tt.wantMD5 {
				t.Errorf("FileMD5() resultMD5 = %v, want %v", resultMD5, tt.wantMD5)
			}
		})
	}
}

func TestStripDir(t *testing.T) {
	tests := []struct {
		name       string
		tempDir    string
		wantResult string
	}{
		{
			name:       "empty string",
			tempDir:    "",
			wantResult: "",
		},
		{
			name:       "path with ./ prefix",
			tempDir:    "./test/foo/bar",
			wantResult: "foo" + string(filepath.Separator) + "bar",
		},
		{
			name:       "path with .\\",
			tempDir:    ".\\test\\foo\\bar",
			wantResult: "foo" + string(filepath.Separator) + "bar",
		},
		{
			name:       "clean path",
			tempDir:    "test/foo/bar",
			wantResult: "foo" + string(filepath.Separator) + "bar",
		},
		{
			name:       "not a path",
			tempDir:    "testfoobar",
			wantResult: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripDir(tt.tempDir)

			if result != tt.wantResult {
				t.Errorf("StripDir() result = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestGetEntriesProjectMap(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) (string, fs.FS)
		wantEntryLen int
		wantErr      bool
	}{
		{
			name: "normal",
			setup: func(t *testing.T) (string, fs.FS) {
				projectDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(projectDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				testChdirRepo(t, projectDir)
				fsys := os.DirFS(".")
				migrationDir := filepath.Join(projectDir, "migrations")
				if err := os.Mkdir(migrationDir, 0755); err != nil {
					t.Fatalf("failed to create migration directory: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
						projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
						projectUpPath := filepath.Join(migrationDir, projectUpName)
						projectDownPath := filepath.Join(migrationDir, projectDownName)
						if err := os.WriteFile(projectUpPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
						if err := os.WriteFile(projectDownPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
					}
				}
				return "migrations", fsys
			},
			wantEntryLen: 24,
			wantErr:      false,
		},
		{
			name: "wrong format",
			setup: func(t *testing.T) (string, fs.FS) {
				projectDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(projectDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				testChdirRepo(t, projectDir)
				fsys := os.DirFS(".")
				migrationDir := filepath.Join(projectDir, "migrations")
				if err := os.Mkdir(migrationDir, 0755); err != nil {
					t.Fatalf("failed to create migration directory: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectUpName := fmt.Sprintf("%s-%v-%v.left.sql", baseProjectName, i, j)
						projectDownName := fmt.Sprintf("%s-%v-%v.right.sql", baseProjectName, i, j)
						projectUpPath := filepath.Join(migrationDir, projectUpName)
						projectDownPath := filepath.Join(migrationDir, projectDownName)
						if err := os.WriteFile(projectUpPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
						if err := os.WriteFile(projectDownPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
					}
				}
				return "migrations", fsys
			},
			wantEntryLen: 0,
			wantErr:      false,
		},
		{
			name: "fsys check",
			setup: func(t *testing.T) (string, fs.FS) {
				fsys := fstest.MapFS{
					"proj1.txt":  {Data: []byte{}},
					"proj2.txt":  {Data: []byte{}},
					"ignore.log": {Data: []byte{}},
				}

				return ".", fsys
			},
			wantEntryLen: 0,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, fsys := tt.setup(t)
			changedDir := filepath.ToSlash(dir)
			resultMap, err := getEntriesProjectMap(fsys, changedDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("getEntriesProjectMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantEntryLen {
				t.Errorf("getEntriesProjectMap() found = %v, want %v", len(resultMap), tt.wantEntryLen)
			}
		})
	}
}
