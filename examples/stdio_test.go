package examples

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
	"github.com/mathetake/gasm/wasm/naivevm"
)

func Test_stdio(t *testing.T) {
	buf, err := os.ReadFile("wasm/stdio.wasm")
	require.NoError(t, err)
	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)
	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	wasiEnv := wasi.NewEnvironment(
		wasi.Stdin(stdinBuf),
		wasi.Stdout(stdoutBuf),
		wasi.Stderr(stderrBuf),
	)
	store := wasm.NewStore(naivevm.NewEngine())
	err = wasiEnv.Register(store)
	require.NoError(t, err)
	err = store.Instantiate(mod, "test")
	require.NoError(t, err)
	_, _, err = store.CallFunction("test", "_start")
	require.NoError(t, err)
	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
