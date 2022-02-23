package examples

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero"
)

// Test_AddInt shows how you can define a function in text format and have it compiled inline.
// See https://github.com/summerwind/the-art-of-webassembly-go/blob/main/chapter1/addint/addint.wat
func Test_AddInt(t *testing.T) {
	exports, err := wazero.InstantiateModule(wazero.NewStore(), &wazero.ModuleConfig{Source: []byte(`(module $test
    (func $addInt ;; TODO: function exports (export "AddInt")
        (param $value_1 i32) (param $value_2 i32)
        (result i32)
        local.get 0 ;; TODO: instruction variables $value_1
        local.get 1 ;; TODO: instruction variables $value_2
        i32.add
    )
    (export "AddInt" (func $addInt))
)`)})
	require.NoError(t, err)

	addInt := exports.Function("AddInt")

	for _, c := range []struct {
		value1, value2, expected uint64 // i32i32_i32 sig, but wasm.Function params and results are uint64
	}{
		{value1: 1, value2: 2, expected: 3},
		{value1: 5, value2: 5, expected: 10},
	} {
		results, err := addInt(context.Background(), c.value1, c.value2)
		require.NoError(t, err)
		require.Equal(t, c.expected, results[0])
	}
}
