package gojs_test

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/goroutine/main.go
var goroutineGo string

func Test_goroutine(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, goroutineGo, wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `producer
consumer
`, stdout)
}

//go:embed testdata/mem/main.go
var memGo string

func Test_mem(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, memGo, wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Zero(t, stdout)
}

//go:embed testdata/stdio/main.go
var stdioGo string

func Test_stdio(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, stdioGo, wazero.NewModuleConfig().
		WithStdin(strings.NewReader("stdin\n")))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "println stdin\nStderr.Write", stderr)
	require.Equal(t, "Stdout.Write", stdout)
}
