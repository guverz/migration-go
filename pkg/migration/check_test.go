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
	return stripDir(filepath.Join(testMigrationStripPrefix, "module", "migrations", moduleSQLFileName))
}

func TestParseIncludes(t *testing.T) {
	tests := []struct {
		name             string
		setup            func() string
		wantMissingFiles int
		wantIncludes     int
		wantErr          bool
	}{
		{
			name: "normal",
			setup: func() string {
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
			setup: func() string {
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
			setup: func() string {
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
			includedFile := tt.setup()
			ctx := newParseContext()
			err := parseIncludes(ctx, includedFile, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInclude() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(ctx.MissingFiles) != tt.wantMissingFiles {
				t.Errorf("parseInclude() found = %v, want %v", len(ctx.MissingFiles), tt.wantMissingFiles)
			}
			if len(ctx.Includes) != tt.wantIncludes {
				t.Errorf("parseInclude() found = %v, want %v", len(ctx.Includes), tt.wantIncludes)
			}
		})
	}
}

func TestGetMetaInfo(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (fs.FS, string)
		wantDir string
		wantUp  string
		wantMD5 string
		wantErr bool
	}{
		{
			name: "fsys check",
			setup: func() (fs.FS, string) {
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
			setup: func() (fs.FS, string) {
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
			fsys, file := tt.setup()

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
		setup       func() (fs.FS, map[string]struct{})
		wantLen     int
		wantMetaCnt int
		wantErr     bool
	}{
		{
			name: "normal behaviour",
			setup: func() (fs.FS, map[string]struct{}) {
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
			setup: func() (fs.FS, map[string]struct{}) {
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
			setup: func() (fs.FS, map[string]struct{}) {
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
			fsys, theMap := tt.setup()

			resultMap, err := getMetaMap(fsys, theMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("getMetaMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			metaCnt := 0
			for _, meta := range resultMap {
				if !meta.isOriginal() {
					metaCnt++
				}
			}
			if len(resultMap) != tt.wantLen {
				t.Errorf("getMetaMap() found = %v, want %v", len(resultMap), tt.wantLen)
			}
			if metaCnt != tt.wantMetaCnt {
				t.Errorf("getMetaMap() found = %v, want %v", metaCnt, tt.wantMetaCnt)
			}
		})
	}
}

func TestGetModule(t *testing.T) {
	tests := []struct {
		name       string
		setup      func() string
		wantPrefix string
		wantMD5    string
		wantErr    bool
	}{
		{
			name: "normal behaviour for an empty file",
			setup: func() string {
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
			setup: func() string {
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
			setup: func() string {
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
			setup: func() string {
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
			setup: func() string {
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
			filePath := tt.setup()

			resultStruct, resultMD5, err := getModule(filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("getModule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if resultStruct.Prefix != tt.wantPrefix {
				t.Errorf("getModule() prefix = %v, want %v", resultStruct.Prefix, tt.wantPrefix)
			}
			if resultMD5 != tt.wantMD5 {
				t.Errorf("getModule() resultMD5 = %v, want %v", resultMD5, tt.wantMD5)
			}
		})
	}
}

func TestGetModuleMap(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() map[string]struct{}
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]struct{} {
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
			setup: func() map[string]struct{} {
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
			moduleMap := tt.setup()

			resultMap, err := getModuleMap(moduleMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("getModuleMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantLen {
				t.Errorf("getModuleMap() len = %v, want %v", len(resultMap), tt.wantLen)
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
			result, err := switchMigrationType(tt.filename, tt.direction)
			if (err != nil) != tt.wantErr {
				t.Fatalf("switchMigrationType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.wantResult {
				t.Errorf("switchMigrationType() result = %v, want %v", result, tt.wantResult)
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
		setup   func() map[string]meta
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]meta {
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
				theMap, err := getMetaMap(fsys, tempMap)
				if err != nil {
					t.Fatalf("error getting meta map: %v", err)
				}
				return theMap
			},
			wantLen: 30,
			wantErr: false,
		},
		{
			name: "mock map",
			setup: func() map[string]meta {
				theMap := make(map[string]meta)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%2 != 0) && (j%5 != 0) {
							projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, j)
							theMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							theMap[projectDownName] = meta{
								MetaInfo: migrationInfo{
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
							theMap[projectUpName] = meta{
								MetaInfo: migrationInfo{},
								MD5:      "",
							}
							theMap[projectDownName] = meta{
								MetaInfo: migrationInfo{},
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
			metaMap := tt.setup()

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
		setup   func() map[string]meta
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]meta {
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
				theMap, err := getMetaMap(fsys, tempMap)
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
			setup: func() map[string]meta {
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
				theMap, err := getMetaMap(fsys, tempMap)
				if err != nil {
					t.Fatalf("error getting meta map: %v", err)
				}
				return theMap
			},
			wantLen: 15,
			wantErr: false,
		},
		{
			name: "wrong migration file name",
			setup: func() map[string]meta {
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
				theMap, err := getMetaMap(fsys, tempMap)
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
			metaMap := tt.setup()

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
		setup          func() map[string]struct{}
		wantMissingLen int
		wantErr        bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]struct{} {
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
			setup: func() map[string]struct{} {
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
			setup: func() map[string]struct{} {
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
			entryMap := tt.setup()

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
		setup          func() (map[string]string, map[string]meta)
		wantMissingLen int
		wantLostLen    int
	}{
		{
			name: "normal behaviour (only missed)",
			setup: func() (map[string]string, map[string]meta) {
				entryMap := make(map[string]struct{})
				metaMap := map[string]meta{}
				// wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if (i%3 != 0) && (j%2 == 0) {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = meta{
								MetaInfo: migrationInfo{
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
			setup: func() (map[string]string, map[string]meta) {
				entryMap := make(map[string]struct{})
				metaMap := map[string]meta{}
				// wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{},
								MD5:      "",
							}
						default:
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							entryMap[projectUpName] = struct{}{}
							entryMap[projectDownName] = struct{}{}
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
									Prefix:       fmt.Sprintf("test-module-0.0.0.0.1-%d-%d", i, j),
									Dir:          "module/migrations",
									Ext:          "pdf",
									UpFileName:   fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.up.pdf", i, j),
									DownFileName: fmt.Sprintf("test-module-0.0.0.0.1-%d-%d.down.pdf", i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = meta{
								MetaInfo: migrationInfo{
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
			missingMap, metaMap := tt.setup()

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
		setup          func() (map[string]meta, map[string]struct{})
		wantDeletedLen int
		wantErr        bool
	}{
		{
			name: "normal behaviour (every module file exists)",
			setup: func() (map[string]meta, map[string]struct{}) {
				moduleEntriesMap := make(map[string]struct{})
				metaMap := map[string]meta{}
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{},
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = meta{
								MetaInfo: migrationInfo{
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
			setup: func() (map[string]meta, map[string]struct{}) {
				moduleEntriesMap := make(map[string]struct{})
				metaMap := map[string]meta{}
				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{},
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
							metaMap[projectUpName] = meta{
								MetaInfo: migrationInfo{
									Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
									Dir:          moduleDir,
									Ext:          "sql",
									UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
									DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
								},
								MD5: fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i),
							}
							metaMap[projectDownName] = meta{
								MetaInfo: migrationInfo{
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
			metaMap, moduleEntriesMap := tt.setup()

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
		setup         func() (map[string]migrationInfo, map[string]migrationInfo)
		wantMissedLen int
	}{
		{
			name: "normal behaviour (new module migration files)",
			setup: func() (map[string]migrationInfo, map[string]migrationInfo) {
				projectMigrations := map[string]migrationInfo{}
				moduleMap := map[string]migrationInfo{}
				// baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							moduleMap[md5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						case (i%3 != 0) && (j%2 != 0):
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[md5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						default:
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[md5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
							moduleMap[md5] = migrationInfo{
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
			name: "normal behaviour (changed module migration files)",
			setup: func() (map[string]migrationInfo, map[string]migrationInfo) {
				projectMigrations := map[string]migrationInfo{}
				moduleMap := map[string]migrationInfo{}
				// baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						switch {
						case (i%3 != 0) && (j%2 == 0):
							firstMD5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							moduleMap[firstMD5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
							secondMD5 := fmt.Sprintf("402bf15aa9%d4b224e8f5700f62f30d93%d0f26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[secondMD5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						case (i%3 != 0) && (j%2 != 0):
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[md5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
						default:
							md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
							projectMigrations[md5] = migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
								Dir:          moduleDir,
								Ext:          "sql",
								UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
								DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
							}
							moduleMap[md5] = migrationInfo{
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
			setup: func() (map[string]migrationInfo, map[string]migrationInfo) {
				projectMigrations := map[string]migrationInfo{}
				moduleMap := map[string]migrationInfo{}
				// baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				moduleDir := filepath.Join("module", "migrations")
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", j, i)
						projectMigrations[md5] = migrationInfo{
							Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, j),
							Dir:          moduleDir,
							Ext:          "sql",
							UpFileName:   fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, j),
							DownFileName: fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, j),
						}
						moduleMap[md5] = migrationInfo{
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
			projectMigrations, moduleMap := tt.setup()

			rslt := checkMissedFiles(projectMigrations, moduleMap)
			if len(rslt) != tt.wantMissedLen {
				t.Errorf("checkMissedFiles() found = %v, want %v", len(rslt), tt.wantMissedLen)
			}
		})
	}
}

func TestGetMapParseContext(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() map[string]struct{}
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]struct{} {
				tmpDir, err := os.MkdirTemp("", "test_get_map_parsecontext_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				entriesMap := make(map[string]struct{})
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, i-1)
					projectUpPath := filepath.Join(projectDir, projectUpName)
					projectDownPath := filepath.Join(projectDir, projectDownName)
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					includePath := filepath.Join(includesDir, includeName)
					if err := os.WriteFile(projectUpPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(projectDownPath, []byte{}, 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(includePath, []byte{}, 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					entriesMap[projectUpPath] = struct{}{}
					entriesMap[projectDownPath] = struct{}{}

				}

				return entriesMap
			},
			wantLen: 10,
			wantErr: false,
		},
		{
			name: "only down files",
			setup: func() map[string]struct{} {
				tmpDir, err := os.MkdirTemp("", "test_get_map_parsecontext_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				entriesMap := make(map[string]struct{})
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, i-1)
					projectDownPath := filepath.Join(projectDir, projectDownName)
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					includePath := filepath.Join(includesDir, includeName)
					if err := os.WriteFile(projectDownPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(includePath, []byte{}, 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					entriesMap[projectDownPath] = struct{}{}

				}

				return entriesMap
			},
			wantLen: 0,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectMigrations := tt.setup()

			rslt, err := getMapParseContext(projectMigrations)
			if (err != nil) != tt.wantErr {
				t.Errorf("getProjectParseContext() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantLen {
				t.Errorf("getProjectParseContext() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestGetMetaParseContext(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() map[string]meta
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]meta {
				tmpDir, err := os.MkdirTemp("", "test_get_meta_parsecontext_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				metaMap := make(map[string]meta)
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				includesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				projectDir := filepath.Join(tmpDir, "migrations")
				baseModuleName := "test-module-0.2.0"
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 25; i++ {
					if i%5 == 0 {
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
						projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, i-1)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						projectDownPath := filepath.Join(projectDir, projectDownName)
						metaMap[projectUpPath] = meta{
							MetaInfo: migrationInfo{},
							MD5:      "",
						}
						metaMap[projectDownPath] = meta{
							MetaInfo: migrationInfo{},
							MD5:      "",
						}
					} else {
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
						projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, i-1)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						projectDownPath := filepath.Join(projectDir, projectDownName)
						moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						moduleDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, i-1)
						moduleUpPath := filepath.Join(moduleDir, moduleUpName)
						moduleDownPath := filepath.Join(moduleDir, moduleDownName)
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						if err := os.WriteFile(moduleUpPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						if err := os.WriteFile(moduleDownPath, []byte{}, 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						if err := os.WriteFile(includePath, []byte{}, 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", i, i%3)
						metaMap[projectUpPath] = meta{
							MetaInfo: migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, i-1),
								Ext:          "sql",
								Dir:          moduleDir,
								UpFileName:   moduleUpName,
								DownFileName: moduleDownName,
							},
							MD5: md5,
						}
						metaMap[projectDownPath] = meta{
							MetaInfo: migrationInfo{
								Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, i-1),
								Ext:          "sql",
								Dir:          moduleDir,
								UpFileName:   moduleUpName,
								DownFileName: moduleDownName,
							},
							MD5: md5,
						}
					}

				}
				return metaMap
			},
			wantLen: 20,
			wantErr: false,
		},
		{
			name: "only down files",
			setup: func() map[string]meta {
				tmpDir, err := os.MkdirTemp("", "test_get_meta_parsecontext_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				metaMap := make(map[string]meta)
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				includesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				projectDir := filepath.Join(tmpDir, "migrations")
				baseModuleName := "test-module-0.2.0"
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 25; i++ {

					projectDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseProjectName, i, i-1)
					projectDownPath := filepath.Join(projectDir, projectDownName)
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleDownName := fmt.Sprintf("%s-%d-%d.down.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					moduleDownPath := filepath.Join(moduleDir, moduleDownName)
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					includePath := filepath.Join(includesDir, includeName)
					if err := os.WriteFile(moduleUpPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(moduleDownPath, []byte{}, 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(includePath, []byte{}, 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", i, i%3)
					metaMap[projectDownPath] = meta{
						MetaInfo: migrationInfo{
							Prefix:       fmt.Sprintf("%s-%d-%d", baseModuleName, i, i-1),
							Ext:          "sql",
							Dir:          moduleDir,
							UpFileName:   moduleUpName,
							DownFileName: moduleDownName,
						},
						MD5: md5,
					}

				}
				return metaMap
			},
			wantLen: 0,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metaMap := tt.setup()

			rslt, err := getMetaParseContext(metaMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("getProjectParseContext() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantLen {
				t.Errorf("getProjectParseContext() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestGetProjectMD5Includes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() map[string]parseContext
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]parseContext {
				tmpDir, err := os.MkdirTemp("", "test_get_project_md5_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					includePath := filepath.Join(includesDir, includeName)
					if err := os.WriteFile(includePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					includesMap := make(map[string]string)
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					includesMap[includePath] = projectUpName
					projectContext[projectUpName] = parseContext{
						Includes: includesMap,
					}
				}
				return projectContext
			},
			wantLen: 10,
			wantErr: false,
		},
		{
			name: "same includes properties",
			setup: func() map[string]parseContext {
				tmpDir, err := os.MkdirTemp("", "test_get_project_md5_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					includePath := filepath.Join(includesDir, includeName)
					if err := os.WriteFile(includePath, []byte(baseIncludeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					includesMap := make(map[string]string)
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					includesMap[includePath] = projectUpName
					projectContext[projectUpName] = parseContext{
						Includes: includesMap,
					}
				}
				return projectContext
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "several includes",
			setup: func() map[string]parseContext {
				tmpDir, err := os.MkdirTemp("", "test_get_project_md5_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseInclude := "include"
				for i := 1; i <= 10; i++ {
					for j := 11; j <= 20; j++ {
						includeName1 := fmt.Sprintf("%s%d.sql", baseInclude, i)
						includeName2 := fmt.Sprintf("%s%d.sql", baseInclude, j)
						includePath1 := filepath.Join(includesDir, includeName1)
						includePath2 := filepath.Join(includesDir, includeName2)
						if err := os.WriteFile(includePath1, []byte(includeName1), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						if err := os.WriteFile(includePath2, []byte(includeName2), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						includesMap := make(map[string]string)
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, j)
						includesMap[includePath1] = projectUpName
						includesMap[includePath2] = projectUpName
						projectContext[projectUpName] = parseContext{
							Includes: includesMap,
						}
					}
				}
				return projectContext
			},
			wantLen: 20,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectContext := tt.setup()

			rslt, err := getProjectMD5Includes(projectContext)
			if (err != nil) != tt.wantErr {
				t.Errorf("getProjectMD5Includes() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantLen {
				t.Errorf("getProjectMD5Includes() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestFillProjectIncludes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (map[string]parseContext, map[string]meta)
		wantLen int
	}{
		{
			name: "normal behaviour",
			setup: func() (map[string]parseContext, map[string]meta) {
				tmpDir, err := os.MkdirTemp("", "test_fill_project_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				metaMap := make(map[string]meta)
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					if i%2 != 0 {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						if err := os.WriteFile(includePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						includesMap := make(map[string]string)
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						includesMap[includePath] = projectUpPath
						projectContext[projectUpPath] = parseContext{
							Includes: includesMap,
						}
						metaMap[projectUpPath] = meta{
							MetaInfo: migrationInfo{},
							MD5:      "temp",
						}
					} else {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						if err := os.WriteFile(includePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						includesMap := make(map[string]string)
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						includesMap[includePath] = projectUpPath
						projectContext[projectUpPath] = parseContext{
							Includes: includesMap,
						}
						metaMap[projectUpPath] = meta{
							MetaInfo: migrationInfo{},
							MD5:      "",
						}
					}

				}
				return projectContext, metaMap
			},
			wantLen: 5,
		},
		{
			name: "normal behaviour",
			setup: func() (map[string]parseContext, map[string]meta) {
				tmpDir, err := os.MkdirTemp("", "test_fill_project_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				metaMap := make(map[string]meta)
				projectDir := filepath.Join(tmpDir, "migrations")
				includesDir := filepath.Join(projectDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					includePath := filepath.Join(includesDir, includeName)
					if err := os.WriteFile(includePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					includesMap := make(map[string]string)
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					projectUpPath := filepath.Join(projectDir, projectUpName)
					includesMap[includePath] = projectUpPath
					projectContext[projectUpPath] = parseContext{
						Includes: includesMap,
					}
					metaMap[projectUpPath] = meta{
						MetaInfo: migrationInfo{},
						MD5:      "meta",
					}
				}
				return projectContext, metaMap
			},
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectContext, metaMap := tt.setup()

			rslt := fillProjectIncludes(projectContext, metaMap)
			if len(rslt) != tt.wantLen {
				t.Errorf("fillProjectIncludes() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestFillModuleIncludes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (map[string]parseContext, map[string]string)
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() (map[string]parseContext, map[string]string) {
				tmpDir, err := os.MkdirTemp("", "test_fill_module_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				metaContext := make(map[string]parseContext)
				projectMD5Includes := make(map[string]string)
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				includesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					if i == 4 {
						text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Curabitur felis dolor, fringilla id vulputate eget, volutpat eleifend nisi. In hac habitasse platea dictumst. Maecenas sit amet felis eleifend, blandit nunc et, venenatis turpis. Etiam scelerisque nec arcu ac euismod. Proin maximus est in velit mollis mattis. Ut risus tortor, porttitor eget gravida a, consectetur non nisi. Proin volutpat congue convallis. Sed consectetur fermentum pulvinar. Pellentesque rutrum rutrum maximus. Quisque rhoncus, justo ac gravida auctor, turpis ex pharetra augue, faucibus dignissim dolor leo ut turpis. Maecenas et sem vitae nunc molestie sagittis. Aliquam non tincidunt felis. Nam quis ornare arcu. Maecenas."
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						if err := os.WriteFile(includePath, []byte(text), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						md5 := "fecdaaa968e07e70c5e2cdae6e03a836"
						includesMap := make(map[string]string)
						metaUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						metaUpPath := filepath.Join(moduleDir, metaUpName)
						includesMap[includePath] = metaUpPath
						metaContext[metaUpPath] = parseContext{
							Includes: includesMap,
						}
						projectMD5Includes[md5] = includePath
					} else {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						if err := os.WriteFile(includePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", i, i%3)
						includesMap := make(map[string]string)
						metaUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						metaUpPath := filepath.Join(moduleDir, metaUpName)
						includesMap[includePath] = metaUpPath
						metaContext[metaUpPath] = parseContext{
							Includes: includesMap,
						}
						projectMD5Includes[md5] = includePath
					}
				}
				return metaContext, projectMD5Includes
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "no include file",
			setup: func() (map[string]parseContext, map[string]string) {
				tmpDir, err := os.MkdirTemp("", "test_fill_module_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				metaContext := make(map[string]parseContext)
				projectMD5Includes := make(map[string]string)
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				includesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(includesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					if i == 4 {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						md5 := "fecdaaa968e07e70c5e2cdae6e03a836"
						includesMap := make(map[string]string)
						metaUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						metaUpPath := filepath.Join(moduleDir, metaUpName)
						includesMap[includePath] = metaUpPath
						metaContext[metaUpPath] = parseContext{
							Includes: includesMap,
						}
						projectMD5Includes[md5] = includePath
					} else {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						includePath := filepath.Join(includesDir, includeName)
						if err := os.WriteFile(includePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						md5 := fmt.Sprintf("402bf15aa94%db224e8f5700f62f30d930%df26a78304b90d67c51c3331144ae56", i, i%3)
						includesMap := make(map[string]string)
						metaUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						metaUpPath := filepath.Join(moduleDir, metaUpName)
						includesMap[includePath] = metaUpPath
						metaContext[metaUpPath] = parseContext{
							Includes: includesMap,
						}
						projectMD5Includes[md5] = includePath
					}
				}
				return metaContext, projectMD5Includes
			},
			wantLen: 0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectContext, projectMD5Includes := tt.setup()

			rslt, err := fillModuleIncludes(projectContext, projectMD5Includes)
			if (err != nil) != tt.wantErr {
				t.Errorf("fillModuleIncludes() err = %v, want %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantLen {
				t.Errorf("fillModuleIncludes() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestCheckDeletedIncludes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (map[string]parseContext, map[string]meta)
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() (map[string]parseContext, map[string]meta) {
				tmpDir, err := os.MkdirTemp("", "test_check_deleted_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				metaMap := make(map[string]meta)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					projectIncludePath := filepath.Join(projectIncludesDir, includeName)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					if err := os.WriteFile(projectIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					includesMap := make(map[string]string)
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					projectUpPath := filepath.Join(projectDir, projectUpName)
					includesMap[projectIncludePath] = projectUpPath
					projectContext[projectUpPath] = parseContext{
						Includes: includesMap,
					}
					metaMap[projectUpPath] = meta{
						MetaInfo: migrationInfo{
							Dir: moduleDir,
						},
						MD5: "meta",
					}
				}
				return projectContext, metaMap
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "missing meta include",
			setup: func() (map[string]parseContext, map[string]meta) {
				tmpDir, err := os.MkdirTemp("", "test_check_deleted_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				metaMap := make(map[string]meta)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					if i%3 == 1 {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						projectIncludePath := filepath.Join(projectIncludesDir, includeName)
						if err := os.WriteFile(projectIncludePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						includesMap := make(map[string]string)
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						includesMap[projectIncludePath] = projectUpPath
						projectContext[projectUpPath] = parseContext{
							Includes: includesMap,
						}
						metaMap[projectUpPath] = meta{
							MetaInfo: migrationInfo{
								Dir: moduleDir,
							},
							MD5: "meta",
						}
					} else {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						projectIncludePath := filepath.Join(projectIncludesDir, includeName)
						moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
						if err := os.WriteFile(projectIncludePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						includesMap := make(map[string]string)
						projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						includesMap[projectIncludePath] = projectUpPath
						projectContext[projectUpPath] = parseContext{
							Includes: includesMap,
						}
						metaMap[projectUpPath] = meta{
							MetaInfo: migrationInfo{
								Dir: moduleDir,
							},
							MD5: "meta",
						}
					}

				}
				return projectContext, metaMap
			},
			wantLen: 4,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectContext, metaMap := tt.setup()

			rslt, err := checkDeletedIncludes(projectContext, metaMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkDeletedIncludes() err = %v, want %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantLen {
				t.Errorf("checkDeletedIncludes() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestCheckMissedIncludes(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (map[string]parseContext, map[string]meta, map[string]parseContext)
		wantLen int
		wantErr bool
	}{
		{
			name: "normal behaviour",
			setup: func() (map[string]parseContext, map[string]meta, map[string]parseContext) {
				tmpDir, err := os.MkdirTemp("", "test_check_missed_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				moduleContext := make(map[string]parseContext)
				metaMap := make(map[string]meta)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					projectIncludePath := filepath.Join(projectIncludesDir, includeName)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					if err := os.WriteFile(projectIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					includesProjectMap, includesModuleMap := make(map[string]string), make(map[string]string)
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					projectUpPath := filepath.Join(projectDir, projectUpName)
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					includesProjectMap[projectIncludePath] = projectUpPath
					includesModuleMap[moduleIncludePath] = moduleUpPath
					moduleContext[moduleUpPath] = parseContext{
						Includes: includesModuleMap,
					}
					projectContext[projectUpPath] = parseContext{
						Includes: includesProjectMap,
					}
					metaMap[projectUpPath] = meta{
						MetaInfo: migrationInfo{
							Dir:        moduleDir,
							UpFileName: moduleUpName,
						},
						MD5: "meta",
					}
				}
				return projectContext, metaMap, moduleContext
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "changed includes",
			setup: func() (map[string]parseContext, map[string]meta, map[string]parseContext) {
				tmpDir, err := os.MkdirTemp("", "test_check_missed_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectContext := make(map[string]parseContext)
				moduleContext := make(map[string]parseContext)
				metaMap := make(map[string]meta)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseProjectName := "test-project-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					projectIncludePath := filepath.Join(projectIncludesDir, includeName)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					if err := os.WriteFile(projectIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(moduleIncludePath, []byte(fmt.Sprintf("%stemp", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					includesProjectMap, includesModuleMap := make(map[string]string), make(map[string]string)
					projectUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseProjectName, i, i-1)
					projectUpPath := filepath.Join(projectDir, projectUpName)
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					includesProjectMap[projectIncludePath] = projectUpPath
					includesModuleMap[moduleIncludePath] = moduleUpPath
					moduleContext[moduleUpPath] = parseContext{
						Includes: includesModuleMap,
					}
					projectContext[projectUpPath] = parseContext{
						Includes: includesProjectMap,
					}
					metaMap[projectUpPath] = meta{
						MetaInfo: migrationInfo{
							Dir:        moduleDir,
							UpFileName: moduleUpName,
						},
						MD5: "meta",
					}
				}
				return projectContext, metaMap, moduleContext
			},
			wantLen: 10,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectContext, metaMap, moduleContext := tt.setup()

			rslt, err := checkMissedIncludes(projectContext, metaMap, moduleContext)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkMissedIncludes() err = %v, want %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantLen {
				t.Errorf("checkMissedIncludes() found = %v, want %v", len(rslt), tt.wantLen)
			}
		})
	}
}

func TestCheckMissedFilesForIncludes(t *testing.T) {
	tests := []struct {
		name           string
		setup          func() map[string]migrationInfo
		wantIncludeLen int
		wantWarnLen    int
		wantErr        bool
	}{
		{
			name: "normal behaviour",
			setup: func() map[string]migrationInfo {
				tmpDir, err := os.MkdirTemp("", "test_check_missed_files_for_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				missedFiles := make(map[string]migrationInfo)
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleDownName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					if err := os.WriteFile(moduleUpPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					missedFiles[moduleUpPath] = migrationInfo{
						Dir:          moduleDir,
						UpFileName:   moduleUpName,
						DownFileName: moduleDownName,
					}
				}
				return missedFiles
			},
			wantIncludeLen: 10,
			wantWarnLen:    0,
			wantErr:        false,
		},
		{
			name: "deleted includes",
			setup: func() map[string]migrationInfo {
				tmpDir, err := os.MkdirTemp("", "test_check_missed_files_for_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				missedFiles := make(map[string]migrationInfo)
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					if i%3 == 1 {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						moduleDownName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						moduleUpPath := filepath.Join(moduleDir, moduleUpName)
						if err := os.WriteFile(moduleUpPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						missedFiles[moduleUpPath] = migrationInfo{
							Dir:          moduleDir,
							UpFileName:   moduleUpName,
							DownFileName: moduleDownName,
						}
					} else {
						includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
						moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
						if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						moduleDownName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
						moduleUpPath := filepath.Join(moduleDir, moduleUpName)
						if err := os.WriteFile(moduleUpPath, []byte(fmt.Sprintf("@includes/%s", includeName)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						missedFiles[moduleUpPath] = migrationInfo{
							Dir:          moduleDir,
							UpFileName:   moduleUpName,
							DownFileName: moduleDownName,
						}
					}

				}
				return missedFiles
			},
			wantIncludeLen: 6,
			wantWarnLen:    4,
			wantErr:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missedFiles := tt.setup()

			warnings, rslt, err := checkMissedFilesForIncludes(missedFiles)
			includesLen := 0
			for _, includesMap := range rslt {
				includesLen += len(includesMap)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("checkMissedFilesForIncludes() err = %v, want %v", err, tt.wantErr)
			}
			if includesLen != tt.wantIncludeLen {
				t.Errorf("checkMissedFilesForIncludes() found = %v, want %v", includesLen, tt.wantIncludeLen)
			}
			if len(warnings) != tt.wantWarnLen {
				t.Errorf("checkMissedFilesForIncludes() found = %v, want %v", len(warnings), tt.wantWarnLen)
			}
		})
	}
}

func TestProcessMissedFilesIncludes(t *testing.T) {
	tests := []struct {
		name           string
		setup          func() (map[string]string, map[string]string, map[string]map[string]string)
		wantIncludeLen int
		wantErr        bool
	}{
		{
			name: "normal behaviour",
			setup: func() (map[string]string, map[string]string, map[string]map[string]string) {
				tmpDir, err := os.MkdirTemp("", "test_process_missed_files_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectMD5Includes := make(map[string]string)
				missedIncludes := make(map[string]string)
				newMissedIncludes := make(map[string]map[string]string)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					projectIncludePath := filepath.Join(projectIncludesDir, includeName)
					if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(projectIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					md5, err := fileMD5(projectIncludePath)
					if err != nil {
						t.Fatalf("error calculating md5 of an include file: %v", err)
					}
					projectMD5Includes[md5] = projectIncludePath
					moduleMap := make(map[string]string)
					moduleMap[moduleIncludePath] = moduleUpPath
					newMissedIncludes[moduleUpPath] = moduleMap
					missedIncludes[moduleIncludePath] = "included"
				}
				return projectMD5Includes, missedIncludes, newMissedIncludes
			},
			wantIncludeLen: 0,
			wantErr:        false,
		},
		{
			name: "normal behaviour",
			setup: func() (map[string]string, map[string]string, map[string]map[string]string) {
				tmpDir, err := os.MkdirTemp("", "test_process_missed_files_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectMD5Includes := make(map[string]string)
				missedIncludes := make(map[string]string)
				newMissedIncludes := make(map[string]map[string]string)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					projectIncludePath := filepath.Join(projectIncludesDir, includeName)
					if err := os.WriteFile(moduleIncludePath, []byte(includeName), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					if err := os.WriteFile(projectIncludePath, []byte(fmt.Sprintf("%schanged", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					md5, err := fileMD5(projectIncludePath)
					if err != nil {
						t.Fatalf("error calculating md5 of an include file: %v", err)
					}
					projectMD5Includes[md5] = projectIncludePath
					moduleMap := make(map[string]string)
					moduleMap[moduleIncludePath] = moduleUpPath
					newMissedIncludes[moduleUpPath] = moduleMap
				}
				return projectMD5Includes, missedIncludes, newMissedIncludes
			},
			wantIncludeLen: 10,
			wantErr:        false,
		},
		{
			name: "nonexistent include",
			setup: func() (map[string]string, map[string]string, map[string]map[string]string) {
				tmpDir, err := os.MkdirTemp("", "test_process_missed_files_includes_*")
				if err != nil {
					t.Errorf("failed to created dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				projectMD5Includes := make(map[string]string)
				missedIncludes := make(map[string]string)
				newMissedIncludes := make(map[string]map[string]string)
				projectDir := filepath.Join(tmpDir, "migrations")
				moduleDir := filepath.Join(tmpDir, "module", "migrations")
				projectIncludesDir := filepath.Join(projectDir, "includes")
				moduleIncludesDir := filepath.Join(moduleDir, "includes")
				if err := os.MkdirAll(moduleIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				if err := os.MkdirAll(projectIncludesDir, 0755); err != nil {
					t.Fatalf("error creaing directories: %v", err)
				}
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"
				for i := 1; i <= 10; i++ {
					includeName := fmt.Sprintf("%s%d.sql", baseIncludeName, i-1)
					moduleIncludePath := filepath.Join(moduleIncludesDir, includeName)
					projectIncludePath := filepath.Join(projectIncludesDir, includeName)
					if err := os.WriteFile(projectIncludePath, []byte(fmt.Sprintf("%schanged", includeName)), 0644); err != nil {
						t.Fatalf("error creating file: %v", err)
					}
					moduleUpName := fmt.Sprintf("%s-%d-%d.up.sql", baseModuleName, i, i-1)
					moduleUpPath := filepath.Join(moduleDir, moduleUpName)
					md5, err := fileMD5(projectIncludePath)
					if err != nil {
						t.Fatalf("error calculating md5 of an include file: %v", err)
					}
					projectMD5Includes[md5] = projectIncludePath
					moduleMap := make(map[string]string)
					moduleMap[moduleIncludePath] = moduleUpPath
					newMissedIncludes[moduleUpPath] = moduleMap
					missedIncludes[moduleIncludePath] = "included"
				}
				return projectMD5Includes, missedIncludes, newMissedIncludes
			},
			wantIncludeLen: 0,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectMD5Includes, missedIncludes, newMissedIncludes := tt.setup()

			rslt, err := processMissedFilesIncludes(projectMD5Includes, missedIncludes, newMissedIncludes)
			if (err != nil) != tt.wantErr {
				t.Errorf("processMissedFilesIncludes() err = %v, want %v", err, tt.wantErr)
			}
			if len(rslt) != tt.wantIncludeLen {
				t.Errorf("processMissedFilesIncludes() found = %v, want %v", len(rslt), tt.wantIncludeLen)
			}
		})
	}
}

func TestGetEntriesProjectMap(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() fs.FS
		wantEntryLen int
		wantErr      bool
	}{
		{
			name: "normal",
			setup: func() fs.FS {
				projectDir, err := os.MkdirTemp("", "test_get_entries_project_map_*")
				if err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(projectDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				fsys := os.DirFS(projectDir)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
						projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						projectDownPath := filepath.Join(projectDir, projectDownName)
						if err := os.WriteFile(projectUpPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
						if err := os.WriteFile(projectDownPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
					}
				}
				return fsys
			},
			wantEntryLen: 24,
			wantErr:      false,
		},
		{
			name: "wrong format",
			setup: func() fs.FS {
				projectDir, err := os.MkdirTemp("", "test_get_entries_project_map_*")
				if err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(projectDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				fsys := os.DirFS(projectDir)
				baseProjectName := "test-project-0.1.0"
				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectUpName := fmt.Sprintf("%s-%v-%v.left.sql", baseProjectName, i, j)
						projectDownName := fmt.Sprintf("%s-%v-%v.right.sql", baseProjectName, i, j)
						projectUpPath := filepath.Join(projectDir, projectUpName)
						projectDownPath := filepath.Join(projectDir, projectDownName)
						if err := os.WriteFile(projectUpPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
						if err := os.WriteFile(projectDownPath, []byte(""), 0644); err != nil {
							t.Fatalf("failed to create file: %v", err)
						}
					}
				}
				return fsys
			},
			wantEntryLen: 0,
			wantErr:      false,
		},
		{
			name: "fsys check",
			setup: func() fs.FS {
				fsys := fstest.MapFS{
					"proj1.txt":  {Data: []byte{}},
					"proj2.txt":  {Data: []byte{}},
					"ignore.log": {Data: []byte{}},
				}

				return fsys
			},
			wantEntryLen: 0,
			wantErr:      false,
		},
		{
			name: "fsys check2",
			setup: func() fs.FS {
				fsys := make(fstest.MapFS)
				wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {

						if i == 3 && j == 2 {
							projectUpName := fmt.Sprintf("%s-left.test", wrongProjectName)
							projectDownName := fmt.Sprintf("%s-right.test", wrongProjectName)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
						} else {
							projectUpName := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
							projectDownName := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
							fsys[projectUpName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
							fsys[projectDownName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
						}
					}
				}

				return fsys
			},
			wantEntryLen: 98,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := tt.setup()

			resultMap, err := getEntriesProjectMap(fsys, ".")
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

func TestGetEntriesModuleMap(t *testing.T) {
	tests := []struct {
		name         string
		setup        func() fs.FS
		wantEntryLen int
		wantErr      bool
	}{
		{
			name: "fsys check",
			setup: func() fs.FS {
				fsys := fstest.MapFS{
					".gitmodules":           {Data: []byte("[submodule \"module\"]\n\tpath = module\n\turl = ./module")},
					"migrations/proj1.txt":  {Data: []byte{}},
					"migrations/proj2.txt":  {Data: []byte{}},
					"migrations/ignore.log": {Data: []byte{}},
				}

				return fsys
			},
			wantEntryLen: 0,
			wantErr:      false,
		},
		{
			name: "fsys check2",
			setup: func() fs.FS {
				fsys := make(fstest.MapFS)
				wrongProjectName := "1test23"
				baseProjectName := "test-project-0.1.0"
				moduleDir := "module/migrations"
				gitmodulesText := "[submodule \"module\"]\n\tpath = module\n\turl = ./module"
				fsys[".gitmodules"] = &fstest.MapFile{Data: []byte(gitmodulesText), Mode: 0644}
				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						if i == 3 && j == 2 {
							moduleUpName := fmt.Sprintf("%s/%s-left.test", moduleDir, wrongProjectName)
							moduleDownName := fmt.Sprintf("%s/%s-right.test", moduleDir, wrongProjectName)
							fsys[moduleUpName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
							fsys[moduleDownName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
						} else {
							moduleUpName := fmt.Sprintf("%s/%s-%v-%v.up.sql", moduleDir, baseProjectName, i, j)
							moduleDownName := fmt.Sprintf("%s/%s-%v-%v.down.sql", moduleDir, baseProjectName, i, j)
							fsys[moduleUpName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
							fsys[moduleDownName] = &fstest.MapFile{Data: []byte(""), Mode: 0644}
						}
					}
				}

				return fsys
			},
			wantEntryLen: 98,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := tt.setup()

			resultMap, err := getEntriesModuleMap(fsys, ".")
			if (err != nil) != tt.wantErr {
				t.Errorf("getEntriesModuleMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantEntryLen {
				t.Errorf("getEntriesModuleMap() found = %v, want %v", len(resultMap), tt.wantEntryLen)
			}
		})
	}
}
