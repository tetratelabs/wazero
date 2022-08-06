package gojs_test

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/time/main.go
var timeGo string

func Test_time(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, timeGo, wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `Local
1ms
`, stdout)
}
