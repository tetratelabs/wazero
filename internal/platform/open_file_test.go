package platform

import (
	"os"
	path "path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestOpenFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("directory with a trailing slash", func(t *testing.T) {
		path := path.Join(tmpDir, "dir", "nested")
		err := os.MkdirAll(path, 0o700)
		require.NoError(t, err)

		f, errno := OpenFile(path+"/", os.O_RDONLY, 0)
		require.Zero(t, errno)
		require.NoError(t, f.Close())
	})

	// from os.TestDirFSPathsValid
	if runtime.GOOS != "windows" {
		t.Run("strange name", func(t *testing.T) {
			f, errno := OpenFile(path.Join(tmpDir, `e:xperi\ment.txt`), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
			require.Zero(t, errno)
			require.NoError(t, f.Close())
		})
	}
}

func TestOpenFile_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("file with a trailing slash is ENOTDIR", func(t *testing.T) {
		nested := path.Join(tmpDir, "dir", "nested")
		err := os.MkdirAll(nested, 0o700)
		require.NoError(t, err)

		nestedFile := path.Join(nested, "file")
		err = os.WriteFile(nestedFile, nil, 0o700)
		require.NoError(t, err)

		_, errno := OpenFile(nestedFile+"/", os.O_RDONLY, 0)
		require.EqualErrno(t, syscall.ENOTDIR, errno)
	})

	t.Run("not found must be ENOENT", func(t *testing.T) {
		_, errno := OpenFile(path.Join(tmpDir, "not-really-exist.txt"), os.O_RDONLY, 0o600)
		require.EqualErrno(t, syscall.ENOENT, errno)
	})

	// This is the same as https://github.com/ziglang/zig/blob/d24ebf1d12cf66665b52136a2807f97ff021d78d/lib/std/os/test.zig#L105-L112
	t.Run("try creating on existing file must be EEXIST", func(t *testing.T) {
		filepath := path.Join(tmpDir, "file.txt")
		f, errno := OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		require.Zero(t, errno)
		defer require.NoError(t, f.Close())

		_, errno = OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		require.EqualErrno(t, syscall.EEXIST, errno)
	})

	t.Run("writing to a read-only file is EBADF", func(t *testing.T) {
		path := path.Join(tmpDir, "file")
		require.NoError(t, os.WriteFile(path, nil, 0o600))

		f, errno := OpenFile(path, os.O_RDONLY, 0)
		defer require.NoError(t, f.Close())
		require.Zero(t, errno)

		_, err := f.Write([]byte{1, 2, 3, 4})
		require.EqualErrno(t, syscall.EBADF, UnwrapOSError(err))
	})

	t.Run("writing to a directory is EBADF", func(t *testing.T) {
		path := path.Join(tmpDir, "diragain")
		require.NoError(t, os.Mkdir(path, 0o755))

		f, errno := OpenFile(path, os.O_RDONLY, 0)
		defer require.NoError(t, f.Close())
		require.Zero(t, errno)

		_, err := f.Write([]byte{1, 2, 3, 4})
		require.EqualErrno(t, syscall.EBADF, UnwrapOSError(err))
	})

	// This is similar to https://github.com/WebAssembly/wasi-testsuite/blob/dc7f8d27be1030cd4788ebdf07d9b57e5d23441e/tests/rust/src/bin/dangling_symlink.rs
	t.Run("dangling symlinks", func(t *testing.T) {
		target := path.Join(tmpDir, "target")
		symlink := path.Join(tmpDir, "dangling_symlink_symlink.cleanup")

		err := os.Symlink(target, symlink)
		require.NoError(t, err)

		_, errno := OpenFile(symlink, O_DIRECTORY|O_NOFOLLOW, 0o0666)
		require.EqualErrno(t, syscall.ENOTDIR, errno)

		_, errno = OpenFile(symlink, O_NOFOLLOW, 0o0666)
		require.EqualErrno(t, syscall.ELOOP, errno)
	})
}
