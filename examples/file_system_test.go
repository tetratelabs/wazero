package examples

import (
	"bytes"
	"embed"
	_ "embed"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
)

// catFS is an embedded filesystem limited to cat.go
//go:embed testdata/cat.go
var catFS embed.FS

// catGo is the TinyGo source
//go:embed testdata/cat.go
var catGo []byte

// catWasm was compiled from catGo
//go:embed testdata/cat.wasm
var catWasm []byte

// Test_Cat writes the input file to stdout, just like `cat`.
//
// This is a basic introduction to the WebAssembly System Interface (WASI).
// See https://github.com/WebAssembly/WASI
func Test_Cat(t *testing.T) {
	r := wazero.NewRuntime()

	// First, configure where the WebAssembly Module (Wasm) console outputs to (stdout).
	stdoutBuf := bytes.NewBuffer(nil)
	sysConfig := wazero.NewSysConfig().WithStdout(stdoutBuf)

	// Since wazero uses fs.FS we can use standard libraries to do things like trim the leading path.
	rooted, err := fs.Sub(catFS, "testdata")
	require.NoError(t, err)

	// Since this runs a main function (_start in WASI), configure the arguments.
	// Remember, arg[0] is the program name!
	sysConfig = sysConfig.WithFS(rooted).WithArgs("cat", "/cat.go")

	// Compile the `cat` module.
	compiled, err := r.CompileModule(catWasm)
	require.NoError(t, err)

	// Instantiate WASI, which implements system I/O such as console output.
	wasi, err := r.InstantiateModule(wazero.WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	// StartWASICommand runs the "_start" function which is what TinyGo compiles "main" to.
	cat, err := wazero.StartWASICommandWithConfig(r, compiled, sysConfig)
	require.NoError(t, err)
	defer cat.Close()

	// To ensure it worked, verify stdout from WebAssembly had what we expected.
	require.Equal(t, catGo, stdoutBuf.Bytes())
}
