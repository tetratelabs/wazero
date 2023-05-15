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

func TestUtimens(t *testing.T) {
	t.Run("doesn't exist", func(t *testing.T) {
		err := Utimens("nope", nil, true)
		require.EqualErrno(t, syscall.ENOENT, err)

		err = Utimens("nope", nil, false)
		if SupportsSymlinkNoFollow {
			require.EqualErrno(t, syscall.ENOENT, err)
		} else {
			require.EqualErrno(t, syscall.ENOSYS, err)
		}
	})
	testUtimens(t, false)
}

func testUtimens(t *testing.T, futimes bool) {
	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	//
	// Negative isn't tested as most platforms don't return consistent results.
	tests := []struct {
		name  string
		times *[2]syscall.Timespec
	}{
		{
			name: "nil",
		},
		{
			name: "a=omit,m=omit",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_OMIT},
				{Sec: 123, Nsec: UTIME_OMIT},
			},
		},
		{
			name: "a=now,m=omit",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_NOW},
				{Sec: 123, Nsec: UTIME_OMIT},
			},
		},
		{
			name: "a=omit,m=now",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_OMIT},
				{Sec: 123, Nsec: UTIME_NOW},
			},
		},
		{
			name: "a=now,m=now",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_NOW},
				{Sec: 123, Nsec: UTIME_NOW},
			},
		},
		{
			name: "a=now,m=set",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_NOW},
				{Sec: 123, Nsec: 4 * 1e3},
			},
		},
		{
			name: "a=set,m=now",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: 4 * 1e3},
				{Sec: 123, Nsec: UTIME_NOW},
			},
		},
		{
			name: "a=set,m=omit",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: 4 * 1e3},
				{Sec: 123, Nsec: UTIME_OMIT},
			},
		},
		{
			name: "a=omit,m=set",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: UTIME_OMIT},
				{Sec: 123, Nsec: 4 * 1e3},
			},
		},
		{
			name: "a=set,m=set",
			times: &[2]syscall.Timespec{
				{Sec: 123, Nsec: 4 * 1e3},
				{Sec: 223, Nsec: 5 * 1e3},
			},
		},
	}
	for _, fileType := range []string{"dir", "file", "link", "link-follow"} {
		for _, tt := range tests {
			tc := tt
			fileType := fileType
			name := fileType + " " + tc.name
			symlinkNoFollow := fileType == "link"

			// symlinkNoFollow is invalid for file descriptor based operations,
			// because the default for open is to follow links. You can't avoid
			// this. O_NOFOLLOW is used only to return ELOOP on a link.
			if futimes && symlinkNoFollow {
				continue
			}

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
					statPath = link
				case "link-follow":
					path = link
					statPath = file
				default:
					panic(tc)
				}

				oldSt, errno := Lstat(statPath)
				require.EqualErrno(t, 0, errno)

				if !futimes {
					err = Utimens(path, tc.times, !symlinkNoFollow)
					if symlinkNoFollow && !SupportsSymlinkNoFollow {
						require.EqualErrno(t, syscall.ENOSYS, err)
						return
					}
					require.EqualErrno(t, 0, errno)
				} else {
					flag := syscall.O_RDWR
					if path == dir {
						flag = syscall.O_RDONLY
						if runtime.GOOS == "windows" {
							// windows requires O_RDWR, which is invalid for directories
							t.Skip("windows cannot update timestamps on a dir")
						}
					}

					f := requireOpenFile(t, path, flag, 0)

					errno = f.Utimens(tc.times)
					require.EqualErrno(t, 0, f.Close())
					require.EqualErrno(t, 0, errno)
				}

				newSt, errno := Lstat(statPath)
				require.EqualErrno(t, 0, errno)

				if CompilerSupported() {
					if tc.times != nil && tc.times[0].Nsec == UTIME_OMIT {
						require.Equal(t, oldSt.Atim, newSt.Atim)
					} else if tc.times == nil || tc.times[0].Nsec == UTIME_NOW {
						now := time.Now().UnixNano()
						require.True(t, newSt.Atim <= now, "expected atim %d <= now %d", newSt.Atim, now)
					} else {
						require.Equal(t, tc.times[0].Nano(), newSt.Atim)
					}
				}

				// When compiler isn't supported, we can still check mtim.
				if tc.times != nil && tc.times[1].Nsec == UTIME_OMIT {
					require.Equal(t, oldSt.Mtim, newSt.Mtim)
				} else if tc.times == nil || tc.times[1].Nsec == UTIME_NOW {
					now := time.Now().UnixNano()
					require.True(t, newSt.Mtim <= now, "expected mtim %d <= now %d", newSt.Mtim, now)
				} else {
					require.Equal(t, tc.times[1].Nano(), newSt.Mtim)
				}
			})
		}
	}
}
