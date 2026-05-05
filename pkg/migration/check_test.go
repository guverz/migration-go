package migration

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// testMigrationStripPrefix is a synthetic top-level directory name so StripDir(...) in #migration
// matches production: first path segment is discarded (like a repo root folder), remainder is
// module/migrations/... . Using a real MkDirTemp absolute path breaks StripDir on Windows (first
// segment becomes "C:") and on Unix (first segment "").
const testMigrationStripPrefix = "repo"

// testChdirRepo switches cwd to repo root so MigrationList can be called with a relative dir
// (e.g. "migrations") like ./test/migrations in production. Cleanup restores the previous wd
// before the temp dir is removed.
func testChdirRepo(t *testing.T, repoRoot string) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("chdir error: %v", err)
		}
	})
}

// testMetaPathForModuleFile returns the path string to embed in #migration:... after the same
// StripDir logic as the CLI, without relying on absolute temp paths.
func testMetaPathForModuleFile(moduleSQLFileName string) string {
	return StripDir(filepath.Join(testMigrationStripPrefix, "module", "migrations", moduleSQLFileName))
}

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
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("error removing temp directory: %v", err)
					}
				})
				includeDirPath := filepath.Join(tmpDir, "includes")
				if err := os.Mkdir(includeDirPath, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				includedFilePath := filepath.Join(tmpDir, "included.txt")
				baseIncludeName := "include"
				if err := os.WriteFile(includedFilePath, []byte(fmt.Sprintf("@includes/%s_0.txt", baseIncludeName)), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				for i := 0; i < 5; i++ {
					includePath := filepath.Join(includeDirPath, fmt.Sprintf("%s_%v.txt", baseIncludeName, i))
					if i != 4 {
						if err := os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i+1)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
					} else {
						if err := os.WriteFile(includePath, []byte(""), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
					}
				}
				return includedFilePath
			},
			wantMissingFiles: 0,
			wantIncludes:     5,
			wantErr:          false,
		},
		{
			name: "missing include",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("error removing temp directory: %v", err)
					}
				})
				includeDirPath := filepath.Join(tmpDir, "includes")
				if err := os.Mkdir(includeDirPath, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				includedFilePath := filepath.Join(tmpDir, "included.txt")
				baseIncludeName := "include"
				if err := os.WriteFile(includedFilePath, []byte(fmt.Sprintf("@includes/%s_0.txt", baseIncludeName)), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				for i := 1; i < 6; i++ {
					includePath := filepath.Join(includeDirPath, fmt.Sprintf("%s_%v.txt", baseIncludeName, i))
					if i != 4 {
						if err := os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i+1)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
					} else {
						if err := os.WriteFile(includePath, []byte(""), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
					}
				}
				return includedFilePath
			},
			wantMissingFiles: 1,
			wantIncludes:     0,
			wantErr:          false,
		},
		{
			name: "include loop error",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("error removing temp directory: %v", err)
					}
				})
				includeDirPath := filepath.Join(tmpDir, "includes")
				if err := os.Mkdir(includeDirPath, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				includedFilePath := filepath.Join(tmpDir, "included.txt")
				baseIncludeName := "include"
				if err := os.WriteFile(includedFilePath, []byte(fmt.Sprintf("@includes/%s_0.txt", baseIncludeName)), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				for i := 0; i < 5; i++ {
					includePath := filepath.Join(includeDirPath, fmt.Sprintf("%s_%v.txt", baseIncludeName, i))
					if i != 4 {
						if err := os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i+1)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
					} else {
						if err := os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i-2)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
					}
				}
				return includedFilePath
			},
			wantMissingFiles: 0,
			wantIncludes:     5,
			wantErr:          true,
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

