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
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				dstFileName := "newfile.txt"
				srcFileName := "source.txt"
				srcFileText := ""
				srcFileHeader := "23"

				dstFilePath := filepath.Join(tmpDir, dstFileName)
				srcFilePath := filepath.Join(tmpDir, srcFileName)
				if err := os.WriteFile(dstFilePath, []byte(""), 0644); err != nil {
					t.Fatalf("failed to create dstFile: %v", err)
				}
				if err := os.WriteFile(srcFilePath, []byte(srcFileText), 0644); err != nil {
					t.Fatalf("failed to create srcFile: %v", err)
				}

				return dstFilePath, srcFilePath, srcFileHeader
			},
			wantExists: true,
			wantHeader: "23",
			wantErr:    false,
		},
		{
			name: "nonexistent dstFile",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				dstFileName := "newfile.txt"
				srcFileName := "source.txt"
				srcFileText := ""
				srcFileHeader := "23"

				dstFilePath := filepath.Join(tmpDir, dstFileName)
				srcFilePath := filepath.Join(tmpDir, srcFileName)
				// if err := os.WriteFile(dstFilePath, []byte{}, 0644); err != nil {
				// 	t.Fatalf("failed to create dstFile: %v", err)
				// }
				if err := os.WriteFile(srcFilePath, []byte(srcFileText), 0644); err != nil {
					t.Fatalf("failed to create srcFile: %v", err)
				}

				return dstFilePath, srcFilePath, srcFileHeader
			},
			wantExists: false,
			wantHeader: "",
			wantErr:    true,
		},
		{
			name: "nonexistent srcFile",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir, err := os.MkdirTemp("", "test_parse_includes_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				dstFileName := "newfile.txt"
				srcFileName := "source.txt"
				// srcFileText := ""
				srcFileHeader := "23"

				dstFilePath := filepath.Join(tmpDir, dstFileName)
				srcFilePath := filepath.Join(tmpDir, srcFileName)
				if err := os.WriteFile(dstFilePath, []byte{}, 0644); err != nil {
					t.Fatalf("failed to create dstFile: %v", err)
				}
				// if err := os.WriteFile(srcFilePath, []byte(srcFileText), 0644); err != nil {
				// t.Fatalf("failed to create srcFile: %v", err)
				// }

				return dstFilePath, srcFilePath, srcFileHeader
			},
			wantExists: true,
			wantHeader: "",
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dstFile, srcFile, header := tt.setup(t)

			err := appendToFrom(dstFile, srcFile, header)
			if (err != nil) != tt.wantErr {
				t.Errorf("appendToFrom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fileHeader, err := os.ReadFile(dstFile)
			if err != nil {
				if !os.IsNotExist(err) {
					t.Errorf("error reading file: %v", err)
				}
			}
			var existsFlag bool
			existsFlag, err = findFileViaDir(dstFile)
			if err != nil {
				t.Errorf("error FindFileViaDir: %v", err)
				return
			}

			if existsFlag != tt.wantExists {
				t.Errorf("appendToFrom() found = %v, want %v", existsFlag, tt.wantExists)
			}
			if string(fileHeader) != tt.wantHeader {
				t.Errorf("appendToFrom() found = %v, want %v", string(fileHeader), tt.wantHeader)
			}
		})
	}
}

