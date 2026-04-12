package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
		_ = os.Chdir(oldWD)
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
		{
			name: "missing include",
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
				for i := 1; i < 6; i++ {
					includePath := filepath.Join(includeDirPath, fmt.Sprintf("%s_%v.txt", baseIncludeName, i))
					if i != 4 {
						os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i+1)), 0644)
					} else {
						os.WriteFile(includePath, []byte(""), 0644)
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
						os.WriteFile(includePath, []byte(fmt.Sprintf("@%s_%v.txt", baseIncludeName, i-2)), 0644)
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

func TestMigrationList(t *testing.T) {
	tests := []struct {
		name                string
		setup               func(t *testing.T) string
		wantDeletedFiles    int
		wantDeletedIncludes int
		wantMissedIncludes  int
		wantMissedFiles     int
		wantLostPairs       int
		wantErr             bool
	}{
		{
			name: "normal behaviour",
			setup: func(t *testing.T) string {
				migrationDir, err := os.MkdirTemp("", "test_migration_list_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(migrationDir)
				})
				testChdirRepo(t, migrationDir)

				projectDir := filepath.Join(migrationDir, "migrations")
				projectIncludes := filepath.Join(projectDir, "includes")
				moduleDir := filepath.Join(migrationDir, "module")
				moduleMigrationsDir := filepath.Join(moduleDir, "migrations")
				moduleIncludes := filepath.Join(moduleMigrationsDir, "includes")

				gitmodules := filepath.Join(migrationDir, ".gitmodules")
				os.WriteFile(gitmodules, []byte("[submodule \"module\"]\n\tpath = module\n\turl = ./module"), 0644)

				os.Mkdir(projectDir, 0755)
				os.Mkdir(projectIncludes, 0755)
				os.Mkdir(moduleDir, 0755)
				os.Mkdir(moduleMigrationsDir, 0755)
				os.Mkdir(moduleIncludes, 0755)

				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"

				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
						projectNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
						projectFileUp := filepath.Join(projectDir, projectNameUp)
						projectFileDown := filepath.Join(projectDir, projectNameDown)

						moduleNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
						moduleNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseModuleName, i, j)
						moduleFileUp := filepath.Join(moduleMigrationsDir, moduleNameUp)
						moduleFileDown := filepath.Join(moduleMigrationsDir, moduleNameDown)

						if i == 3 && j == 2 {
							includeName1 := fmt.Sprintf("%s_1.txt", baseIncludeName)
							includeName2 := fmt.Sprintf("%s_2.txt", baseIncludeName)
							moduleIncludeFile1 := filepath.Join(moduleIncludes, includeName1)
							moduleIncludeFile2 := filepath.Join(moduleIncludes, includeName2)
							projectIncludeFile1 := filepath.Join(projectIncludes, includeName1)
							projectIncludeFile2 := filepath.Join(projectIncludes, includeName2)

							os.WriteFile(moduleIncludeFile1, []byte("1"), 0644)
							os.WriteFile(moduleIncludeFile2, []byte("2"), 0644)
							os.WriteFile(projectIncludeFile1, []byte("1"), 0644)
							os.WriteFile(projectIncludeFile2, []byte("2"), 0644)

							os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s\n@includes/%s", moduleNameUp, includeName1)), 0644)
							os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s\n@includes/%s", moduleNameDown, includeName2)), 0644)

							md5up, _ := FileMD5(moduleFileUp)
							md5down, _ := FileMD5(moduleFileDown)
							moduleMD5 := md5up + md5down

							relativeModuleFileUp := testMetaPathForModuleFile(moduleNameUp)
							relativeModuleFileDown := testMetaPathForModuleFile(moduleNameDown)

							os.WriteFile(projectFileUp, []byte(fmt.Sprintf("# %s\n#migration: %s;%s\n@includes/%s", projectNameUp, relativeModuleFileUp, moduleMD5, includeName1)), 0644)
							os.WriteFile(projectFileDown, []byte(fmt.Sprintf("# %s\n#migration: %s;%s\n@includes/%s", projectNameDown, relativeModuleFileDown, moduleMD5, includeName2)), 0644)
						} else if i == 2 && j == 3 {
							continue
						} else {
							os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s", moduleNameUp)), 0644)
							os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s", moduleNameDown)), 0644)

							md5up, _ := FileMD5(moduleFileUp)
							md5down, _ := FileMD5(moduleFileDown)
							moduleMD5 := md5up + md5down

							relativeModuleFileUp := testMetaPathForModuleFile(moduleNameUp)
							relativeModuleFileDown := testMetaPathForModuleFile(moduleNameDown)

							os.WriteFile(projectFileUp, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameUp, relativeModuleFileUp, moduleMD5)), 0644)
							os.WriteFile(projectFileDown, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameDown, relativeModuleFileDown, moduleMD5)), 0644)
						}
					}
				}
				return "migrations"
			},
			wantDeletedFiles:    0,
			wantDeletedIncludes: 0,
			wantMissedIncludes:  0,
			wantMissedFiles:     0,
			wantLostPairs:       0,
			wantErr:             false,
		},
		{
			name: "missed files & includes",
			setup: func(t *testing.T) string {
				migrationDir, err := os.MkdirTemp("", "test_migration_list_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(migrationDir)
				})
				testChdirRepo(t, migrationDir)

				projectDir := filepath.Join(migrationDir, "migrations")
				projectIncludes := filepath.Join(projectDir, "includes")
				moduleDir := filepath.Join(migrationDir, "module")
				moduleMigrationsDir := filepath.Join(moduleDir, "migrations")
				moduleIncludes := filepath.Join(moduleMigrationsDir, "includes")

				gitmodules := filepath.Join(migrationDir, ".gitmodules")
				os.WriteFile(gitmodules, []byte("[submodule \"module\"]\n\tpath = module\n\turl = ./module"), 0644)

				os.Mkdir(projectDir, 0755)
				os.Mkdir(projectIncludes, 0755)
				os.Mkdir(moduleDir, 0755)
				os.Mkdir(moduleMigrationsDir, 0755)
				os.Mkdir(moduleIncludes, 0755)

				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"
				baseIncludeName := "include"

				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
						projectNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
						projectFileUp := filepath.Join(projectDir, projectNameUp)
						projectFileDown := filepath.Join(projectDir, projectNameDown)

						moduleNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
						moduleNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseModuleName, i, j)
						moduleFileUp := filepath.Join(moduleMigrationsDir, moduleNameUp)
						moduleFileDown := filepath.Join(moduleMigrationsDir, moduleNameDown)

						if i == 3 && j == 2 {
							includeName1 := fmt.Sprintf("%s_1.txt", baseIncludeName)
							includeName2 := fmt.Sprintf("%s_2.txt", baseIncludeName)
							moduleIncludeFile1 := filepath.Join(moduleIncludes, includeName1)
							moduleIncludeFile2 := filepath.Join(moduleIncludes, includeName2)
							// projectIncludeFile1 := filepath.Join(projectIncludes, includeName1)
							// projectIncludeFile2 := filepath.Join(projectIncludes, includeName2)

							os.WriteFile(moduleIncludeFile1, []byte("1"), 0644)
							os.WriteFile(moduleIncludeFile2, []byte("2"), 0644)
							// os.WriteFile(projectIncludeFile1, []byte("1"), 0644)
							// os.WriteFile(projectIncludeFile2, []byte("2"), 0644)

							os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s\n@includes/%s", moduleNameUp, includeName1)), 0644)
							os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s\n@includes/%s", moduleNameDown, includeName2)), 0644)

							// md5up, _ := FileMD5(moduleFileUp)
							// md5down, _ := FileMD5(moduleFileDown)
							// moduleMD5 := md5up + md5down

							// relativeModuleFileUp := StripDir(moduleFileUp)
							// relativeModuleFileDown := StripDir(moduleFileDown)

							// os.WriteFile(projectFileUp, []byte(fmt.Sprintf("# %s\n#migration: %s;%s\n@includes/%s", projectNameUp, relativeModuleFileUp, moduleMD5, includeName1)), 0644)
							// os.WriteFile(projectFileDown, []byte(fmt.Sprintf("# %s\n#migration: %s;%s\n@includes/%s", projectNameDown, relativeModuleFileDown, moduleMD5, includeName2)), 0644)
						} else if i == 2 && j == 3 {
							continue
						} else {
							os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s", moduleNameUp)), 0644)
							os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s", moduleNameDown)), 0644)

							md5up, _ := FileMD5(moduleFileUp)
							md5down, _ := FileMD5(moduleFileDown)
							moduleMD5 := md5up + md5down

							relativeModuleFileUp := testMetaPathForModuleFile(moduleNameUp)
							relativeModuleFileDown := testMetaPathForModuleFile(moduleNameDown)

							os.WriteFile(projectFileUp, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameUp, relativeModuleFileUp, moduleMD5)), 0644)
							os.WriteFile(projectFileDown, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameDown, relativeModuleFileDown, moduleMD5)), 0644)
						}
					}
				}
				return "migrations"
			},
			wantDeletedFiles:    0,
			wantDeletedIncludes: 0,
			wantMissedIncludes:  2, // two @includes in project copy without matching files on disk
			wantMissedFiles:     2,
			wantLostPairs:       0,
			wantErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrationRelDir := tt.setup(t)

			rslts := &ListResults{}

			err := MigrationList(migrationRelDir, rslts)
			if (err != nil) != tt.wantErr {
				t.Errorf("MigrationList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if rslts.DeletedFilesCnt != tt.wantDeletedFiles {
				// for foo, bar := range rslts.DeletedFiles {
				// 	t.Errorf("project %s module %s", foo, bar)
				// }
				t.Errorf("MigrationList() DeletedFilesCnt = %v, want %v", rslts.DeletedFilesCnt, tt.wantDeletedFiles)
			}
			if rslts.DeletedIncludesCnt != tt.wantDeletedIncludes {
				// for foo, bar := range rslts.DeletedIncludes {
				// 	t.Errorf("include %s included by %s", foo, bar)
				// }
				t.Errorf("MigrationList() DeletedIncludesCnt = %v, want %v", rslts.DeletedIncludesCnt, tt.wantDeletedIncludes)
			}
			if rslts.MissedIncludesCnt != tt.wantMissedIncludes {
				t.Errorf("MigrationList() MissedIncludesCnt = %v, want %v", rslts.MissedIncludesCnt, tt.wantMissedIncludes)
			}
			if rslts.MissedFilesCnt != tt.wantMissedFiles {
				t.Errorf("MigrationList() MissedFilesCnt = %v, want %v", rslts.MissedFilesCnt, tt.wantMissedFiles)
			}
			if len(rslts.LostPairs) != tt.wantLostPairs {
				t.Errorf("MigrationList() LostPairsCnt = %v, want %v", len(rslts.LostPairs), tt.wantLostPairs)
			}
		})
	}
}
