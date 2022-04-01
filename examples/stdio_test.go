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

	// Compile the `stdioWasm` module.
	code, err := r.CompileModule(stdioWasm)
	require.NoError(t, err)

	// Configure standard I/O (ex stdout) to write to buffers instead of no-op.
	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	config := wazero.NewModuleConfig().WithStdin(stdinBuf).WithStdout(stdoutBuf).WithStderr(stderrBuf)

	// Instantiate WASI, which implements system I/O such as console output.
	wasi, err := r.InstantiateModule(wazero.WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	// InstantiateModuleWithConfig runs the "_start" function which is what TinyGo compiles "main" to.
	module, err := r.InstantiateModuleWithConfig(code, config)
	require.NoError(t, err)
	defer module.Close()

	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
