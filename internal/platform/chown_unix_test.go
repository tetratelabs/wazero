//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestChown(t *testing.T) {
	tmpDir := t.TempDir()

	dir := path.Join(tmpDir, "dir")
	require.NoError(t, os.Mkdir(dir, 0o0777))

	dirF, errno := OpenFile(dir, syscall.O_RDONLY, 0)
	require.Zero(t, errno)

	dirStat, err := dirF.Stat()
	require.NoError(t, err)
	dirSys := dirStat.Sys().(*syscall.Stat_t)

	// Similar to TestChown in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.Zero(t, Chown(dir, -1, -1))
		checkUidGid(t, dir, dirSys.Uid, dirSys.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.Zero(t, Chown(dir, -1, gid))
		checkUidGid(t, dir, dirSys.Uid, uint32(gid))
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Chown
			require.Zero(t, Chown(dir, -1, g))
			checkUidGid(t, dir, dirSys.Uid, uint32(g))

			// Revert back with os.File.Chown
			require.NoError(t, dirF.(*os.File).Chown(-1, gid))
			checkUidGid(t, dir, dirSys.Uid, uint32(gid))
		})
	}

	t.Run("not found", func(t *testing.T) {
		require.EqualErrno(t, syscall.ENOENT, Chown(path.Join(tmpDir, "a"), -1, gid))
	})
}

func TestChownFile(t *testing.T) {
	tmpDir := t.TempDir()

	dir := path.Join(tmpDir, "dir")
	require.NoError(t, os.Mkdir(dir, 0o0777))

	dirF, errno := OpenFile(dir, syscall.O_RDONLY, 0)
	require.Zero(t, errno)

	dirStat, err := dirF.Stat()
	require.NoError(t, err)

	dirSys := dirStat.Sys().(*syscall.Stat_t)

	// Similar to TestChownFile in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.Zero(t, ChownFile(dirF, -1, -1))
		checkUidGid(t, dir, dirSys.Uid, dirSys.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.Zero(t, ChownFile(dirF, -1, gid))
		checkUidGid(t, dir, dirSys.Uid, uint32(gid))
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our ChownFile
			require.Zero(t, ChownFile(dirF, -1, g))
			checkUidGid(t, dir, dirSys.Uid, uint32(g))

			// Revert back with os.File.Chown
			require.NoError(t, dirF.(*os.File).Chown(-1, gid))
			checkUidGid(t, dir, dirSys.Uid, uint32(gid))
		})
	}

	t.Run("closed", func(t *testing.T) {
		require.NoError(t, dirF.Close())
		require.EqualErrno(t, syscall.EBADF, ChownFile(dirF, -1, gid))
	})
}

func TestLchown(t *testing.T) {
	tmpDir := t.TempDir()

	dir := path.Join(tmpDir, "dir")
	require.NoError(t, os.Mkdir(dir, 0o0777))

	dirF, errno := OpenFile(dir, syscall.O_RDONLY, 0)
	require.Zero(t, errno)

	dirStat, err := dirF.Stat()
	require.NoError(t, err)

	dirSys := dirStat.Sys().(*syscall.Stat_t)

	link := path.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(dir, link))

	linkF, errno := OpenFile(link, syscall.O_RDONLY, 0)
	require.Zero(t, errno)

	linkStat, err := linkF.Stat()
	require.NoError(t, err)

	linkSys := linkStat.Sys().(*syscall.Stat_t)

	// Similar to TestLchown in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.Zero(t, Lchown(link, -1, -1))
		checkUidGid(t, link, linkSys.Uid, linkSys.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.Zero(t, Chown(dir, -1, gid))
		checkUidGid(t, link, linkSys.Uid, uint32(gid))
		// Make sure the target didn't change.
		checkUidGid(t, dir, dirSys.Uid, dirSys.Gid)
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Lchown
			require.Zero(t, Lchown(link, -1, g))
			checkUidGid(t, link, linkSys.Uid, uint32(g))
			// Make sure the target didn't change.
			checkUidGid(t, dir, dirSys.Uid, dirSys.Gid)

			// Revert back with syscall.Lchown
			require.NoError(t, syscall.Lchown(link, -1, gid))
			checkUidGid(t, link, linkSys.Uid, uint32(gid))
		})
	}

	t.Run("not found", func(t *testing.T) {
		require.EqualErrno(t, syscall.ENOENT, Lchown(path.Join(tmpDir, "a"), -1, gid))
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