func TestConcatMD5(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
	if err != nil {
		t.Errorf("failed to created dir: %v", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil && err == nil {
			err = removeErr
		}
	}()

	tests := []struct {
		name    string
		setup   func(string) (string, string)
		wantMD5 string
		wantErr bool
	}{
		{
			name: "lorem impsum 100",
			setup: func(tmpDir string) (string, string) {
				text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Curabitur felis dolor, fringilla id vulputate eget, volutpat eleifend nisi. In hac habitasse platea dictumst. Maecenas sit amet felis eleifend, blandit nunc et, venenatis turpis. Etiam scelerisque nec arcu ac euismod. Proin maximus est in velit mollis mattis. Ut risus tortor, porttitor eget gravida a, consectetur non nisi. Proin volutpat congue convallis. Sed consectetur fermentum pulvinar. Pellentesque rutrum rutrum maximus. Quisque rhoncus, justo ac gravida auctor, turpis ex pharetra augue, faucibus dignissim dolor leo ut turpis. Maecenas et sem vitae nunc molestie sagittis. Aliquam non tincidunt felis. Nam quis ornare arcu. Maecenas."
				file1 := filepath.Join(tmpDir, "test1.txt")
				file2 := filepath.Join(tmpDir, "test2.txt")
				if err := os.WriteFile(file1, []byte(text), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				if err := os.WriteFile(file2, []byte(text), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				return file1, file2
			},
			wantMD5: "fecdaaa968e07e70c5e2cdae6e03a836fecdaaa968e07e70c5e2cdae6e03a836",
			wantErr: false,
		},
		{
			name: "empty file",
			setup: func(tmpDir string) (string, string) {
				file1 := filepath.Join(tmpDir, "test1.txt")
				file2 := filepath.Join(tmpDir, "test2.txt")
				if err := os.WriteFile(file1, []byte(""), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				if err := os.WriteFile(file2, []byte(""), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				return file1, file2
			},
			wantMD5: "d41d8cd98f00b204e9800998ecf8427ed41d8cd98f00b204e9800998ecf8427e",
			wantErr: false,
		},
		{
			name: "no file",
			setup: func(tmpDir string) (string, string) {
				file := filepath.Join(tmpDir, "none")
				return file, file
			},
			wantMD5: "",
			wantErr: false,
		},
		{
			name: "dir",
			setup: func(tmpDir string) (string, string) {
				dir := filepath.Join(tmpDir, "dir")
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				return dir, dir
			},
			wantMD5: "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upPath, downPath := tt.setup(tmpDir)

			resultMD5, err := ConcatMD5(upPath, downPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConcatMD5() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if resultMD5 != tt.wantMD5 {
				t.Errorf("ConcatMD5() resultMD5 = %v, want %v", resultMD5, tt.wantMD5)
			}
		})
	}
}

func TestGetMetaInfo(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (fs.FS, string)
		wantDir string
		wantUp  string
		wantMD5 string
		wantErr bool
	}{
		{
			name: "fsys check",
			setup: func(t *testing.T) (fs.FS, string) {
				fsys := fstest.MapFS{
					".gitmodules":           {Data: []byte("[submodule \"module\"]\n\tpath = module\n\turl = ./module")},
					"migrations/proj1.txt":  {Data: []byte{}},
					"migrations/proj2.txt":  {Data: []byte{}},
					"migrations/ignore.log": {Data: []byte{}},
				}

				return fsys, ".gitmodules"
			},
			wantDir: "",
			wantUp:  "",
			wantMD5: "",
			wantErr: false,
		},
		{
			name: "fsys check2",
			setup: func(t *testing.T) (fs.FS, string) {
				fsys := make(fstest.MapFS)
				var builder strings.Builder

				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 4 {
							fmt.Fprintf(&builder, "#migration: module/migrations/test-module-0.0.0.0.1-1-2.up.pdf;402bf15aa947b224e8f5700f62f30d9300f26a78304b90d67c51c3331144ae56\n")
						}
						fmt.Fprintf(&builder, "foo%d-%d\"]\n\tbar = foo%d-%d\n\tbar = foo%d-%d\n", i, j, i, j, i, j)
					}
				}

				metaText := builder.String()
				fsys["metafile.txt"] = &fstest.MapFile{Data: []byte(metaText), Mode: 0644}
				return fsys, "metafile.txt"
			},
			wantDir: filepath.Join("module", "migrations"),
			wantUp:  "test-module-0.0.0.0.1-1-2.up.pdf",
			wantMD5: "402bf15aa947b224e8f5700f62f30d9300f26a78304b90d67c51c3331144ae56",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys, file := tt.setup(t)

			resultStruct, md5, err := getMetaInfo(fsys, file)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMetaInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if resultStruct.Dir != tt.wantDir {
				t.Errorf("getMetaInfo() found = %v, want %v", resultStruct.Dir, tt.wantDir)
			}
			if resultStruct.UpFileName != tt.wantUp {
				t.Errorf("getMetaInfo() found = %v, want %v", resultStruct.UpFileName, tt.wantUp)
			}
			if md5 != tt.wantMD5 {
				t.Errorf("getMetaInfo() found = %v, want %v", md5, tt.wantMD5)
			}
		})
	}
}

func TestGetMetaMap(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (fs.FS, map[string]struct{})
		wantLen     int
		wantMetaCnt int
		wantErr     bool
	}{
		{
			name: "normal behaviour",
			setup: func(t *testing.T) (fs.FS, map[string]struct{}) {
				fsys := make(fstest.MapFS)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							text1 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.up.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", i, j, j, i)
							text2 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.down.pdf;402bf15aa941b224e8f570%df62f30d9301f26a78304b90d67c%d1c3331144ae56\n", i, j, j, i)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(text1), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
						}
					}
				}
				theMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				return fsys, theMap
			},
			wantLen:     100,
			wantMetaCnt: 2,
			wantErr:     false,
		},
		{
			name: "wrong meta field",
			setup: func(t *testing.T) (fs.FS, map[string]struct{}) {
				fsys := make(fstest.MapFS)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							text1 := fmt.Sprintf("#migration: module/migr;ations/test-module-0.0.0.0.1-%d-%d.up.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", i, j, j, i)
							text2 := fmt.Sprintf("#migration: module/migr;ations/test-module-0.0.0.0.1-%d-%d.down.pdf;402bf15aa941b224e8f570%df62f30d9301f26a78304b90d67c%d1c3331144ae56\n", i, j, j, i)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(text1), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						}
					}
				}
				theMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				return fsys, theMap
			},
			wantLen:     0,
			wantMetaCnt: 0,
			wantErr:     true,
		},
		{
			name: "wrong meta migration name",
			setup: func(t *testing.T) (fs.FS, map[string]struct{}) {
				fsys := make(fstest.MapFS)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							text1 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.right.pdf;402bf15aa94%db224;e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", i, j, j, i)
							text2 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.left.pdf;402bf15aa941b224e8f;570%df62f30d9301f26a78304b90d67c%d1c3331144ae56\n", i, j, j, i)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(text1), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						}
					}
				}
				theMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				return fsys, theMap
			},
			wantLen:     0,
			wantMetaCnt: 0,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys, theMap := tt.setup(t)

			resultMap, err := GetMetaMap(fsys, theMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMetaMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			metaCnt := 0
			for _, meta := range resultMap {
				if !meta.IsOriginal() {
					metaCnt++
				}
			}
			if len(resultMap) != tt.wantLen {
				t.Errorf("GetMetaMap() found = %v, want %v", len(resultMap), tt.wantLen)
			}
			if metaCnt != tt.wantMetaCnt {
				t.Errorf("GetMetaMap() found = %v, want %v", metaCnt, tt.wantMetaCnt)
			}
		})
	}
}

