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
	testFutimens(t, true)
}

func testFutimens(t *testing.T, usePath bool) {
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
			if !usePath && symlinkNoFollow {
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

				var oldSt Stat_t
				require.NoError(t, Lstat(statPath, &oldSt))

				if usePath {
					err = Utimens(path, tc.times, !symlinkNoFollow)
					if symlinkNoFollow && !SupportsSymlinkNoFollow {
						require.EqualErrno(t, syscall.ENOSYS, err)
						return
					}
					require.NoError(t, err)
				} else {
					flag := syscall.O_RDWR
					if path == dir {
						flag = syscall.O_RDONLY
						if runtime.GOOS == "windows" {
							// windows requires O_RDWR, which is invalid for directories
							t.Skip("windows cannot update timestamps on a dir")
						}
					}
					f, err := OpenFile(path, flag, 0)
					require.NoError(t, err)
					err = UtimensFile(f, tc.times)
					require.NoError(t, f.Close())
					require.NoError(t, err)
				}

				var newSt Stat_t
				require.NoError(t, Lstat(statPath, &newSt))

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

func TestUtimensFile(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin": // supported
	case "freebsd": // TODO: support freebsd w/o CGO
	case "windows":
		if !IsGo120 {
			t.Skip("windows only works after Go 1.20") // TODO: possibly 1.19 ;)
		}
	default: // expect ENOSYS and callers need to fall back to Utimens
		t.Skip("unsupported GOOS", runtime.GOOS)
	}

	testFutimens(t, false)

	t.Run("closed file", func(t *testing.T) {
		file := path.Join(t.TempDir(), "file")
		err := os.WriteFile(file, []byte{}, 0o700)
		require.NoError(t, err)
		fileF, err := OpenFile(file, syscall.O_RDWR, 0)
		require.NoError(t, err)
		require.NoError(t, fileF.Close())

		err = UtimensFile(fileF, nil)
		require.EqualErrno(t, syscall.EBADF, err)
	})

	t.Run("closed dir", func(t *testing.T) {
		dir := path.Join(t.TempDir(), "dir")
		err := os.Mkdir(dir, 0o700)
		require.NoError(t, err)
		dirF, err := OpenFile(dir, syscall.O_RDONLY, 0)
		require.NoError(t, err)
		require.NoError(t, dirF.Close())

		err = UtimensFile(dirF, nil)
		require.EqualErrno(t, syscall.EBADF, err)
	})
}