func TestMissedFiles(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(t *testing.T) string
		wantCollectedCnt int
		wantProjectErr   bool
		wantErr          bool
	}{
		{
			name: "nothing to collect",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_missed_files_*")
				if err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				testChdirRepo(t, tmpDir)

				projectMigrationsDir := filepath.Join(tmpDir, "migrations")
				projectIncludes := filepath.Join(projectMigrationsDir, "includes")
				moduleDir := filepath.Join(tmpDir, "module")
				moduleMigrationsDir := filepath.Join(moduleDir, "migrations")
				moduleIncludes := filepath.Join(moduleMigrationsDir, "includes")

				gitmodules := filepath.Join(tmpDir, ".gitmodules")
				if err := os.WriteFile(gitmodules, []byte("[submodule \"module\"]\n\tpath = module\n\turl = ./module"), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				if err := os.MkdirAll(projectIncludes, 0755); err != nil {
					t.Fatalf("eror creating directory: %v", err)
				}
				if err := os.MkdirAll(moduleIncludes, 0755); err != nil {
					t.Fatalf("eror creating directory: %v", err)
				}

				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"

				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
						projectNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
						projectFileUp := filepath.Join(projectMigrationsDir, projectNameUp)
						projectFileDown := filepath.Join(projectMigrationsDir, projectNameDown)

						moduleNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
						moduleNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseModuleName, i, j)
						moduleFileUp := filepath.Join(moduleMigrationsDir, moduleNameUp)
						moduleFileDown := filepath.Join(moduleMigrationsDir, moduleNameDown)

						if err := os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s", moduleNameUp)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						if err := os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s", moduleNameDown)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}

						concatMD5, err := concatMD5(moduleFileUp, moduleFileDown)
						if err != nil {
							t.Fatalf("error getting concat md5 of files: %v", err)
						}

						relativeModuleFileUp := testMetaPathForModuleFile(moduleNameUp)
						relativeModuleFileDown := testMetaPathForModuleFile(moduleNameDown)

						if err := os.WriteFile(projectFileUp, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameUp, relativeModuleFileUp, concatMD5)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}
						if err := os.WriteFile(projectFileDown, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameDown, relativeModuleFileDown, concatMD5)), 0644); err != nil {
							t.Fatalf("error creating file: %v", err)
						}

					}
				}
				return "migrations"
			},
			wantCollectedCnt: 0,
			wantProjectErr:   false,
			wantErr:          false,
		},
		{
			name: "2 missing files",
			setup: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "test_missed_files_*")
				if err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					if err := os.RemoveAll(tmpDir); err != nil {
						t.Fatalf("failed to remove temp dir: %v", err)
					}
				})
				testChdirRepo(t, tmpDir)

				projectMigrationsDir := filepath.Join(tmpDir, "migrations")
				projectIncludes := filepath.Join(projectMigrationsDir, "includes")
				moduleDir := filepath.Join(tmpDir, "module")
				moduleMigrationsDir := filepath.Join(moduleDir, "migrations")
				moduleIncludes := filepath.Join(moduleMigrationsDir, "includes")

				gitmodules := filepath.Join(tmpDir, ".gitmodules")
				if err := os.WriteFile(gitmodules, []byte("[submodule \"module\"]\n\tpath = module\n\turl = ./module"), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				if err := os.MkdirAll(projectIncludes, 0755); err != nil {
					t.Fatalf("eror creating directory: %v", err)
				}
				if err := os.MkdirAll(moduleIncludes, 0755); err != nil {
					t.Fatalf("eror creating directory: %v", err)
				}

				baseProjectName := "test-project-0.1.0"
				baseModuleName := "test-module-0.1.0"

				for i := 1; i < 5; i++ {
					for j := 1; j < 4; j++ {
						projectNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseProjectName, i, j)
						projectNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseProjectName, i, j)
						projectFileUp := filepath.Join(projectMigrationsDir, projectNameUp)
						projectFileDown := filepath.Join(projectMigrationsDir, projectNameDown)

						moduleNameUp := fmt.Sprintf("%s-%v-%v.up.sql", baseModuleName, i, j)
						moduleNameDown := fmt.Sprintf("%s-%v-%v.down.sql", baseModuleName, i, j)
						moduleFileUp := filepath.Join(moduleMigrationsDir, moduleNameUp)
						moduleFileDown := filepath.Join(moduleMigrationsDir, moduleNameDown)
						switch {
						case i == 3 && j == 2:
							if err := os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s", moduleNameUp)), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
							if err := os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s", moduleNameDown)), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
						case i == 2 && j == 3:
							continue
						default:
							if err := os.WriteFile(moduleFileUp, []byte(fmt.Sprintf("# %s", moduleNameUp)), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
							if err := os.WriteFile(moduleFileDown, []byte(fmt.Sprintf("# %s", moduleNameDown)), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}

							concatMD5, err := concatMD5(moduleFileUp, moduleFileDown)
							if err != nil {
								t.Fatalf("error getting concat md5 of files: %v", err)
							}

							relativeModuleFileUp := testMetaPathForModuleFile(moduleNameUp)
							relativeModuleFileDown := testMetaPathForModuleFile(moduleNameDown)

							if err := os.WriteFile(projectFileUp, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameUp, relativeModuleFileUp, concatMD5)), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
							if err := os.WriteFile(projectFileDown, []byte(fmt.Sprintf("# %s\n#migration: %s;%s", projectNameDown, relativeModuleFileDown, concatMD5)), 0644); err != nil {
								t.Fatalf("error creating file: %v", err)
							}
						}
					}
				}
				return "migrations"
			},
			wantCollectedCnt: 2,
			wantProjectErr:   true,
			wantErr:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			savedMigrationDir := MigrationDir
			t.Cleanup(func() { MigrationDir = savedMigrationDir })
			migrationRelDir := tt.setup(t)
			MigrationDir = migrationRelDir
			fsys := os.DirFS(".")
			rslts, err := migrationList(fsys, migrationRelDir)
			if err != nil {
				t.Errorf("migrationList failed: %v", err)
				return
			}
			collected, err := missedFiles(rslts.MissedFiles, mockVersionGetter{})
			if (err != nil) != tt.wantErr {
				t.Errorf("missedFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (len(rslts.MissedFiles) != 0) != tt.wantProjectErr {
				t.Errorf("missedFiles() found = %v, want %v", (len(rslts.MissedFiles) != 0), tt.wantProjectErr)
			}
			if collected != tt.wantCollectedCnt {
				t.Errorf("missedFiles() found = %v, want %v", collected, tt.wantCollectedCnt)
			}
		})
	}
}

type mockVersionGetter struct{}

func (r mockVersionGetter) GetProjectFromGit(dir string) (string, error) {
	return "test-project", nil
}

func (r mockVersionGetter) GetVersion(dir string) (string, error) {
	return "0.1.0", nil
}

func (r mockVersionGetter) GetRelease() string {
	return "1"
}

func (r mockVersionGetter) GetFull(dir string) (string, error) {
	return "test-project-0.1.0-1", nil
}
