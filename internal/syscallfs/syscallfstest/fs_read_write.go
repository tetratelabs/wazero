package syscallfstest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/syscallfs"
)

// TestReadWriteFS is a test suite used to test the capabilities of file systems
// supporting both read and write operations (e.g. creating directories,
// writing files, etc...)
//
// The intent is for this test suite to help validate that implementations of
// the FS interface all abide to the same behavior. All implementations of this
// interface is Wazero that allow mutations are tested against this test suite,
// and programs providing their own file system should validate their behavior
// with:
//
//	func TestCustomFS(t *testing.T) {
//		syscallfstest.TestReadWriteFS(t, func(t *testing.T) syscallfs.FS {
//			...
//		})
//	}
func TestReadWriteFS(t *testing.T, makeFS MakeFS) {
	t.Run("OpenFile", OpenFileTests.RunFunc(makeFS))
	t.Run("Mkdir", MkdirTests.RunFunc(makeFS))
	t.Run("Rmdir", RmdirTests.RunFunc(makeFS))
	t.Run("Rename", RenameTests.RunFunc(makeFS))
	t.Run("Unlink", UnlinkTests.RunFunc(makeFS))
	t.Run("Utimes", UtimesTests.RunFunc(makeFS))

	// TODO: syscallfs.FS used to match the required behavior of fs.FS but it
	// isn't the case anymore. If we want syscallfs.FS.Open to match fs.FS we
	// should re-enable this test.
	/*
		t.Run("fstest", func(t *testing.T) {
			fsys := makeFS(t)

			files := fstest.MapFS{
				"file-0": &fstest.MapFile{Data: []byte(``)},
				"file-1": &fstest.MapFile{Data: []byte(`Hello World!`)},
				"file-2": &fstest.MapFile{Data: []byte(`1234567890`)},
			}

			for name, file := range files {
				if err := writeFile(fsys, name, file.Data); err != nil {
					t.Fatal(err)
				}
			}

			expected := make([]string, 0, len(files))
			for name := range files {
				expected = append(expected, name)
			}
			sort.Strings(expected)

			if err := fstest.TestFS(fsys, expected...); err != nil {
				t.Error(err)
			}
		})
	*/
}

var OpenFileTests = TestFS{
	"opening files which do not exist fails with ENOENT": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error {
			_, err := fsys.OpenFile("test", os.O_RDONLY, 0)
			return err
		}),

	"files can be created in the file system": expect(nil,
		func(fsys syscallfs.FS) error {
			f, err := fsys.OpenFile("test", os.O_CREATE|os.O_EXCL|os.O_TRUNC|os.O_RDWR, 0644)
			if err != nil {
				return err
			}
			f.Close()
			return nil
		}),

	"existing directories can be opened": expect(nil,
		func(fsys syscallfs.FS) error {
			const directory = "test"
			const file0 = directory + "/file-0"
			const file1 = directory + "/file-1"
			const file2 = directory + "/file-2"
			if err := fsys.Mkdir(directory, 0755); err != nil {
				return err
			}
			for _, file := range []string{file0, file1, file2} {
				if err := writeFile(fsys, file, nil); err != nil {
					return err
				}
			}
			f, err := fsys.OpenFile(directory, os.O_RDONLY, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			d, ok := f.(fs.ReadDirFile)
			if !ok {
				return fmt.Errorf("the open file is not a directory")
			}
			entries, err := d.ReadDir(-1)
			if err != nil {
				return err
			}
			if len(entries) != 3 {
				return fmt.Errorf("wrong number of directory entries: want=%d got=%d", 3, len(entries))
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Name() < entries[j].Name()
			})
			for i, want := range []string{file0, file1, file2} {
				if got := path.Join(directory, entries[i].Name()); want != got {
					return fmt.Errorf("wrong file at index %d: want=%s got=%s", i, want, got)
				}
			}
			return nil
		}),

	"existing files can be opened": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			const data = "Hello World!"
			if err := writeFile(fsys, path, []byte(data)); err != nil {
				return err
			}
			b, err := readFile(fsys, path)
			if err != nil {
				return err
			}
			if string(b) != data {
				return fmt.Errorf("file content mismatch: want=%q got=%q", data, b)
			}
			return nil
		}),
}

