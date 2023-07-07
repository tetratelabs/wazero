package config

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestConfig_Init(t *testing.T) {
	t.Parallel()

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
