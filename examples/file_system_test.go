package examples

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
	"github.com/stretchr/testify/require"
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
	buf, err := ioutil.ReadFile("wasm/file_system.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	memFS := wasi.MemFS()
	err = writeFile(memFS, "input.txt", []byte("Hello, file system!"))
	require.NoError(t, err)

	vm, err := wasm.NewVM(mod, wasi.New(wasi.Preopen(".", memFS)).Modules())
	require.NoError(t, err)

	_, _, err = vm.ExecExportedFunction("_start")
	require.NoError(t, err)

	out, err := readFile(memFS, "output.txt")
	require.NoError(t, err)
	require.Equal(t, "Hello, file system!", string(out))
}
