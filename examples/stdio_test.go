package examples

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
	"github.com/stretchr/testify/require"
)

func Test_stdio(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/stdio.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	vm, err := wasm.NewVM(mod, wasi.New(
		wasi.Stdin(stdinBuf),
		wasi.Stdout(stdoutBuf),
		wasi.Stderr(stderrBuf),
	).Modules())
	require.NoError(t, err)

	_, _, err = vm.ExecExportedFunction("_start")
	require.NoError(t, err)
	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
