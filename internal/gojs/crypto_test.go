package gojs_test

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/crypto/main.go
var cryptoGo string

func Test_crypto(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, cryptoGo, wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `7a0c9f9f0d
`, stdout)
}
