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

// filesystemWasm was compiled from TinyGo testdata/file_system.go
//go:embed testdata/file_system.wasm
var filesystemWasm []byte

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

func readFile(fs wasi.FS, path string) ([]byte, error) {
	f, err := fs.OpenWASI(0, path, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)

	if _, err := io.Copy(buf, f); err != nil {
		return buf.Bytes(), nil
	}

	return buf.Bytes(), f.Close()
}

func Test_file_system(t *testing.T) {
	mod, err := wazero.DecodeModuleBinary(filesystemWasm)
	require.NoError(t, err)

	memFS := wazero.WASIMemFS()
	err = writeFile(memFS, "input.txt", []byte("Hello, file system!"))
	require.NoError(t, err)

	// Configure WASI host functions with the memory filesystem
	store, err := wazero.NewStoreWithConfig(&wazero.StoreConfig{
		ModuleToHostFunctions: map[string]*wazero.HostFunctions{
			wasi.ModuleSnapshotPreview1: wazero.WASISnapshotPreview1WithConfig(
				&wazero.WASIConfig{Preopens: map[string]wasi.FS{".": memFS}},
			),
		},
	})
	require.NoError(t, err)

	// Note: TinyGo binaries must be treated as WASI Commands to initialize memory.
	_, err = wazero.StartWASICommand(store, mod)
	require.NoError(t, err)

	out, err := readFile(memFS, "output.txt")
	require.NoError(t, err)
	require.Equal(t, "Hello, file system!", string(out))
}
