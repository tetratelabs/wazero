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

	// Since wazero uses fs.FS, we can use standard libraries to do things like trim the leading path.
	rooted, err := fs.Sub(catFS, "testdata")
	require.NoError(t, err)

	// Combine the above into our baseline config, overriding defaults (which discard stdout and have no file system).
	config := wazero.NewModuleConfig().WithStdout(stdoutBuf).WithFS(rooted)

	// Compile the `cat` module.
	code, err := r.CompileModule(catWasm)
	require.NoError(t, err)

	// Instantiate WASI, which implements system I/O such as console output.
	wasi, err := r.InstantiateModule(wazero.WASISnapshotPreview1())
	require.NoError(t, err)
	defer wasi.Close()

	// InstantiateModuleWithConfig by default runs the "_start" function which is what TinyGo compiles "main" to.
	// * Set the program name (arg[0]) to "cat" and add args to write "cat.go" to stdout twice.
	// * We use both "/cat.go" and "./cat.go" because WithFS by default maps the workdir "." to "/".
	cat, err := r.InstantiateModuleWithConfig(code, config.WithArgs("cat", "/cat.go", "./cat.go"))
	require.NoError(t, err)
	defer cat.Close()

	// We expect the WebAssembly function wrote "cat.go" twice!
	require.Equal(t, append(catGo, catGo...), stdoutBuf.Bytes())
}