var MkdirTests = TestFS{
	"directories can be created in the file system": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := fsys.Mkdir(path, 0755); err != nil {
				return err
			}
			s, err := fs.Stat(fsys, path)
			if err != nil {
				return err
			}
			if !s.IsDir() {
				return fmt.Errorf("directory created at %q is not seen as a directory", path)
			}
			return nil
		}),

	"create a directory which already exists fails with EEXIST": expect(syscall.EEXIST,
		func(fsys syscallfs.FS) error {
			const path = "test"
			const perm = 0755
			if err := fsys.Mkdir(path, perm); err != nil {
				return err
			}
			return fsys.Mkdir(path, perm)
		}),

	"create a directory in place where a file exists fails with EEXIST": expect(syscall.EEXIST,
		func(fsys syscallfs.FS) error {
			const path = "test"
			f, err := fsys.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_TRUNC|os.O_RDWR, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			return fsys.Mkdir(path, 0755)
		}),
}

var RmdirTests = TestFS{
	"removing a directory which does not exist fails with ENOENT": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error { return fsys.Rmdir("nope") }),

	"removing a directory which is not empty fails with ENOTEMPTY": expect(syscall.ENOTEMPTY,
		func(fsys syscallfs.FS) error {
			if err := fsys.Mkdir("tmp", 0755); err != nil {
				return err
			}
			if err := writeFile(fsys, "tmp/test", nil); err != nil {
				return err
			}
			return fsys.Rmdir("tmp")
		}),

	"removing a file which is not a directory fails with ENOTDIR": expect(syscall.ENOTDIR,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := writeFile(fsys, path, nil); err != nil {
				return err
			}
			return fsys.Rmdir(path)
		}),

	"empty directories can be removed": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := fsys.Mkdir(path, 0755); err != nil {
				return err
			}
			return fsys.Rmdir(path)
		}),
}

// TODO: handle the different behavior on windows
var RenameTests = TestFS{
	"files can be moved to an existing directory": expect(nil,
		func(fsys syscallfs.FS) error {
			const source = "source"
			const target = "target"
			if err := writeFile(fsys, source, nil); err != nil {
				return err
			}
			if err := fsys.Rename(source, target); err != nil {
				return err
			}
			if _, err := fs.Stat(fsys, source); !errors.Is(err, syscall.ENOENT) {
				return fmt.Errorf("the source file still exists after being moved: %w", err)
			}
			if _, err := fs.Stat(fsys, target); err != nil {
				return err
			}
			return nil
		}),

	"files can be moved to a location where a file already exists": expect(nil,
		func(fsys syscallfs.FS) error {
			const source = "source"
			const target = "target"
			if err := writeFile(fsys, source, []byte(`1`)); err != nil {
				return err
			}
			if err := writeFile(fsys, target, []byte(`2`)); err != nil {
				return err
			}
			if err := fsys.Rename(source, target); err != nil {
				return err
			}
			if _, err := fs.Stat(fsys, source); !errors.Is(err, syscall.ENOENT) {
				return fmt.Errorf("the source file still exists after being moved: %w", err)
			}
			if b, err := readFile(fsys, target); err != nil {
				return err
			} else if string(b) != `1` {
				return fmt.Errorf("the target file has the wrong content after being moved: %q", b)
			}
			return nil
		}),

	"a file can be moved to itself": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := writeFile(fsys, path, []byte(`1`)); err != nil {
				return err
			}
			if err := fsys.Rename(path, path); err != nil {
				return err
			}
			if b, err := readFile(fsys, path); err != nil {
				return err
			} else if string(b) != `1` {
				return fmt.Errorf("the target file has the wrong content after being moved: %q", b)
			}
			return nil
		}),

	"directories can be moved to a location where a directory already exists": expect(nil,
		func(fsys syscallfs.FS) error {
			const source = "source"
			const target = "target"
			if err := fsys.Mkdir(source, 0755); err != nil {
				return err
			}
			if err := fsys.Mkdir(target, 0755); err != nil {
				return err
			}
			if err := fsys.Rename(source, target); err != nil {
				return err
			}
			if _, err := fs.Stat(fsys, source); !errors.Is(err, syscall.ENOENT) {
				return fmt.Errorf("the source directory still exists after being moved: %w", err)
			}
			if _, err := fs.Stat(fsys, target); err != nil {
				return err
			}
			return nil
		}),

	"a directory can be moved to itself": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := fsys.Mkdir(path, 0755); err != nil {
				return err
			}
			if err := fsys.Rename(path, path); err != nil {
				return err
			}
			if _, err := fs.Stat(fsys, path); err != nil {
				return err
			}
			return nil
		}),

	"moving a file which does not exist fails with ENOENT": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error {
			return fsys.Rename("source", "target")
		}),

	"moving a directory to a location which does not exist fails with ENOENT": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error {
			const source = "source"
			const target = "target/does/not/exist"
			if err := fsys.Mkdir(source, 0755); err != nil {
				return err
			}
			return fsys.Rename(source, target)
		}),

	"moving a directory to a location where a file exists fails with ENOTDIR": expect(syscall.ENOTDIR,
		func(fsys syscallfs.FS) error {
			const source = "source"
			const target = "target"
			if err := writeFile(fsys, target, nil); err != nil {
				return err
			}
			if err := fsys.Mkdir(source, 0755); err != nil {
				return err
			}
			return fsys.Rename(source, target)
		}),

	"moving a file to a location where a directory exists fails with EISDIR": expect(syscall.EISDIR,
		func(fsys syscallfs.FS) error {
			const source = "source"
			const target = "target"
			if err := fsys.Mkdir(target, 0755); err != nil {
				return err
			}
			if err := writeFile(fsys, source, nil); err != nil {
				return err
			}
			return fsys.Rename(source, target)
		}),
}

