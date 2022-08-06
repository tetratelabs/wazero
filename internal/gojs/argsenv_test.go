package gojs_test

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/argsenv/main.go
var argsenvGo string

func Test_argsAndEnv(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, argsenvGo, wazero.NewModuleConfig().WithArgs("prog", "a", "b").WithEnv("c", "d").WithEnv("a", "b"))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `
args 0 = a
args 1 = b
environ 0 = c=d
environ 1 = a=b
`, stdout)
}