func TestGetModule(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) string
		wantPrefix string
		wantMD5    string
		wantErr    bool
	}{
		{
			name: "normal behaviour for an empty file",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				prefix := "test-project-0.1.0-0-1"
				fileNameUp := fmt.Sprintf("%s.up.txt", prefix)
				fileNameDown := fmt.Sprintf("%s.down.txt", prefix)
				file1 := filepath.Join(tmpDir, fileNameUp)
				file2 := filepath.Join(tmpDir, fileNameDown)
				if err := os.WriteFile(file1, []byte(""), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				if err := os.WriteFile(file2, []byte(""), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				return file1
			},
			wantPrefix: "test-project-0.1.0-0-1",
			wantMD5:    "d41d8cd98f00b204e9800998ecf8427ed41d8cd98f00b204e9800998ecf8427e",
			wantErr:    false,
		},
		{
			name: "get down file and miss up file",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				prefix := "test-project-0.1.0-0-1"
				// fileNameUp := fmt.Sprintf("%s.up.txt", prefix)
				fileNameDown := fmt.Sprintf("%s.down.txt", prefix)
				file2 := filepath.Join(tmpDir, fileNameDown)
				// if err := os.WriteFile(file1, []byte(""), 0644); err != nil {
				// t.Fatalf("error creating file: %v", err)
				// }
				if err := os.WriteFile(file2, []byte(""), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				return file2
			},
			wantPrefix: "test-project-0.1.0-0-1",
			wantMD5:    "",
			wantErr:    false,
		},
		{
			name: "lorem ipsum",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Curabitur felis dolor, fringilla id vulputate eget, volutpat eleifend nisi. In hac habitasse platea dictumst. Maecenas sit amet felis eleifend, blandit nunc et, venenatis turpis. Etiam scelerisque nec arcu ac euismod. Proin maximus est in velit mollis mattis. Ut risus tortor, porttitor eget gravida a, consectetur non nisi. Proin volutpat congue convallis. Sed consectetur fermentum pulvinar. Pellentesque rutrum rutrum maximus. Quisque rhoncus, justo ac gravida auctor, turpis ex pharetra augue, faucibus dignissim dolor leo ut turpis. Maecenas et sem vitae nunc molestie sagittis. Aliquam non tincidunt felis. Nam quis ornare arcu. Maecenas."
				prefix := "lorem-ipsum-0.1.0-0-1"
				fileNameUp := fmt.Sprintf("%s.up.txt", prefix)
				fileNameDown := fmt.Sprintf("%s.down.txt", prefix)
				file1 := filepath.Join(tmpDir, fileNameUp)
				file2 := filepath.Join(tmpDir, fileNameDown)
				if err := os.WriteFile(file1, []byte(text), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				if err := os.WriteFile(file2, []byte(text), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				return file1
			},
			wantPrefix: "lorem-ipsum-0.1.0-0-1",
			wantMD5:    "fecdaaa968e07e70c5e2cdae6e03a836fecdaaa968e07e70c5e2cdae6e03a836",
			wantErr:    false,
		},
		{
			name: "no files",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				file := filepath.Join(tmpDir, "none")
				return file
			},
			wantPrefix: "",
			wantMD5:    "",
			wantErr:    true,
		},
		{
			name: "dir",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				dir := filepath.Join(tmpDir, "dir")
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				return dir
			},
			wantPrefix: "",
			wantMD5:    "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setup(t)

			resultStruct, resultMD5, err := GetModule(filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetModule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if resultStruct.Prefix != tt.wantPrefix {
				t.Errorf("GetModule() prefix = %v, want %v", resultStruct.Prefix, tt.wantPrefix)
			}
			if resultMD5 != tt.wantMD5 {
				t.Errorf("GetModule() resultMD5 = %v, want %v", resultMD5, tt.wantMD5)
			}
		})
	}
}

