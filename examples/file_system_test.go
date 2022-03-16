package examples

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
)

// filesystemWasm was compiled from TinyGo testdata/file_system.go
//go:embed testdata/file_system.wasm
var filesystemWasm []byte

// Embed a directory as FS.
//go:embed testdata/file_system_input/input.txt
var embeddedFS embed.FS

func writeFile(t *testing.T, fs wasi.OpenFileFS, path string, data []byte) {
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(0644))
	require.NoError(t, err)

	// fs.File can be casted to io.Writer when it's writable.
	writer, ok := f.(io.Writer)
	require.True(t, ok)

	_, err = io.Copy(writer, bytes.NewBuffer(data))
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)
}

func Test_file_system(t *testing.T) {
	r := wazero.NewRuntime()

	// embeddedFS is a read-only embed.FS file system. Get the sub FS to shorten the nested directories.
	embeddedFS, err := fs.Sub(embeddedFS, "testdata/file_system_input")
	require.NoError(t, err)

	// WASIMemFS returns a in-memory file system that supports both read and write.
	memFS := wazero.WASIMemFS()
	// memFS is writable. Create another input file.
	writeFile(t, memFS, "input.txt", []byte("Hello, "))

	// Configure what resources you share with the WASI program.
	wasiConfig := wazero.NewWASIConfig()
	// Share fs.FS as pre-opened directories
	wasiConfig = wasiConfig.WithPreopens(
		map[string]fs.FS{
			".":      memFS,      // pre-open the writable in-memory directory at the working directory
			"/input": embeddedFS, // pre-open the embeddedFS at "/input"
		},
	)
	// the wasm module will concatenate the contents of the two input.txt and write to output.txt
	wasiConfig = wasiConfig.WithArgs("./input.txt", "/input/input.txt", "./output.txt")

	wasi, err := r.InstantiateModule(wazero.WASISnapshotPreview1WithConfig(wasiConfig))
	require.NoError(t, err)
	defer wasi.Close()

	// Note: TinyGo binaries must be treated as WASI Commands to initialize memory.
	mod, err := wazero.StartWASICommandFromSource(r, filesystemWasm)
	require.NoError(t, err)
	defer mod.Close()

	// "input.txt" in the MemFS has "Hello, ", and "/testdata/file_system_input/input.txt" has "file system!\n".
	// So, the result "output.txt" should contain "Hello, file system!\n".
	out, err := fs.ReadFile(memFS, "output.txt")
	require.NoError(t, err)
	require.Equal(t, "Hello, file system!", string(out))
}
