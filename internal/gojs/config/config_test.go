package config

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestConfig_Init(t *testing.T) {
	t.Parallel()

	t.Run("OsWorkdir", func(t *testing.T) {
		c := &Config{OsWorkdir: true}
		require.NoError(t, c.Init())
		actual := c.Workdir

		// Check c:\ or d:\ aren't retained.
		require.Equal(t, -1, strings.IndexByte(actual, '\\'))
		require.Equal(t, -1, strings.IndexByte(actual, ':'))
	})
}