func TestGetModuleMap(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) map[string]struct{}
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func(t *testing.T) map[string]struct{} {
				tmpDir, err := os.MkdirTemp("", "test_get_module_map_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				testChdirRepo(t, tmpDir)
				gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")
				gitmodulesText := "[submodule \"module\"]\n\tpath = module\n\turl = ./module"
				if err := os.WriteFile(gitmodulesPath, []byte(gitmodulesText), 0644); err != nil {
					t.Fatalf("error creating .gitmodules file: %v", err)
				}
				moduleDir := filepath.Join(tmpDir, "module")
				moduleMigrationDir := filepath.Join(moduleDir, "migrations")
				migrationDir := filepath.Join(tmpDir, "migrations")
				if err := os.MkdirAll(migrationDir, 0755); err != nil {
					t.Fatalf("error creating module dir: %v", err)
				}
				if err := os.MkdirAll(moduleMigrationDir, 0755); err != nil {
					t.Fatalf("error creating module/migrations dir: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 5; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							moduleUpName := fmt.Sprintf("%s-up.test", baseProjectName)
							moduleDownName := fmt.Sprintf("%s-down.test", baseProjectName)
							moduleUpPath := filepath.Join(moduleMigrationDir, moduleUpName)
							moduleDownPath := filepath.Join(moduleMigrationDir, moduleDownName)
							if err := os.WriteFile(moduleUpPath, []byte(moduleUpName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
							if err := os.WriteFile(moduleDownPath, []byte(moduleDownName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
						} else {
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							moduleDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							moduleUpPath := filepath.Join(moduleMigrationDir, moduleUpName)
							moduleDownPath := filepath.Join(moduleMigrationDir, moduleDownName)
							if err := os.WriteFile(moduleUpPath, []byte(moduleUpName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
							if err := os.WriteFile(moduleDownPath, []byte(moduleDownName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
						}
					}
				}
				fsys := os.DirFS(tmpDir)
				theMap, err := getEntriesModuleMap(fsys, "migrations")
				if err != nil {
					t.Fatalf("error getting map of module entries: %v", err)
				}
				return theMap
			},
			wantLen: 24,
			wantErr: false,
		},
		{
			name: "incomplete pair",
			setup: func(t *testing.T) map[string]struct{} {
				tmpDir, err := os.MkdirTemp("", "test_get_module_map_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				testChdirRepo(t, tmpDir)
				gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")
				gitmodulesText := "[submodule \"module\"]\n\tpath = module\n\turl = ./module"
				if err := os.WriteFile(gitmodulesPath, []byte(gitmodulesText), 0644); err != nil {
					t.Fatalf("error creating .gitmodules file: %v", err)
				}
				moduleDir := filepath.Join(tmpDir, "module")
				moduleMigrationDir := filepath.Join(moduleDir, "migrations")
				migrationDir := filepath.Join(tmpDir, "migrations")
				if err := os.MkdirAll(migrationDir, 0755); err != nil {
					t.Fatalf("error creating module dir: %v", err)
				}
				if err := os.MkdirAll(moduleMigrationDir, 0755); err != nil {
					t.Fatalf("error creating module/migrations dir: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 5; i++ {
					for j := 1; j <= 5; j++ {
						if j == 2 {
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							moduleUpPath := filepath.Join(moduleMigrationDir, moduleUpName)
							if err := os.WriteFile(moduleUpPath, []byte(moduleUpName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
						} else {
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							moduleDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							moduleUpPath := filepath.Join(moduleMigrationDir, moduleUpName)
							moduleDownPath := filepath.Join(moduleMigrationDir, moduleDownName)
							if err := os.WriteFile(moduleUpPath, []byte(moduleUpName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
							if err := os.WriteFile(moduleDownPath, []byte(moduleDownName), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
						}
					}
				}
				fsys := os.DirFS(tmpDir)
				theMap, err := getEntriesModuleMap(fsys, "migrations")
				if err != nil {
					t.Fatalf("error getting map of module entries: %v", err)
				}
				return theMap
			},
			wantLen: 20,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moduleMap := tt.setup(t)

			resultMap, err := GetModuleMap(moduleMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetModuleMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantLen {
				t.Errorf("GetModuleMap() len = %v, want %v", len(resultMap), tt.wantLen)
			}
		})
	}
}

func TestSwitchMigrationType(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		direction  string
		wantResult string
		wantErr    bool
	}{
		{
			name:       "default behaviour",
			filename:   "test-project-0.1.1.up.test",
			direction:  "left",
			wantResult: "test-project-0.1.1.left.test",
			wantErr:    false,
		},
		{
			name:       "empty filename",
			filename:   "",
			direction:  "left",
			wantResult: "",
			wantErr:    true,
		},
		{
			name:       "empty filename",
			filename:   "test-project-0.1.1.up.test",
			direction:  "",
			wantResult: "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SwitchMigrationType(tt.filename, tt.direction)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SwitchMigrationType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.wantResult {
				t.Errorf("SwitchMigrationType() result = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestGetProjectInfo(t *testing.T) {
	tests := []struct {
		name        string
		projectPath string
		wantPrefix  string
		wantDir     string
		wantExt     string
		wantErr     bool
	}{
		{
			name:        "default behaviour",
			projectPath: filepath.Join("tmp", "migrations", "test-project-0.1.1.up.test"),
			wantPrefix:  "test-project-0.1.1",
			wantDir:     filepath.Join("tmp", "migrations"),
			wantExt:     "test",
			wantErr:     false,
		},
		{
			name:        "no dir",
			projectPath: "test-project-0.1.1.up.test",
			wantPrefix:  "test-project-0.1.1",
			wantDir:     ".",
			wantExt:     "test",
			wantErr:     false,
		},
		{
			name:        "wrong file name",
			projectPath: "cat.jpg",
			wantPrefix:  "",
			wantDir:     "",
			wantExt:     "",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultStruct, err := getProjectInfo(tt.projectPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getProjectInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if resultStruct.Prefix != tt.wantPrefix {
				t.Errorf("getProjectInfo() prefix = %v, want %v", resultStruct.Prefix, tt.wantPrefix)
			}
			if resultStruct.Dir != tt.wantDir {
				t.Errorf("getProjectInfo() dir = %v, want %v", resultStruct.Dir, tt.wantDir)
			}
			if resultStruct.Ext != tt.wantExt {
				t.Errorf("getProjectInfo() ext = %v, want %v", resultStruct.Ext, tt.wantExt)
			}
		})
	}
}

func TestFillProjectMigrations(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) map[string]Meta
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func(t *testing.T) map[string]Meta {
				fsys := make(fstest.MapFS)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%2 == 0) && (j%2 != 0) {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							text1 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.up.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", i, j, j, i)
							text2 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.down.pdf;402bf15aa941b224e8f570%df62f30d9301f26a78304b90d67c%d1c3331144ae56\n", i, j, j, i)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(text1), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectUpName)), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectDownName)), Mode: 0644}
						}
					}
				}
				tempMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				theMap, err := GetMetaMap(fsys, tempMap)
				if err != nil {
					t.Fatalf("error getting meta map: %v", err)
				}
				return theMap
			},
			wantLen: 15,
			wantErr: false,
		},
		{
			name: "mock map",
			setup: func(t *testing.T) map[string]Meta {
				theMap := make(map[string]Meta)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%2 != 0) && (j%5 != 0) {
							projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, j)
							theMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							theMap[projectDownName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							theMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{},
								MD5:      "",
							}
							theMap[projectDownName] = Meta{
								MetaInfo: MigrationInfo{},
								MD5:      "",
							}
						}
					}
				}
				return theMap
			},
			wantLen: 20,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metaMap := tt.setup(t)

			resultMap, err := fillProjectMigrations(metaMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("fillProjectMigrations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantLen {
				t.Errorf("fillProjectMigrations() len = %v, want %v", len(resultMap), tt.wantLen)
			}
		})
	}
}

func TestFillModuleMigrations(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) map[string]Meta
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func(t *testing.T) map[string]Meta {
				fsys := make(fstest.MapFS)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%2 == 0) && (j%2 != 0) {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							text1 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.up.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", i, j, j, i)
							text2 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.down.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", i, j, j, i)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(text1), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectUpName)), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectDownName)), Mode: 0644}
						}
					}
				}
				tempMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				theMap, err := GetMetaMap(fsys, tempMap)
				if err != nil {
					t.Fatalf("error getting meta map: %v", err)
				}
				return theMap
			},
			wantLen: 15,
			wantErr: false,
		},
		{
			name: "lack of down files",
			setup: func(t *testing.T) map[string]Meta {
				fsys := make(fstest.MapFS)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%2 == 0) && (j%2 != 0) {
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							text2 := fmt.Sprintf("#migration: module/migrations/test-module-0.0.0.0.1-%d-%d.down.pdf;402bf15aa941b224e8f570%df62f30d9301f26a78304b90d67c%d1c3331144ae56\n", i, j, j, i)
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectUpName)), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectDownName)), Mode: 0644}
						}
					}
				}
				tempMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				theMap, err := GetMetaMap(fsys, tempMap)
				if err != nil {
					t.Fatalf("error getting meta map: %v", err)
				}
				return theMap
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "wrong migration file name",
			setup: func(t *testing.T) map[string]Meta {
				fsys := make(fstest.MapFS)
				wrongProjectName := "5#$134ss"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%2 == 0) && (j%2 != 0) {
							projectUpName := fmt.Sprintf("%s-%v-%v.left.sql", wrongProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.right.sql", wrongProjectName, i, j)
							text1 := fmt.Sprintf("#migration: %s-%d-%d.up.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", wrongProjectName, i, j, j, i)
							text2 := fmt.Sprintf("#migration: %s-%d-%d.down.pdf;402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56\n", wrongProjectName, i, j, j, i)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(text1), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(text2), Mode: 0644}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.left.sql", wrongProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.right.sql", wrongProjectName, i, j)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectUpName)), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(fmt.Sprintf("# %s", projectDownName)), Mode: 0644}
						}
					}
				}
				tempMap, err := getEntriesProjectMap(fsys, ".")
				if err != nil {
					t.Fatalf("error getting map of project entries: %v", err)
				}
				theMap, err := GetMetaMap(fsys, tempMap)
				if err != nil {
					t.Fatalf("error getting meta map: %v", err)
				}
				return theMap
			},
			wantLen: 0,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metaMap := tt.setup(t)

			resultMap, err := fillModuleMigrations(metaMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("fillProjectMigrations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantLen {
				t.Errorf("fillProjectMigrations() len = %v, want %v", len(resultMap), tt.wantLen)
			}
		})
	}
}

