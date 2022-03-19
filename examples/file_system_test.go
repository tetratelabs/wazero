package examples

import (
	"bytes"
	_ "embed"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
)

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
	wasiConfig := wazero.NewWASIConfig().WithStdout(stdoutBuf)

	// Next, configure a sand-boxed filesystem to include one file.
	file := "cat.go" // arbitrary file
	memFS := wazero.WASIMemFS()
	err := writeFile(memFS, file, catGo)
	require.NoError(t, err)
	wasiConfig.WithPreopens(map[string]wasi.FS{".": memFS})

	// Since this runs a main function (_start in WASI), configure the arguments.
	// Remember, arg[0] is the program name!
	wasiConfig.WithArgs("cat", file)

	// Now, instantiate WASI with the above configuration.
	wasi, err := r.InstantiateModule(wazero.WASISnapshotPreview1WithConfig(wasiConfig))
	require.NoError(t, err)
	defer wasi.Close()

	// Finally, start the `cat` program's main function (in WASI, this is named `_start`).
	cat, err := wazero.StartWASICommandFromSource(r, catWasm)
	require.NoError(t, err)
	defer cat.Close()

	// To ensure it worked, verify stdout from WebAssembly had what we expected.
	require.Equal(t, string(catGo), stdoutBuf.String())
}

func writeFile(fs wasi.FS, path string, data []byte) error {
	f, err := fs.OpenWASI(0, path, wasi.O_CREATE|wasi.O_TRUNC, wasi.R_FD_WRITE, 0, 0)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, bytes.NewBuffer(data)); err != nil {
		return err
	}

	return f.Close()
}
