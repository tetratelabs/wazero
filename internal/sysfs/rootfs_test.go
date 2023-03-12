package sysfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"syscall"
	"testing"
	gofstest "testing/fstest"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewRootFS(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		rootFS, err := NewRootFS(nil, nil)
		require.NoError(t, err)

		require.Equal(t, UnimplementedFS{}, rootFS)
	})
	t.Run("only root", func(t *testing.T) {
		testFS := NewDirFS(t.TempDir())

		rootFS, err := NewRootFS([]FS{testFS}, []string{""})
		require.NoError(t, err)

		// Should not be a composite filesystem
		require.Equal(t, testFS, rootFS)
	})
	t.Run("only non root", func(t *testing.T) {
		testFS := NewDirFS(".")

		rootFS, err := NewRootFS([]FS{testFS}, []string{"/tmp"})
		require.NoError(t, err)

		// unwrapping returns in original order
		require.Equal(t, []FS{testFS}, rootFS.(*CompositeFS).FS())
		require.Equal(t, []string{"/tmp"}, rootFS.(*CompositeFS).GuestPaths())

		// String is human-readable
		require.Equal(t, "[.:/tmp]", rootFS.String())

		// Guest can look up /tmp
		f, err := rootFS.OpenFile("/tmp", os.O_RDONLY, 0)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Guest can look up / and see "/tmp" in it
		f, err = rootFS.OpenFile("/", os.O_RDONLY, 0)
		require.NoError(t, err)
		dirents, err := f.(fs.ReadDirFile).ReadDir(-1)
		require.NoError(t, err)
		require.Equal(t, 1, len(dirents))
		require.Equal(t, "tmp", dirents[0].Name())
		require.True(t, dirents[0].IsDir())
	})
	t.Run("multiple roots unsupported", func(t *testing.T) {
		testFS := NewDirFS(".")

		_, err := NewRootFS([]FS{testFS, testFS}, []string{"/", "/"})
		require.EqualError(t, err, "multiple root filesystems are invalid: [.:/ .:/]")
	})
	t.Run("virtual paths unsupported", func(t *testing.T) {
		testFS := NewDirFS(".")

		_, err := NewRootFS([]FS{testFS}, []string{"usr/bin"})
		require.EqualError(t, err, "only single-level guest paths allowed: [.:usr/bin]")
	})
	t.Run("multiple matches", func(t *testing.T) {
		tmpDir1 := t.TempDir()
		testFS1 := NewDirFS(tmpDir1)
		require.NoError(t, os.Mkdir(joinPath(tmpDir1, "tmp"), 0o700))
		require.NoError(t, os.WriteFile(joinPath(tmpDir1, "a"), []byte{1}, 0o600))

		tmpDir2 := t.TempDir()
		testFS2 := NewDirFS(tmpDir2)
		require.NoError(t, os.WriteFile(joinPath(tmpDir2, "a"), []byte{2}, 0o600))

		rootFS, err := NewRootFS([]FS{testFS2, testFS1}, []string{"/tmp", "/"})
		require.NoError(t, err)

		// unwrapping returns in original order
		require.Equal(t, []FS{testFS2, testFS1}, rootFS.(*CompositeFS).FS())
		require.Equal(t, []string{"/tmp", "/"}, rootFS.(*CompositeFS).GuestPaths())

		// Should be a composite filesystem
		require.NotEqual(t, testFS1, rootFS)
		require.NotEqual(t, testFS2, rootFS)

		t.Run("last wins", func(t *testing.T) {
			f, err := rootFS.OpenFile("/tmp/a", os.O_RDONLY, 0)
			require.NoError(t, err)
			defer f.Close()
			b, err := io.ReadAll(f)
			require.NoError(t, err)
			require.Equal(t, []byte{2}, b)
		})

		// This test is covered by fstest.TestFS, but doing again here
		t.Run("root includes prefix mount", func(t *testing.T) {
			f, err := rootFS.OpenFile(".", os.O_RDONLY, 0)
			require.NoError(t, err)
			defer f.Close()

			require.Equal(t, []string{"a", "tmp"}, readDirNames(t, f))
		})
	})
}

func readDirNames(t *testing.T, f fs.File) []string {
	names, err := platform.Readdirnames(f, -1)
	require.NoError(t, err)
	sort.Strings(names)
	return names
}

func TestRootFS_String(t *testing.T) {
	tmpFS := NewDirFS(".")
	rootFS := NewDirFS(".")

	testFS, err := NewRootFS([]FS{rootFS, tmpFS}, []string{"/", "/tmp"})
	require.NoError(t, err)

	require.Equal(t, "[.:/ .:/tmp]", testFS.String())
}