func TestCheckPairs(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) map[string]struct{}
		wantMissingLen int
		wantErr        bool
	}{
		{
			name: "normal behaviour",
			setup: func(t *testing.T) map[string]struct{} {
				entryMap := make(map[string]struct{})
				// wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%3 != 0) && (j%2 == 0) {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
						}
					}
				}

				return entryMap
			},
			wantMissingLen: 14,
			wantErr:        false,
		},
		{
			name: "wrong migration type (right/left)",
			setup: func(t *testing.T) map[string]struct{} {
				entryMap := make(map[string]struct{})
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							projectUpName := fmt.Sprintf("%s-%v-%v.left.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.right.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
						}
					}
				}

				return entryMap
			},
			wantMissingLen: 0,
			wantErr:        true,
		},
		{
			name: "wrong migration name",
			setup: func(t *testing.T) map[string]struct{} {
				entryMap := make(map[string]struct{})
				wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							projectUpName := fmt.Sprintf("%s-%v-%v.wrong", wrongProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.wrong", wrongProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
						}
					}
				}

				return entryMap
			},
			wantMissingLen: 0,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryMap := tt.setup(t)

			resultMap, err := checkPairs(entryMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkPairs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantMissingLen {
				t.Errorf("checkPairs() found = %v, want %v", len(resultMap), tt.wantMissingLen)
			}
		})
	}
}

