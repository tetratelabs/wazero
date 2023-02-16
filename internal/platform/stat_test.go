package platform

import (
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_Stat(t *testing.T) {
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

func TestStat_dev_inode(t *testing.T) {
	tmpDir := t.TempDir()

	path1 := path.Join(tmpDir, "1")
	fa, err := os.Create(path1)
	require.NoError(t, err)

	path2 := path.Join(tmpDir, "2")
	fb, err := os.Create(path2)
	require.NoError(t, err)

	stat1, err := fa.Stat()
	require.NoError(t, err)
	_, _, _, _, device1, inode1, err := Stat(fa, stat1)
	require.NoError(t, err)

	stat2, err := fb.Stat()
	require.NoError(t, err)
	_, _, _, _, device2, inode2, err := Stat(fb, stat2)
	require.NoError(t, err)

	// The files should be on the same device, but different inodes
	require.Equal(t, device1, device2)
	require.NotEqual(t, inode1, inode2)

	// Redoing stat should result in the same inodes
	stat1Again, err := os.Stat(path1)
	require.NoError(t, err)
	_, _, _, _, device1Again, inode1Again, err := Stat(fa, stat1Again)
	require.NoError(t, err)
	require.Equal(t, device1, device1Again)
	require.Equal(t, inode1, inode1Again)

	// On Windows, we cannot rename while opening.
	// So we manually close here before renaming.
	require.NoError(t, fa.Close())
	require.NoError(t, fb.Close())

	// Renaming a file shouldn't change its inodes.
	require.NoError(t, Rename(path1, path2))
	fa, err = os.Open(path2)
	require.NoError(t, err)
	defer func() { require.NoError(t, fa.Close()) }()
	stat1Again, err = os.Stat(path2)
	require.NoError(t, err)
	_, _, _, _, device1Again, inode1Again, err = Stat(fa, stat1Again)
	require.NoError(t, err)
	require.Equal(t, device1, device1Again)
	require.Equal(t, inode1, inode1Again)
}
