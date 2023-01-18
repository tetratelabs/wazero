package syscallfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	pathutil "path"
	"sort"
	"strings"
	"syscall"
	"testing"
	gofstest "testing/fstest"

	"github.com/tetratelabs/wazero/internal/fstest"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestNewRootFS(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		rootFS, err := NewRootFS()
		require.NoError(t, err)

		require.Equal(t, UnimplementedFS{}, rootFS)
	})
	t.Run("only root", func(t *testing.T) {
		testFS, err := NewDirFS(t.TempDir(), "/")
		require.NoError(t, err)

		rootFS, err := NewRootFS(testFS)
		require.NoError(t, err)

		// Should not be a composite filesystem
		require.Equal(t, testFS, rootFS)
	})
	t.Run("only non root unsupported", func(t *testing.T) {
		testFS, err := NewDirFS(".", "/tmp")
		require.NoError(t, err)

		_, err = NewRootFS(testFS)
		require.EqualError(t, err, "you must supply a root filesystem: .:/tmp")
	})
	t.Run("multiple roots unsupported", func(t *testing.T) {
		testFS, err := NewDirFS(".", "/")
		require.NoError(t, err)

		_, err = NewRootFS(testFS, testFS)
		require.EqualError(t, err, "multiple root filesystems are invalid: [.:/ .:/]")
	})
	t.Run("virtual paths unsupported", func(t *testing.T) {
		testFS, err := NewDirFS(".", "/usr/bin")
		require.NoError(t, err)

		_, err = NewRootFS(testFS)
		require.EqualError(t, err, "only single-level guest paths allowed: .:/usr/bin")
	})
	t.Run("multiple matches", func(t *testing.T) {
		tmpDir1 := t.TempDir()
		testFS1, err := NewDirFS(tmpDir1, "/")
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(pathutil.Join(tmpDir1, "tmp"), 0o700))
		require.NoError(t, os.WriteFile(pathutil.Join(tmpDir1, "a"), []byte{1}, 0o600))

		tmpDir2 := t.TempDir()
		testFS2, err := NewDirFS(tmpDir2, "/tmp")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(pathutil.Join(tmpDir2, "a"), []byte{2}, 0o600))

		rootFS, err := NewRootFS(testFS2, testFS1)
		require.NoError(t, err)

		// unwrapping returns in original order
		unwrapped := rootFS.(*CompositeFS).Unwrap()
		require.Equal(t, []FS{testFS2, testFS1}, unwrapped)

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
	entries, err := f.(fs.ReadDirFile).ReadDir(-1)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestRootFS_String(t *testing.T) {
	tmpFS, err := NewDirFS(".", "/tmp")
	require.NoError(t, err)

	rootFS, err := NewDirFS(".", "/")
	require.NoError(t, err)

	testFS, err := NewRootFS(rootFS, tmpFS)
	require.NoError(t, err)

	require.Equal(t, "[.:/ .:/tmp]", testFS.String())
}

func TestRootFS_Open(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory, so we can test reads outside the FS root.
	tmpDir = pathutil.Join(tmpDir, t.Name())
	require.NoError(t, os.Mkdir(tmpDir, 0o700))

	testFS, err := NewDirFS(tmpDir, "/")
	require.NoError(t, err)

	testOpen_Read(t, tmpDir, testFS)

	testOpen_O_RDWR(t, tmpDir, testFS)

	t.Run("path outside root valid", func(t *testing.T) {
		_, err := testFS.OpenFile("../foo", os.O_RDONLY, 0)

		// syscall.FS allows relative path lookups
		require.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestRootFS_TestFS(t *testing.T) {
	t.Parallel()

	// Set up the test files
	tmpDir1 := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir1))

	// move one directory outside the other
	tmpDir2 := t.TempDir()
	require.NoError(t, os.Rename(pathutil.Join(tmpDir1, "dir"), pathutil.Join(tmpDir2, "dir")))

	// Create a root mount
	testFS1, err := NewDirFS(tmpDir1, "/")
	require.NoError(t, err)

	// Create a dir mount
	testFS2, err := NewDirFS(pathutil.Join(tmpDir2, "dir"), "/dir")
	require.NoError(t, err)

	testFS, err := NewRootFS(testFS1, testFS2)
	require.NoError(t, err)

	// Run TestFS via the adapter
	require.NoError(t, fstest.TestFS(&testFSAdapter{testFS}))
}

func TestRootFS_examples(t *testing.T) {
	tests := []struct {
		name                 string
		fs                   []FS
		expected, unexpected []string
	}{
		// e.g. from Go project root:
		//	$ GOOS=js GOARCH=wasm bin/go test -c -o template.wasm text/template
		//	$ wazero run -mount=src/text/template:/ template.wasm -test.v
		{
			name: "go test text/template",
			fs: []FS{
				&adapter{fs: testfs.FS{"go-example-stdout-ExampleTemplate-0.txt": &testfs.File{}}, guestDir: "/tmp"},
				&adapter{fs: testfs.FS{"testdata/file1.tmpl": &testfs.File{}}, guestDir: "."},
			},
			expected:   []string{"/tmp/go-example-stdout-ExampleTemplate-0.txt", "testdata/file1.tmpl"},
			unexpected: []string{"DOES NOT EXIST"},
		},
		// e.g. from TinyGo project root:
		//	$ ./build/tinygo test -target wasi -c -o flate.wasm compress/flate
		//	$ wazero run -mount=$(go env GOROOT)/src/compress/flate:/ flate.wasm -test.v
		{
			name: "tinygo test compress/flate",
			fs: []FS{
				&adapter{fs: testfs.FS{}, guestDir: "/"},
				&adapter{fs: testfs.FS{"testdata/e.txt": &testfs.File{}}, guestDir: "../"},
				&adapter{fs: testfs.FS{"testdata/Isaac.Newton-Opticks.txt": &testfs.File{}}, guestDir: "../../"},
			},
			expected:   []string{"../testdata/e.txt", "../../testdata/Isaac.Newton-Opticks.txt"},
			unexpected: []string{"../../testdata/e.txt"},
		},
		// e.g. from Go project root:
		//	$ GOOS=js GOARCH=wasm bin/go test -c -o net.wasm ne
		//	$ wazero run -mount=src/net:/ net.wasm -test.v -test.short
		{
			name: "go test net",
			fs: []FS{
				&adapter{fs: testfs.FS{"services": &testfs.File{}}, guestDir: "/etc"},
				&adapter{fs: testfs.FS{"testdata/aliases": &testfs.File{}}, guestDir: "/"},
			},
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
				}, guestDir: "/"},
			},
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
				&adapter{fs: testfs.FS{"zig-cache": &testfs.File{}}, guestDir: "/"},
				&adapter{fs: testfs.FS{"qSQRrUkgJX9L20mr": &testfs.File{}}, guestDir: "/tmp"},
			},
			expected:   []string{"zig-cache", "/tmp/qSQRrUkgJX9L20mr"},
			unexpected: []string{"/qSQRrUkgJX9L20mr"},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			root, err := NewRootFS(tc.fs...)
			require.NoError(t, err)

			for _, p := range tc.expected {
				f, err := root.OpenFile(p, os.O_RDONLY, 0)
				require.NoError(t, err, p)
				require.NoError(t, f.Close(), p)
			}

			for _, p := range tc.unexpected {
				_, err := root.OpenFile(p, os.O_RDONLY, 0)
				require.Equal(t, syscall.ENOENT, err)
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