func TestProcessMissingProjectPairs(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (map[string]string, map[string]Meta)
		wantMissingLen int
		wantLostLen    int
	}{
		{
			name: "normal behaviour (only missed)",
			setup: func(t *testing.T) (map[string]string, map[string]Meta) {
				entryMap := make(map[string]struct{})
				metaMap := map[string]Meta{}
				// wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%3 != 0) && (j%2 == 0) {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						}
					}
				}
				missingMap, err := checkPairs(entryMap)
				if err != nil {
					t.Fatalf("error checking pairs: %v", err)
				}
				return missingMap, metaMap
			},
			wantMissingLen: 14,
			wantLostLen:    0,
		},
		{
			name: "normal behaviour (missed & lost)",
			setup: func(t *testing.T) (map[string]string, map[string]Meta) {
				entryMap := make(map[string]struct{})
				metaMap := map[string]Meta{}
				// wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						case (i%3 != 0) && (j%2 != 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{},
								MD5:      "",
							}
						default:
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						}
					}
				}
				missingMap, err := checkPairs(entryMap)
				if err != nil {
					t.Fatalf("error checking pairs: %v", err)
				}
				return missingMap, metaMap
			},
			wantMissingLen: 14,
			wantLostLen:    21,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missingMap, metaMap := tt.setup(t)

			lostMap, missMap := processMissingProjectPairs(missingMap, metaMap)
			if len(lostMap) != tt.wantLostLen {
				t.Errorf("processMissingProjectPairs() found = %v, want %v", len(lostMap), tt.wantLostLen)
			}
			if len(missMap) != tt.wantMissingLen {
				t.Errorf("processMissingProjectPairs() found = %v, want %v", len(missMap), tt.wantMissingLen)
			}
		})
	}
}

