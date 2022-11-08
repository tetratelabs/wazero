package gojs_test

import (
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_goroutine(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "goroutine", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `producer
consumer
`, stdout)
}

func Test_mem(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "mem", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Zero(t, stdout)
}

func Test_stdio(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "stdio", wazero.NewModuleConfig().
		WithStdin(strings.NewReader("stdin\n")))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "println stdin\nStderr.Write", stderr)
	require.Equal(t, "Stdout.Write", stdout)
}

func Test_gc(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "gc", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "", stderr)
	require.Equal(t, "before gc\nafter gc\n", stdout)
}
