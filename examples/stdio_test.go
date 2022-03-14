package examples

import (
	"bytes"
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
)

// stdioWasm was compiled from TinyGo testdata/stdio.go
//go:embed testdata/stdio.wasm
var stdioWasm []byte

func Test_stdio(t *testing.T) {
	r := wazero.NewRuntime()

	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)

	// Configure WASI host functions with the IO buffers
	wasiConfig := wazero.NewWASIConfig().WithStdin(stdinBuf).WithStdout(stdoutBuf).WithStderr(stderrBuf)
	wasi, err := r.InstantiateModule(wazero.WASISnapshotPreview1WithConfig(wasiConfig))
	require.NoError(t, err)
	defer wasi.Close()

	// StartWASICommand runs the "_start" function which is what TinyGo compiles "main" to
	mod, err := wazero.StartWASICommandFromSource(r, stdioWasm)
	require.NoError(t, err)
	defer mod.Close()

	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