func TestCheckDeletedFiles(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (map[string]Meta, map[string]struct{})
		wantDeletedLen int
		wantErr        bool
	}{
		{
			name: "normal behaviour (every module file exists)",
			setup: func(t *testing.T) (map[string]Meta, map[string]struct{}) {
				moduleEntriesMap := make(map[string]struct{})
				metaMap := map[string]Meta{}
				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
							moduleUpPath := filepath.Join(moduleDir, moduleUpName)
							moduleEntriesMap[moduleUpPath] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						case (i%3 != 0) && (j%2 != 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
							moduleUpPath := filepath.Join(moduleDir, moduleUpName)
							moduleEntriesMap[moduleUpPath] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{},
								MD5:      "",
							}
						default:
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
							moduleDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseModuleName, i, j)
							moduleUpPath := filepath.Join(moduleDir, moduleUpName)
							moduleDownPath := filepath.Join(moduleDir, moduleDownName)
							moduleEntriesMap[moduleUpPath] = struct{}{}
							moduleEntriesMap[moduleDownPath] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						}
					}
				}

				return metaMap, moduleEntriesMap
			},
			wantDeletedLen: 0,
			wantErr:        false,
		},
		{
			name: "normal behaviour (some module files are missing)",
			setup: func(t *testing.T) (map[string]Meta, map[string]struct{}) {
				moduleEntriesMap := make(map[string]struct{})
				metaMap := map[string]Meta{}
				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						case (i%3 != 0) && (j%2 != 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{},
								MD5:      "",
							}
						default:
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							moduleUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
							moduleDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseModuleName, i, j)
							moduleUpPath := filepath.Join(moduleDir, moduleUpName)
							moduleDownPath := filepath.Join(moduleDir, moduleDownName)
							moduleEntriesMap[moduleUpPath] = struct{}{}
							moduleEntriesMap[moduleDownPath] = struct{}{}
							metaMap[projectUpName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = Meta{
								MetaInfo: MigrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
						}
					}
				}

				return metaMap, moduleEntriesMap
			},
			wantDeletedLen: 28,
			wantErr:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metaMap, moduleEntriesMap := tt.setup(t)

			rslt, err := checkDeletedFiles(metaMap, moduleEntriesMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkDeletedFiles() error = %v, want %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantDeletedLen {
				t.Errorf("checkDeletedFiles() found = %v, want %v", len(rslt), tt.wantDeletedLen)
			}
		})
	}
}

