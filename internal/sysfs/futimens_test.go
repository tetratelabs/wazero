//go:build windows || linux || darwin

package sysfs

import (
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUtimens(t *testing.T) {
	t.Run("doesn't exist", func(t *testing.T) {
		err := utimens("nope", 0, 0)
		require.EqualErrno(t, sys.ENOENT, err)
	})
	testUtimens(t, false)
}

func TestFileUtimens(t *testing.T) {
	testUtimens(t, true)

	testEBADFIfFileClosed(t, func(f experimentalsys.File) experimentalsys.Errno {
		return f.Utimens(experimentalsys.UTIME_OMIT, experimentalsys.UTIME_OMIT)
	})
	testEBADFIfDirClosed(t, func(d experimentalsys.File) experimentalsys.Errno {
		return d.Utimens(experimentalsys.UTIME_OMIT, experimentalsys.UTIME_OMIT)
	})
}

func testUtimens(t *testing.T, futimes bool) {
	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	//
	// Negative isn't tested as most platforms don't return consistent results.
	tests := []struct {
		name       string
		atim, mtim int64
	}{
		{
			name: "nil",
		},
		{
			name: "a=omit,m=omit",
			atim: sys.UTIME_OMIT,
			mtim: sys.UTIME_OMIT,
		},
		{
			name: "a=set,m=omit",
			atim: int64(123*time.Second + 4*time.Microsecond),
			mtim: sys.UTIME_OMIT,
		},
		{
			name: "a=omit,m=set",
			atim: sys.UTIME_OMIT,
			mtim: int64(123*time.Second + 4*time.Microsecond),
		},
		{
			name: "a=set,m=set",
			atim: int64(123*time.Second + 4*time.Microsecond),
			mtim: int64(223*time.Second + 5*time.Microsecond),
		},
	}
	for _, fileType := range []string{"dir", "file", "link"} {
		for _, tt := range tests {
			tc := tt
			fileType := fileType
			name := fileType + " " + tc.name

			t.Run(name, func(t *testing.T) {
				tmpDir := t.TempDir()
				file := path.Join(tmpDir, "file")
				err := os.WriteFile(file, []byte{}, 0o700)
				require.NoError(t, err)

				link := file + "-link"
				require.NoError(t, os.Symlink(file, link))

				dir := path.Join(tmpDir, "dir")
				err = os.Mkdir(dir, 0o700)
				require.NoError(t, err)

				var path, statPath string
				switch fileType {
				case "dir":
					path = dir
					statPath = dir
				case "file":
					path = file
					statPath = file
				case "link":
					path = link
					statPath = file
				default:
					panic(tc)
				}

				oldSt, errno := lstat(statPath)
				require.EqualErrno(t, 0, errno)

				if !futimes {
					errno = utimens(path, tc.atim, tc.mtim)
					require.EqualErrno(t, 0, errno)
				} else {
					flag := sys.O_RDWR
					if path == dir {
						flag = sys.O_RDONLY
						if runtime.GOOS == "windows" {
							// windows requires O_RDWR, which is invalid for directories
							t.Skip("windows cannot update timestamps on a dir")
						}
					}

					f := requireOpenFile(t, path, flag, 0)

					errno = f.Utimens(tc.atim, tc.mtim)
					require.EqualErrno(t, 0, f.Close())
					require.EqualErrno(t, 0, errno)
				}

				newSt, errno := lstat(statPath)
				require.EqualErrno(t, 0, errno)

				if platform.CompilerSupported() {
					if tc.atim == sys.UTIME_OMIT {
						require.Equal(t, oldSt.Atim, newSt.Atim)
					} else {
						require.Equal(t, tc.atim, newSt.Atim)
					}
				}

				// When compiler isn't supported, we can still check mtim.
				if tc.mtim == sys.UTIME_OMIT {
					require.Equal(t, oldSt.Mtim, newSt.Mtim)
				} else {
					require.Equal(t, tc.mtim, newSt.Mtim)
				}
			})
		}
	}
}
