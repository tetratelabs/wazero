package platform

import (
	"io/fs"
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUtimesNano(t *testing.T) {
	tmpDir := t.TempDir()
	file := path.Join(tmpDir, "file")
	err := os.WriteFile(file, []byte{}, 0o700)
	require.NoError(t, err)

	dir := path.Join(tmpDir, "dir")
	err = os.Mkdir(dir, 0o700)
	require.NoError(t, err)

	t.Run("doesn't exist", func(t *testing.T) {
		err := UtimesNano("nope",
			time.Unix(123, 4*1e3).UnixNano(),
			time.Unix(567, 8*1e3).UnixNano())
		require.EqualErrno(t, syscall.ENOENT, err)
	})

	type test struct {
		name                 string
		path                 string
		atimeNsec, mtimeNsec int64
	}

	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	//
	// Negative isn't tested as most platforms don't return consistent results.
	tests := []test{
		{
			name:      "file positive",
			path:      file,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{
			name:      "dir positive",
			path:      dir,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "file zero", path: file},
		{name: "dir zero", path: dir},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := UtimesNano(tc.path, tc.atimeNsec, tc.mtimeNsec)
			require.NoError(t, err)

			var stat Stat_t
			require.NoError(t, Stat(tc.path, &stat))
			if CompilerSupported() {
				require.Equal(t, stat.Atim, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, stat.Mtim, tc.mtimeNsec)
		})
	}
}

func TestUtimesNanoFile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("TODO: implement futimens on darwin, freebsd, linux w/o CGO")
	}

	tmpDir := t.TempDir()

	file := path.Join(tmpDir, "file")
	err := os.WriteFile(file, []byte{}, 0o700)
	require.NoError(t, err)
	fileF, err := OpenFile(file, syscall.O_RDWR, 0)
	require.NoError(t, err)
	defer fileF.Close()

	dir := path.Join(tmpDir, "dir")
	err = os.Mkdir(dir, 0o700)
	require.NoError(t, err)
	dirF, err := OpenFile(dir, syscall.O_RDONLY, 0)
	require.NoError(t, err)
	defer fileF.Close()

	type test struct {
		name                 string
		file                 fs.File
		atimeNsec, mtimeNsec int64
	}

	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	//
	// Negative isn't tested as most platforms don't return consistent results.
	tests := []test{
		{
			name:      "file positive",
			file:      fileF,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{
			name:      "dir positive",
			file:      dirF,
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "file zero", file: fileF},
		{name: "dir zero", file: dirF},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := UtimesNanoFile(tc.file, tc.atimeNsec, tc.mtimeNsec)
			require.NoError(t, err)

			var stat Stat_t
			require.NoError(t, StatFile(tc.file, &stat))
			if CompilerSupported() {
				require.Equal(t, stat.Atim, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, stat.Mtim, tc.mtimeNsec)
		})
	}

	require.NoError(t, fileF.Close())
	t.Run("closed file", func(t *testing.T) {
		err := UtimesNanoFile(fileF,
			time.Unix(123, 4*1e3).UnixNano(),
			time.Unix(567, 8*1e3).UnixNano())
		require.EqualErrno(t, syscall.EBADF, err)
	})

	require.NoError(t, dirF.Close())
	t.Run("closed dir", func(t *testing.T) {
		err := UtimesNanoFile(dirF,
			time.Unix(123, 4*1e3).UnixNano(),
			time.Unix(567, 8*1e3).UnixNano())
		require.EqualErrno(t, syscall.EBADF, err)
	})
}
