package gojs_test

import (
	"bytes"
	"fmt"
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

	input := "stdin\n"
	stdout, stderr, err := compileAndRun(testCtx, "stdio", wazero.NewModuleConfig().
		WithStdin(strings.NewReader(input)))

	require.Equal(t, "stderr 6\n", stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "stdout 6\n", stdout)
}

func Test_stdio_large(t *testing.T) {
	t.Parallel()

	size := 2 * 1024 * 1024 // 2MB
	input := make([]byte, size)
	stdout, stderr, err := compileAndRun(testCtx, "stdio", wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(input)))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, fmt.Sprintf("stderr %d\n", size), stderr)
	require.Equal(t, fmt.Sprintf("stdout %d\n", size), stdout)
}

func Test_gc(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "gc", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "", stderr)
	require.Equal(t, "before gc\nafter gc\n", stdout)
}
