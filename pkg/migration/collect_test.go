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
				return "migrations"
			},
			wantCollectedCnt: 0,
			wantProjectErr:   false,
			wantErr:          false,
		},
		{
			name: "2 missing files",
			setup: func(t *testing.T) string {
				migrationDir, err := os.MkdirTemp("", "test_migration_list_*")
				if err != nil {
					t.Fatalf("Failed to create dir: %v", err)
				}
				t.Cleanup(func() {
					os.RemoveAll(migrationDir)
				})
				testChdirRepo(t, migrationDir)

				// need to improvise with .git dir

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

							// relativeModuleFileUp := testMetaPathForModuleFile(moduleNameUp)
							// relativeModuleFileDown := testMetaPathForModuleFile(moduleNameDown)

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
			wantCollectedCnt: 2,
			wantProjectErr:   false,
			wantErr:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrationRelDir := tt.setup(t)

			rslts := &ListResults{}
			MigrationList(migrationRelDir, rslts)

			collected, err := MissedFiles(rslts)
			if (err != nil) != tt.wantErr {
				t.Errorf("MissingFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (len(rslts.MissedFiles) != 0) != tt.wantProjectErr {
				t.Errorf("MissedFiles() found = %v, want %v", (len(rslts.MissedFiles) != 0), tt.wantProjectErr)
			}
			if collected != tt.wantCollectedCnt {
				t.Errorf("MissedFiles() found = %v, want %v", collected, tt.wantCollectedCnt)
			}
		})
	}
}