var UnlinkTests = TestFS{
	"unlinking a file which does not exist fails with ENOENT": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error { return fsys.Unlink("nope") }),

	"unlinking a directory fails with EISDIR": expect(syscall.EISDIR,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := fsys.Mkdir(path, 0755); err != nil {
				return err
			}
			return fsys.Unlink(path)
		}),

	"existing files can be unlinked and do not exist in the directory anymore afterwards": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := writeFile(fsys, path, nil); err != nil {
				return err
			}
			if err := fsys.Unlink(path); err != nil {
				return err
			}
			_, err := fs.Stat(fsys, path)
			return err
		}),
}

var UtimesTests = TestFS{
	"changing time of a file which does not exist fails with ENOENT": expect(syscall.ENOENT,
		func(fsys syscallfs.FS) error {
			atim := time.Unix(123, 4*1e3).UnixNano()
			mtim := time.Unix(567, 8*1e3).UnixNano()
			return fsys.Utimes("nope", atim, mtim)
		}),

	"times of existing files can be set to zero": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := writeFile(fsys, path, nil); err != nil {
				return err
			}
			return testUtimes(fsys, path, 0, 0)
		}),

	"times of existing files can be changed": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := writeFile(fsys, path, nil); err != nil {
				return err
			}
			// Use microsecond granularity because Windows doesn't support
			// nanosecond precision.
			atim := time.Unix(123, 4*1e3).UnixNano()
			mtim := time.Unix(567, 8*1e3).UnixNano()
			return testUtimes(fsys, path, atim, mtim)
		}),

	"times of existing directories can be set to zero": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := fsys.Mkdir(path, 0755); err != nil {
				return err
			}
			return testUtimes(fsys, path, 0, 0)
		}),

	"times of existing directories can be changed": expect(nil,
		func(fsys syscallfs.FS) error {
			const path = "test"
			if err := fsys.Mkdir(path, 0755); err != nil {
				return err
			}
			atim := time.Unix(123, 4*1e3).UnixNano()
			mtim := time.Unix(567, 8*1e3).UnixNano()
			return testUtimes(fsys, path, atim, mtim)
		}),
}

func testUtimes(fsys syscallfs.FS, path string, atim, mtim int64) error {
	if err := fsys.Utimes(path, atim, mtim); err != nil {
		return err
	}
	s, err := fs.Stat(fsys, path)
	if err != nil {
		return err
	}
	statAtim, statMtim, _ := platform.StatTimes(s)
	if platform.CompilerSupported() {
		// Only some platforms support access time, otherwise we check for
		// modification time only.
		if statAtim != atim {
			return fmt.Errorf("access time mismatch: want=%v got=%v", atim, statAtim)
		}
	}
	if statMtim != mtim {
		return fmt.Errorf("modification time mismatch: want=%v got=%v", mtim, statMtim)
	}
	return nil
}

func readFile(fsys syscallfs.FS, path string) ([]byte, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := bytes.Buffer{}
	_, err = b.ReadFrom(f)
	return b.Bytes(), err
}

func writeFile(fsys syscallfs.FS, path string, data []byte) error {
	f, err := fsys.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(data) > 0 {
		_, err := f.(io.Writer).Write(data)
		if err != nil {
			return err
		}
	}
	return nil
}
