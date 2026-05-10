package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFileViaDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_find_file_*")
	if err != nil {
		t.Errorf("Failed to created dir: %v", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil && err == nil {
			err = removeErr
		}
	}()

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
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				return dir
			},
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "existing file",
			setup: func(tmpDir string) string {
				file := filepath.Join(tmpDir, "test.txt")
				if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
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
			setup: func(_ string) string {
				return ""
			},
			wantFound: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPath := tt.setup(tmpDir)

			found, err := findFileViaDir(testPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("findFileViaDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if found != tt.wantFound {
				t.Errorf("findFileViaDir() found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestFileMD5(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_file_md5_*")
	if err != nil {
		t.Errorf("Failed to created dir: %v", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil && err == nil {
			err = removeErr
		}
	}()

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
				if err := os.WriteFile(file, []byte(text), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
				return file
			},
			wantMD5: "fecdaaa968e07e70c5e2cdae6e03a836",
			wantErr: false,
		},
		{
			name: "empty file",
			setup: func(tmpDir string) string {
				file := filepath.Join(tmpDir, "test.txt")
				if err := os.WriteFile(file, []byte(""), 0644); err != nil {
					t.Fatalf("error creating file: %v", err)
				}
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
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatalf("error creating dir: %v", err)
				}
				return dir
			},
			wantMD5: "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPath := tt.setup(tmpDir)

			resultMD5, err := fileMD5(testPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("fileMD5() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if resultMD5 != tt.wantMD5 {
				t.Errorf("fileMD5() resultMD5 = %v, want %v", resultMD5, tt.wantMD5)
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
			result := stripDir(tt.tempDir)

			if result != tt.wantResult {
				t.Errorf("stripDir() result = %v, want %v", result, tt.wantResult)
			}
		})
	}
}