func TestRootFS_Open(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the FS root.
	tmpDir = joinPath(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	testRootFS := NewDirFS(tmpDir)
	testDirFS := NewDirFS(t.TempDir())
	testFS, err := NewRootFS([]FS{testRootFS, testDirFS}, []string{"/", "/emptydir"})
	require.NoError(t, err)

	testOpen_Read(t, testFS, true)

	testOpen_O_RDWR(t, tmpDir, testFS)

	t.Run("path outside root valid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// syscall.FS allows relative path lookups
		require.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestRootFS_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	tmpFS := NewDirFS(t.TempDir())
	testFS, err := NewRootFS([]FS{NewDirFS(tmpDir), tmpFS}, []string{"/", "/tmp"})
	require.NoError(t, err)
	testStat(t, testFS)
}

func TestRootFS_TestFS(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir1 := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir1))

	// move one directory outside the other
	tmpDir2 := t.TempDir()
	require.NoError(t, os.Rename(joinPath(tmpDir1, "dir"), joinPath(tmpDir2, "dir")))

	// Create a root mount
	testFS1 := NewDirFS(tmpDir1)

	// Create a dir mount
	testFS2 := NewDirFS(joinPath(tmpDir2, "dir"))

	testFS, err := NewRootFS([]FS{testFS1, testFS2}, []string{"/", "/dir"})
	require.NoError(t, err)

	// Run TestFS via the adapter
	require.NoError(t, fstest.TestFS(testFS.(fs.FS)))
}

func TestRootFS_examples(t *testing.T) {
	tests := []struct {
		name                 string
		fs                   []FS
		guestPaths           []string
		expected, unexpected []string
	}{
		// e.g. from Go project root:
		//	$ GOOS=js GOARCH=wasm bin/go test -c -o template.wasm text/template
		//	$ wazero run -mount=src/text/template:/ template.wasm -test.v
		{
			name: "go test text/template",
			fs: []FS{
				&adapter{fs: testfs.FS{"go-example-stdout-ExampleTemplate-0.txt": &testfs.File{}}},
				&adapter{fs: testfs.FS{"testdata/file1.tmpl": &testfs.File{}}},
			},
			guestPaths: []string{"/tmp", "/"},
			expected:   []string{"/tmp/go-example-stdout-ExampleTemplate-0.txt", "testdata/file1.tmpl"},
			unexpected: []string{"DOES NOT EXIST"},
		},
		// e.g. from TinyGo project root:
		//	$ ./build/tinygo test -target wasi -c -o flate.wasm compress/flate
		//	$ wazero run -mount=$(go env GOROOT)/src/compress/flate:/ flate.wasm -test.v
		{
			name: "tinygo test compress/flate",
			fs: []FS{
				&adapter{fs: testfs.FS{}},
				&adapter{fs: testfs.FS{"testdata/e.txt": &testfs.File{}}},
				&adapter{fs: testfs.FS{"testdata/Isaac.Newton-Opticks.txt": &testfs.File{}}},
			},
			guestPaths: []string{"/", "../", "../../"},
			expected:   []string{"../testdata/e.txt", "../../testdata/Isaac.Newton-Opticks.txt"},
			unexpected: []string{"../../testdata/e.txt"},
		},
		// e.g. from Go project root:
		//	$ GOOS=js GOARCH=wasm bin/go test -c -o net.wasm ne
		//	$ wazero run -mount=src/net:/ net.wasm -test.v -test.short
		{
			name: "go test net",
			fs: []FS{
				&adapter{fs: testfs.FS{"services": &testfs.File{}}},
				&adapter{fs: testfs.FS{"testdata/aliases": &testfs.File{}}},
			},
			guestPaths: []string{"/etc", "/"},
			expected:   []string{"/etc/services", "testdata/aliases"},
			unexpected: []string{"services"},
		},
		// e.g. from wagi-python project root:
		//	$ GOOS=js GOARCH=wasm bin/go test -c -o net.wasm ne
		//	$ wazero run -hostlogging=filesystem -mount=.:/ -env=PYTHONHOME=/opt/wasi-python/lib/python3.11 \
		//	  -env=PYTHONPATH=/opt/wasi-python/lib/python3.11 opt/wasi-python/bin/python3.wasm
		{
			name: "python",
			fs: []FS{
				&adapter{fs: gofstest.MapFS{ // to allow resolution of "."
					"pybuilddir.txt": &gofstest.MapFile{},
					"opt/wasi-python/lib/python3.11/__phello__/__init__.py": &gofstest.MapFile{},
				}},
			},
			guestPaths: []string{"/"},
			expected: []string{
				".",
				"pybuilddir.txt",
				"opt/wasi-python/lib/python3.11/__phello__/__init__.py",
			},
		},
		// e.g. from Zig project root: TODO: verify this once cli works with multiple mounts
		//	$ zig test --test-cmd wazero --test-cmd run --test-cmd -mount=.:/ -mount=/tmp:/tmp \
		//	  --test-cmd-bin -target wasm32-wasi --zig-lib-dir ./lib ./lib/std/std.zig
		{
			name: "zig",
			fs: []FS{
				&adapter{fs: testfs.FS{"zig-cache": &testfs.File{}}},
				&adapter{fs: testfs.FS{"qSQRrUkgJX9L20mr": &testfs.File{}}},
			},
			guestPaths: []string{"/", "/tmp"},
			expected:   []string{"zig-cache", "/tmp/qSQRrUkgJX9L20mr"},
			unexpected: []string{"/qSQRrUkgJX9L20mr"},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			root, err := NewRootFS(tc.fs, tc.guestPaths)
			require.NoError(t, err)

			for _, p := range tc.expected {
				f, err := root.OpenFile(p, os.O_RDONLY, 0)
				require.NoError(t, err, p)
				require.NoError(t, f.Close(), p)
			}

			for _, p := range tc.unexpected {
				_, err := root.OpenFile(p, os.O_RDONLY, 0)
				require.EqualErrno(t, syscall.ENOENT, err)
			}
		})
	}
}

