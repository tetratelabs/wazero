//go:build !windows

package sysfs

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestDirFS_Chown(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	require.EqualErrno(t, 0, testFS.Mkdir("dir", 0o0777))
	dirF, errno := testFS.OpenFile("dir", syscall.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	dirStat, err := dirF.File().Stat()
	require.NoError(t, err)

	dirSys := dirStat.Sys().(*syscall.Stat_t)

	// Similar to TestChown in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.EqualErrno(t, 0, testFS.Chown("dir", -1, -1))
		checkUidGid(t, path.Join(tmpDir, "dir"), dirSys.Uid, dirSys.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.EqualErrno(t, 0, testFS.Chown("dir", -1, gid))
		checkUidGid(t, path.Join(tmpDir, "dir"), dirSys.Uid, uint32(gid))
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Chown
			require.EqualErrno(t, 0, testFS.Chown("dir", -1, g))
			checkUidGid(t, path.Join(tmpDir, "dir"), dirSys.Uid, uint32(g))

			// Revert back
			require.EqualErrno(t, 0, dirF.Chown(-1, gid))
			checkUidGid(t, path.Join(tmpDir, "dir"), dirSys.Uid, uint32(gid))
		})
	}

	t.Run("not found", func(t *testing.T) {
		require.EqualErrno(t, syscall.ENOENT, testFS.Chown("a", -1, gid))
	})
}

func TestDirFS_Lchown(t *testing.T) {
	tmpDir := t.TempDir()
	testFS := NewDirFS(tmpDir)

	require.EqualErrno(t, 0, testFS.Mkdir("dir", 0o0777))
	dirF, errno := testFS.OpenFile("dir", syscall.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	dirStat, err := dirF.File().Stat()
	require.NoError(t, err)

	dirSys := dirStat.Sys().(*syscall.Stat_t)

	require.EqualErrno(t, 0, testFS.Symlink("dir", "link"))
	linkF, errno := testFS.OpenFile("link", syscall.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	linkStat, err := linkF.File().Stat()
	require.NoError(t, err)

	linkSys := linkStat.Sys().(*syscall.Stat_t)

	// Similar to TestLchown in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.EqualErrno(t, 0, testFS.Lchown("link", -1, -1))
		checkUidGid(t, path.Join(tmpDir, "link"), linkSys.Uid, linkSys.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.EqualErrno(t, 0, testFS.Chown("dir", -1, gid))
		checkUidGid(t, path.Join(tmpDir, "link"), linkSys.Uid, uint32(gid))
		// Make sure the target didn't change.
		checkUidGid(t, path.Join(tmpDir, "dir"), dirSys.Uid, dirSys.Gid)
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Lchown
			require.EqualErrno(t, 0, testFS.Lchown("link", -1, g))
			checkUidGid(t, path.Join(tmpDir, "link"), linkSys.Uid, uint32(g))
			// Make sure the target didn't change.
			checkUidGid(t, path.Join(tmpDir, "dir"), dirSys.Uid, dirSys.Gid)

			// Revert back
			require.EqualErrno(t, 0, testFS.Lchown("link", -1, gid))
			checkUidGid(t, path.Join(tmpDir, "link"), linkSys.Uid, uint32(gid))
		})
	}

	t.Run("not found", func(t *testing.T) {
		require.EqualErrno(t, syscall.ENOENT, testFS.Lchown("a", -1, gid))
	})
}

// checkUidGid uses lstat to ensure the comparison is against the file, not the
// target of a symbolic link.
func checkUidGid(t *testing.T, path string, uid, gid uint32) {
	ls, err := os.Lstat(path)
	require.NoError(t, err)
	sys := ls.Sys().(*syscall.Stat_t)
	require.Equal(t, uid, sys.Uid)
	require.Equal(t, gid, sys.Gid)
}
