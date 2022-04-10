package examples

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/heeus/inv-wazero"
	"github.com/heeus/inv-wazero/wasi"
)

// fibWasm was compiled from TinyGo testdata/fibonacci.go
//go:embed testdata/fibonacci.wasm
var fibWasm []byte // TODO: implement this in text format as it is less distracting setup

func Test_fibonacci(t *testing.T) {
	r := wazero.NewRuntime()

	// Note: fibonacci.go doesn't directly use WASI, but TinyGo needs to be initialized as a WASI Command.
	wm, err := wasi.InstantiateSnapshotPreview1(r)
	require.NoError(t, err)
	defer wm.Close()

	module, err := r.InstantiateModuleFromCode(fibWasm)
	require.NoError(t, err)
	defer module.Close()

	fibonacci := module.ExportedFunction("fibonacci")

	for _, c := range []struct {
		input, expected uint64 // i32_i32 sig, but wasm.ExportedFunction params and results are uint64
	}{
		{input: 20, expected: 6765},
		{input: 10, expected: 55},
		{input: 5, expected: 5},
	} {
		results, err := fibonacci.Call(nil, c.input)
		require.NoError(t, err)
		require.Equal(t, c.expected, results[0])
	}
}