func TestCheckMissedFiles(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) (map[string]MigrationInfo, map[string]MigrationInfo)
		wantMissedLen int
	}{
		{
			name: "normal behaviour (every module file exists)",
			setup: func(t *testing.T) (map[string]MigrationInfo, map[string]MigrationInfo) {
				projectMigrations := map[string]MigrationInfo{}
				moduleMap := map[string]MigrationInfo{}
				// baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							moduleMap[md5] = MigrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						case (i%3 != 0) && (j%2 != 0):
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[md5] = MigrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						default:
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[md5] = MigrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
							moduleMap[md5] = MigrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						}
					}
				}

				return projectMigrations, moduleMap
			},
			wantMissedLen: 28,
		},
		{
			name: "normal behaviour (every module file exists)",
			setup: func(t *testing.T) (map[string]MigrationInfo, map[string]MigrationInfo) {
				projectMigrations := map[string]MigrationInfo{}
				moduleMap := map[string]MigrationInfo{}
				// baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
						projectMigrations[md5] = MigrationInfo{
							Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
							Dir:          moduleDir,
							Ext:          "sql",
							UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
							DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
						}
						moduleMap[md5] = MigrationInfo{
							Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
							Dir:          moduleDir,
							Ext:          "sql",
							UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
							DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
						}
					}
				}

				return projectMigrations, moduleMap
			},
			wantMissedLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectMigrations, moduleMap := tt.setup(t)

			rslt := checkMissedFiles(projectMigrations, moduleMap)
			if len(rslt) != tt.wantMissedLen {
				t.Errorf("checkMissedFiles() found = %v, want %v", len(rslt), tt.wantMissedLen)
			}
		})
	}
}

func TestGetProjectParseContext(t *testing.T) {

}
