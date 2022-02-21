package examples

import (
	"bytes"
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
)

// stdioWasm was compiled from TinyGo testdata/stdio.go
//go:embed testdata/stdio.wasm
var stdioWasm []byte

func Test_stdio(t *testing.T) {
	mod, err := wazero.DecodeModuleBinary(stdioWasm)
	require.NoError(t, err)
	store := wazero.NewStore()

	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	// Configure WASI host functions with the IO buffers
	wasiConfig := &wazero.WASIConfig{Stdin: stdinBuf, Stdout: stdoutBuf, Stderr: stderrBuf}
	_, err = wazero.ExportHostFunctions(store, wasi.ModuleSnapshotPreview1, wazero.WASISnapshotPreview1WithConfig(wasiConfig))
	require.NoError(t, err)

	// StartWASICommand runs the "_start" function which is what TinyGo compiles "main" to
	_, err = wazero.StartWASICommand(store, mod)
	require.NoError(t, err)

	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
