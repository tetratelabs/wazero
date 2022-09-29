package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/fs
var testFS embed.FS

func TestCompositeFS(t *testing.T) {
	tests := []struct {
		name    string
		fs      *compositeFS
		path    string
		content string
	}{
		{
			name: "empty",
			fs:   &compositeFS{},
			path: "bear.txt",
		},
		{
			name: "single mount to root",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/": testFSSub(""),
				},
			},
			path:    "/bear.txt",
			content: "pooh",
		},
		{
			name: "single mount to root",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/": testFSSub(""),
				},
			},
			path: "bear.txt",
		},
		{
			name: "single mount to root",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/": testFSSub(""),
				},
			},
			path:    "/fish/clownfish.txt",
			content: "nemo",
		},
		{
			name: "single mount to root",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/": testFSSub(""),
				},
			},
			path:    "/mammals/primates/ape.txt",
			content: "king kong",
		},
		{
			name: "single mount to path",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/mammals": testFSSub(""),
				},
			},
			path:    "/mammals/bear.txt",
			content: "pooh",
		},
		{
			name: "single mount to path",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/mammals": testFSSub(""),
				},
			},
			path: "/mammals/whale.txt",
		},
		{
			name: "single mount to path",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/mammals": testFSSub(""),
				},
			},
			path: "/bear.txt",
		},
		{
			name: "non-overlapping mounts",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/fish":    testFSSub("fish"),
					"/mammals": testFSSub("mammals"),
				},
			},
			path:    "/fish/clownfish.txt",
			content: "nemo",
		},
		{
			name: "non-overlapping mounts",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/fish":    testFSSub("fish"),
					"/mammals": testFSSub("mammals"),
				},
			},
			path:    "/mammals/whale.txt",
			content: "moby dick",
		},
		{
			name: "non-overlapping mounts",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/fish":    testFSSub("fish"),
					"/mammals": testFSSub("mammals"),
				},
			},
			path:    "/mammals/primates/ape.txt",
			content: "king kong",
		},
		{
			name: "non-overlapping mounts",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/fish":    testFSSub("fish"),
					"/mammals": testFSSub("mammals"),
				},
			},
			path: "/bear.txt",
		},
		{
			name: "overlapping mounts, deep first",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/animals/fish": testFSSub("fish"),
					"/animals":      testFSSub("mammals"),
				},
			},
			path:    "/animals/fish/clownfish.txt",
			content: "nemo",
		},
		{
			name: "overlapping mounts, deep first",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/animals/fish": testFSSub("fish"),
					"/animals":      testFSSub("mammals"),
				},
			},
			path:    "/animals/whale.txt",
			content: "moby dick",
		},
		{
			name: "overlapping mounts, deep first",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/animals/fish": testFSSub("fish"),
					"/animals":      testFSSub("mammals"),
				},
			},
			path: "/animals/bear.txt",
		},
		{
			name: "overlapping mounts, shallow first",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/animals":      testFSSub("mammals"),
					"/animals/fish": testFSSub("fish"),
				},
			},
			path:    "/animals/fish/clownfish.txt",
			content: "nemo",
		},
		{
			name: "overlapping mounts, shallow first",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/animals":      testFSSub("mammals"),
					"/animals/fish": testFSSub("fish"),
				},
			},
			path:    "/animals/whale.txt",
			content: "moby dick",
		},
		{
			name: "overlapping mounts, shallow first",
			fs: &compositeFS{
				paths: map[string]fs.FS{
					"/animals":      testFSSub("mammals"),
					"/animals/fish": testFSSub("fish"),
				},
			},
			path: "/animals/bear.txt",
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(fmt.Sprintf("%s - %s", tt.name, tt.path), func(t *testing.T) {
			content, err := fs.ReadFile(tt.fs, tt.path)
			if tt.content != "" {
				require.NoError(t, err)
				require.Equal(t, tt.content, string(bytes.TrimSpace(content)))
			} else {
				require.ErrorIs(t, err, fs.ErrNotExist)
			}
		})
	}
}

func testFSSub(path string) fs.FS {
	f, _ := fs.Sub(testFS, filepath.Join("testdata", "fs", path))
	return f
}
