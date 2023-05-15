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

	dirF := requireOpenFile(t, dir, syscall.O_RDONLY, 0)
	defer dirF.Close()

	dirSt, errno := dirF.Stat()
	require.EqualErrno(t, 0, errno)

	// Similar to TestChown in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.EqualErrno(t, 0, Chown(dir, -1, -1))
		checkUidGid(t, dir, dirSt.Uid, dirSt.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.EqualErrno(t, 0, Chown(dir, -1, gid))
		checkUidGid(t, dir, dirSt.Uid, uint32(gid))
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Chown
			require.EqualErrno(t, 0, Chown(dir, -1, g))
			checkUidGid(t, dir, dirSt.Uid, uint32(g))

			// Revert back
			require.EqualErrno(t, 0, dirF.Chown(-1, gid))
			checkUidGid(t, dir, dirSt.Uid, uint32(gid))
		})
	}

	t.Run("not found", func(t *testing.T) {
		require.EqualErrno(t, syscall.ENOENT, Chown(path.Join(tmpDir, "a"), -1, gid))
	})
}

func TestDefaultFileChown(t *testing.T) {
	tmpDir := t.TempDir()

	dir := path.Join(tmpDir, "dir")
	require.NoError(t, os.Mkdir(dir, 0o0777))

	dirF := requireOpenFile(t, dir, syscall.O_RDONLY, 0)
	defer dirF.Close()

	dirSt, errno := dirF.Stat()
	require.EqualErrno(t, 0, errno)

	// Similar to TestChownFile in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.EqualErrno(t, 0, dirF.Chown(-1, -1))
		checkUidGid(t, dir, dirSt.Uid, dirSt.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.EqualErrno(t, 0, dirF.Chown(-1, gid))
		checkUidGid(t, dir, dirSt.Uid, uint32(gid))
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Chown
			require.EqualErrno(t, 0, dirF.Chown(-1, g))
			checkUidGid(t, dir, dirSt.Uid, uint32(g))

			// Revert back
			require.EqualErrno(t, 0, dirF.Chown(-1, gid))
			checkUidGid(t, dir, dirSt.Uid, uint32(gid))
		})
	}

	t.Run("closed", func(t *testing.T) {
		require.EqualErrno(t, 0, dirF.Close())
		require.EqualErrno(t, syscall.EBADF, dirF.Chown(-1, gid))
	})
}

func TestLchown(t *testing.T) {
	tmpDir := t.TempDir()

	dir := path.Join(tmpDir, "dir")
	require.NoError(t, os.Mkdir(dir, 0o0777))

	dirF := requireOpenFile(t, dir, syscall.O_RDONLY, 0)
	defer dirF.Close()

	dirSt, errno := dirF.Stat()
	require.EqualErrno(t, 0, errno)

	link := path.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(dir, link))

	linkF := requireOpenFile(t, link, syscall.O_RDONLY, 0)
	defer linkF.Close()

	linkSt, errno := linkF.Stat()
	require.EqualErrno(t, 0, errno)

	// Similar to TestLchown in os_unix_test.go, we can't expect to change
	// owner unless root, and with another user. Instead, test gid.
	gid := os.Getgid()
	groups, err := os.Getgroups()
	require.NoError(t, err)

	t.Run("-1 parameters means leave alone", func(t *testing.T) {
		require.EqualErrno(t, 0, Lchown(link, -1, -1))
		checkUidGid(t, link, linkSt.Uid, linkSt.Gid)
	})

	t.Run("change gid, but not uid", func(t *testing.T) {
		require.EqualErrno(t, 0, Chown(dir, -1, gid))
		checkUidGid(t, link, linkSt.Uid, uint32(gid))
		// Make sure the target didn't change.
		checkUidGid(t, dir, dirSt.Uid, dirSt.Gid)
	})

	// Now, try any other groups of the current user.
	for _, g := range groups {
		g := g
		t.Run(fmt.Sprintf("change to gid %d", g), func(t *testing.T) {
			// Test using our Lchown
			require.EqualErrno(t, 0, Lchown(link, -1, g))
			checkUidGid(t, link, linkSt.Uid, uint32(g))
			// Make sure the target didn't change.
			checkUidGid(t, dir, dirSt.Uid, dirSt.Gid)

			// Revert back
			require.EqualErrno(t, 0, Lchown(link, -1, gid))
			checkUidGid(t, link, linkSt.Uid, uint32(gid))
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
