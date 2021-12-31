package examples

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

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
	buf, err := os.ReadFile("testdata/file_system.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(buf)
	require.NoError(t, err)

	memFS := wasi.MemFS()
	err = writeFile(memFS, "input.txt", []byte("Hello, file system!"))
	require.NoError(t, err)

	wasiEnv := wasi.NewEnvironment(wasi.Preopen(".", memFS))

	store := wasm.NewStore(wazeroir.NewEngine())

	err = wasiEnv.Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	_, _, err = store.CallFunction("test", "_start")
	require.NoError(t, err)

	out, err := readFile(memFS, "output.txt")
	require.NoError(t, err)
	require.Equal(t, "Hello, file system!", string(out))
}
