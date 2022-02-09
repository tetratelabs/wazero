package examples

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/text"
)

// Test_AddInt shows how you can define a function in text format and have it compiled inline.
// See https://github.com/summerwind/the-art-of-webassembly-go/blob/main/chapter1/addint/addint.wat
func Test_AddInt(t *testing.T) {
	ctx := context.Background()
	mod, err := text.DecodeModule([]byte(`(module
    (func $addInt ;; TODO: function exports (export "AddInt")
        (param $value_1 i32) (param $value_2 i32)
        (result i32)
        local.get 0 ;; TODO: instruction variables $value_1
        local.get 1 ;; TODO: instruction variables $value_2
        i32.add
    )
    (export "AddInt" (func $addInt))
)`))
	require.NoError(t, err)

	store := wasm.NewStore(interpreter.NewEngine())
	require.NoError(t, err)

	err = wasi.RegisterAPI(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	for _, c := range []struct {
		value1, value2, result uint64
	}{
		{value1: 1, value2: 2, result: 3},
		{value1: 5, value2: 5, result: 10},
	} {
		ret, retTypes, err := store.CallFunction(ctx, "test", "AddInt", c.value1, c.value2)
		require.NoError(t, err)
		require.Len(t, ret, len(retTypes))
		require.Equal(t, wasm.ValueTypeI32, retTypes[0])
		require.Equal(t, c.result, ret[0])
	}
}
