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
	r := wazero.NewRuntime()

	// Configure standard I/O (ex stdout) to write to buffers instead of no-op.
	stdinBuf := bytes.NewBuffer([]byte("WASI\n"))
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	config := wazero.NewModuleConfig().WithStdin(stdinBuf).WithStdout(stdoutBuf).WithStderr(stderrBuf)

	// Instantiate WASI, which implements system I/O such as console output.
	wm, err := wasi.InstantiateSnapshotPreview1(r)
	require.NoError(t, err)
	defer wm.Close()

	// InstantiateModuleFromCodeWithConfig runs the "_start" function which is what TinyGo compiles "main" to.
	module, err := r.InstantiateModuleFromCodeWithConfig(stdioWasm, config)
	require.NoError(t, err)
	defer module.Close()

	require.Equal(t, "Hello, WASI!", strings.TrimSpace(stdoutBuf.String()))
	require.Equal(t, "Error Message", strings.TrimSpace(stderrBuf.String()))
}
