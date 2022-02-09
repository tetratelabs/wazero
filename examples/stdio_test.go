package examples

import (
	"bytes"
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
)

//go:embed testdata/stdio.wasm
var stdioWasm []byte

func Test_stdio(t *testing.T) {
	ctx := context.Background()
	mod, err := binary.DecodeModule(stdioWasm)
	require.NoError(t, err)
	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	store := wasm.NewStore(interpreter.NewEngine())
	err = wasi.RegisterAPI(store,
		wasi.Stdin(stdinBuf),
		wasi.Stdout(stdoutBuf),
		wasi.Stderr(stderrBuf),
	)
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	_, _, err = store.CallFunction(ctx, "test", "_start")
	require.NoError(t, err)
	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
