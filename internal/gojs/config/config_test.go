package config

import (
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestConfig_Init(t *testing.T) {
	t.Parallel()

	t.Run("User", func(t *testing.T) {
		c := NewConfig()

		// values should be 0 which is root
		require.Equal(t, 0, c.Uid)
		require.Equal(t, 0, c.Gid)
		require.Equal(t, 0, c.Euid)
		require.Equal(t, []int{0}, c.Groups)
		require.False(t, c.OsUser)

		if runtime.GOOS != "windows" {
			c.OsUser = true
			require.NoError(t, c.Init())

			require.Equal(t, syscall.Getuid(), c.Uid)
			require.Equal(t, syscall.Getgid(), c.Gid)
			require.Equal(t, syscall.Geteuid(), c.Euid)

			groups, err := syscall.Getgroups()
			require.NoError(t, err)
			require.Equal(t, groups, c.Groups)
		}
	})

	t.Run("Workdir", func(t *testing.T) {
		c := NewConfig()
		require.Equal(t, "/", c.Workdir)
		require.False(t, c.OsWorkdir)

		c.OsWorkdir = true

		require.NoError(t, c.Init())
		actual := c.Workdir

		// Check c:\ or d:\ aren't retained.
		require.Equal(t, -1, strings.IndexByte(actual, '\\'))
		require.Equal(t, -1, strings.IndexByte(actual, ':'))
	})
}
