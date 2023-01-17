package platform

import (
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_StatTimes(t *testing.T) {
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

			stat, err := os.Stat(file)
			require.NoError(t, err)

			atimeNsec, mtimeNsec, _ := StatTimes(stat)
			require.Equal(t, atimeNsec, tc.atimeNsec)
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}

func TestStatDeviceInode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform.StatDeviceInode not yet implemented on windows")
	}

	tmpDir := t.TempDir()

	path1 := path.Join(tmpDir, "1")
	fa, err := os.Create(path1)
	require.NoError(t, err)
	defer fa.Close()

	path2 := path.Join(tmpDir, "2")
	fb, err := os.Create(path2)
	require.NoError(t, err)
	defer fb.Close()

	stat1, err := fa.Stat()
	require.NoError(t, err)
	device1, inode1 := StatDeviceInode(stat1)

	stat2, err := fb.Stat()
	require.NoError(t, err)
	device2, inode2 := StatDeviceInode(stat2)

	// The files should be on the same device, but different inodes
	require.Equal(t, device1, device2)
	require.NotEqual(t, inode1, inode2)

	// Redoing stat should result in the same inodes
	stat1Again, err := os.Stat(path1)
	require.NoError(t, err)
	device1Again, inode1Again := StatDeviceInode(stat1Again)
	require.Equal(t, device1, device1Again)
	require.Equal(t, inode1, inode1Again)

	// Renaming a file shouldn't change its inodes
	require.NoError(t, os.Rename(path1, path2))
	stat1Again, err = os.Stat(path2)
	require.NoError(t, err)
	device1Again, inode1Again = StatDeviceInode(stat1Again)
	require.Equal(t, device1, device1Again)
	require.Equal(t, inode1, inode1Again)
}
