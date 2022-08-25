package gojs_test

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_time(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "time", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `Local
1ms
`, stdout)
}
