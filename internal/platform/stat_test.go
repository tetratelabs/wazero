package platform

import (
	"os"
	"path"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestStat(t *testing.T) {
	tmpDir := t.TempDir()

	var stat Stat_t
	require.EqualErrno(t, syscall.ENOENT, Stat(path.Join(tmpDir, "cat"), &stat))
	require.EqualErrno(t, syscall.ENOENT, Stat(path.Join(tmpDir, "sub/cat"), &stat))

	t.Run("dir", func(t *testing.T) {
		err := Stat(tmpDir, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
	})

	t.Run("file", func(t *testing.T) {
		file := path.Join(tmpDir, "file")
		require.NoError(t, os.WriteFile(file, nil, 0o400))

		require.NoError(t, Stat(file, &stat))
		require.False(t, stat.Mode.IsDir())
	})

	t.Run("subdir", func(t *testing.T) {
		subdir := path.Join(tmpDir, "sub")
		require.NoError(t, os.Mkdir(subdir, 0o500))

		require.NoError(t, Stat(subdir, &stat))
		require.True(t, stat.Mode.IsDir())
	})
}

func TestStatFile(t *testing.T) {
	tmpDir := t.TempDir()

	var stat Stat_t

	tmpDirF, err := OpenFile(tmpDir, syscall.O_RDONLY, 0)
	if err != nil {
		return
	}
	defer tmpDirF.Close()

	t.Run("dir", func(t *testing.T) {
		err = StatFile(tmpDirF, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
	})

	if runtime.GOOS != "windows" { // windows allows you to stat a closed dir
		t.Run("closed dir", func(t *testing.T) {
			require.NoError(t, tmpDirF.Close())
			require.EqualErrno(t, syscall.EIO, StatFile(tmpDirF, &stat))
		})
	}

	file := path.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(file, nil, 0o400))
	fileF, err := OpenFile(file, syscall.O_RDONLY, 0)
	if err != nil {
		return
	}
	defer fileF.Close()

	t.Run("file", func(t *testing.T) {
		err = StatFile(fileF, &stat)
		require.NoError(t, err)
		require.False(t, stat.Mode.IsDir())
	})

	t.Run("closed file", func(t *testing.T) {
		require.NoError(t, fileF.Close())
		require.EqualErrno(t, syscall.EIO, StatFile(fileF, &stat))
	})

	subdir := path.Join(tmpDir, "sub")
	require.NoError(t, os.Mkdir(subdir, 0o500))
	subdirF, err := OpenFile(subdir, syscall.O_RDONLY, 0)
	if err != nil {
		return
	}
	defer subdirF.Close()

	t.Run("subdir", func(t *testing.T) {
		err = StatFile(subdirF, &stat)
		require.NoError(t, err)
		require.True(t, stat.Mode.IsDir())
	})

	if runtime.GOOS != "windows" { // windows allows you to stat a closed dir
		t.Run("closed subdir", func(t *testing.T) {
			require.NoError(t, subdirF.Close())
			require.EqualErrno(t, syscall.EIO, StatFile(subdirF, &stat))
		})
	}
}

func Test_StatFile_times(t *testing.T) {
	tmpDir := t.TempDir()

	file := path.Join(tmpDir, "file")
	err := os.WriteFile(file, []byte{}, 0o700)
	require.NoError(t, err)

	type test struct {
		name                 string
		atimeNsec, mtimeNsec int64
	}
	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{
			name:      "positive",
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "zero"},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{
				name:      "negative",
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := os.Chtimes(file, time.UnixMicro(tc.atimeNsec/1e3), time.UnixMicro(tc.mtimeNsec/1e3))
			require.NoError(t, err)

			file, err := os.Open(file)
			require.NoError(t, err)
			defer file.Close()

			var stat Stat_t
			require.NoError(t, StatFile(file, &stat))
			require.Equal(t, stat.Atim, tc.atimeNsec)
			require.Equal(t, stat.Mtim, tc.mtimeNsec)
		})
	}
}

func TestStatFile_dev_inode(t *testing.T) {
	tmpDir := t.TempDir()

	path1 := path.Join(tmpDir, "1")
	f1, err := os.Create(path1)
	require.NoError(t, err)

	path2 := path.Join(tmpDir, "2")
	f2, err := os.Create(path2)
	require.NoError(t, err)

	var stat1 Stat_t
	require.NoError(t, StatFile(f1, &stat1))

	var stat2 Stat_t
	require.NoError(t, StatFile(f2, &stat2))

	// The files should be on the same device, but different inodes
	require.Equal(t, stat1.Dev, stat2.Dev)
	require.NotEqual(t, stat1.Ino, stat2.Ino)

	// Redoing stat should result in the same inodes
	var stat1Again Stat_t
	require.NoError(t, StatFile(f1, &stat1Again))

	require.Equal(t, stat1.Dev, stat1Again.Dev)
	require.Equal(t, stat1.Ino, stat1Again.Ino)

	// On Windows, we cannot rename while opening.
	// So we manually close here before renaming.
	require.NoError(t, f1.Close())
	require.NoError(t, f2.Close())

	// Renaming a file shouldn't change its inodes.
	require.NoError(t, Rename(path1, path2))
	f1, err = os.Open(path2)
	require.NoError(t, err)
	defer f1.Close()

	require.NoError(t, StatFile(f1, &stat1Again))
	require.Equal(t, stat1.Dev, stat1Again.Dev)
	require.Equal(t, stat1.Ino, stat1Again.Ino)
}
