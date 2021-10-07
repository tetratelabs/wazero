package examples

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func Test_stdio(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/stdio.wasm")
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
	vm, err := wasm.NewVM()
	require.NoError(t, err)
	err = wasiEnv.RegisterToVirtualMachine(vm)
	require.NoError(t, err)
	err = vm.InstantiateModule(mod, "test")
	require.NoError(t, err)
	_, _, err = vm.ExecExportedFunction("test", "_start")
	require.NoError(t, err)
	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