func Test_stripPrefixesAndTrailingSlash(t *testing.T) {
	tests := []struct {
		path, expected string
	}{
		{
			path:     ".",
			expected: "",
		},
		{
			path:     "/",
			expected: "",
		},
		{
			path:     "./",
			expected: "",
		},
		{
			path:     "./foo",
			expected: "foo",
		},
		{
			path:     ".foo",
			expected: ".foo",
		},
		{
			path:     "././foo",
			expected: "foo",
		},
		{
			path:     "/foo",
			expected: "foo",
		},
		{
			path:     "foo/",
			expected: "foo",
		},
		{
			path:     "//",
			expected: "",
		},
		{
			path:     "../../",
			expected: "../..",
		},
		{
			path:     "./../../",
			expected: "../..",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.path, func(t *testing.T) {
			pathI, pathLen := stripPrefixesAndTrailingSlash(tc.path)
			require.Equal(t, tc.expected, tc.path[pathI:pathLen])
		})
	}
}

func Test_hasPathPrefix(t *testing.T) {
	tests := []struct {
		name                  string
		path, prefix          string
		expectEq, expectMatch bool
	}{
		{
			name:        "empty prefix",
			path:        "foo",
			prefix:      "",
			expectEq:    false,
			expectMatch: true,
		},
		{
			name:        "equal prefix",
			path:        "foo",
			prefix:      "foo",
			expectEq:    true,
			expectMatch: true,
		},
		{
			name:        "sub path",
			path:        "foo/bar",
			prefix:      "foo",
			expectMatch: true,
		},
		{
			name:        "different sub path",
			path:        "foo/bar",
			prefix:      "bar",
			expectMatch: false,
		},
		{
			name:        "different path same length",
			path:        "foo",
			prefix:      "bar",
			expectMatch: false,
		},
		{
			name:        "longer path",
			path:        "foo",
			prefix:      "foo/bar",
			expectMatch: false,
		},
		{
			name:        "path shorter",
			path:        "foo",
			prefix:      "fooo",
			expectMatch: false,
		},
		{
			name:        "path longer",
			path:        "fooo",
			prefix:      "foo",
			expectMatch: false,
		},
		{
			name:        "shorter path",
			path:        "foo",
			prefix:      "foo/bar",
			expectMatch: false,
		},
		{
			name:        "wrong and shorter path",
			path:        "foo",
			prefix:      "bar/foo",
			expectMatch: false,
		},
		{
			name:        "same relative",
			path:        "../..",
			prefix:      "../..",
			expectEq:    true,
			expectMatch: true,
		},
		{
			name:        "longer relative",
			path:        "..",
			prefix:      "../..",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			path := "././." + tc.path + "/"
			eq, match := hasPathPrefix(path, 5, 5+len(tc.path), tc.prefix)
			require.Equal(t, tc.expectEq, eq)
			require.Equal(t, tc.expectMatch, match)
		})
	}
}

// BenchmarkHasPrefixVsIterate shows that iteration is faster than re-slicing
// for a prefix match.
func BenchmarkHasPrefixVsIterate(b *testing.B) {
	s := "../../.."
	prefix := "../bar"
	prefixLen := len(prefix)
	b.Run("strings.HasPrefix", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if strings.HasPrefix(s, prefix) { //nolint
			}
		}
	})
	b.Run("iterate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for i := 0; i < prefixLen; i++ {
				if s[i] != prefix[i] {
					break
				}
			}
		}
	})
}
