package migration

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestGetModuleDir(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) fs.FS
		wantEntryLen int
		wantErr      bool
	}{
		{
			name: "fsys check",
			setup: func(_ *testing.T) fs.FS {
				fsys := make(fstest.MapFS)
				var builder strings.Builder

				for i := 1; i <= 10; i++ {
					for j := 1; j <= 5; j++ {
						fmt.Fprintf(&builder, "[submodule \"module%d-%d\"]\n\tpath = module%d-%d\n\turl = ./module%d-%d", i, j, i, j, i, j)
					}
				}

				gitmodulesText := builder.String()
				fsys[".gitmodules"] = &fstest.MapFile{Data: []byte(gitmodulesText), Mode: 0644}

				return fsys
			},
			wantEntryLen: 50,
			wantErr:      false,
		},
		{
			name: "fsys check2",
			setup: func(_ *testing.T) fs.FS {
				fsys := make(fstest.MapFS)
				gitmodulesText := "[submodule \"module\"]\n\tpath = module\n\turl = ./module"
				fsys[".gitmodules"] = &fstest.MapFile{Data: []byte(gitmodulesText), Mode: 0644}

				return fsys
			},
			wantEntryLen: 1,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := tt.setup(t)

			resultMap, err := getModuleDir(fsys, ".")
			if (err != nil) != tt.wantErr {
				t.Errorf("getModuleDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(resultMap) != tt.wantEntryLen {
				t.Errorf("getModuleDir() found = %v, want %v", len(resultMap), tt.wantEntryLen)
			}
		})
	}
}
