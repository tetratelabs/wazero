package cranelift

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_getEntryPoint(t *testing.T) {
	for _, tc := range []struct {
		tp  *wasm.FunctionType
		exp entryPointFn
	}{
		{
			tp:  &wasm.FunctionType{},
			exp: entryPointNoParamNoResult,
		},
		// No params.
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{i32}},
			exp: entryPointNoParamI32Result,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{i32, f32}},
			exp: entryPointNoParamI32PlusMultiResult,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{i64}},
			exp: entryPointNoParamI64Result,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{i64, i32, f32}},
			exp: entryPointNoParamI64PlusMultiResult,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{f32}},
			exp: entryPointNoParamF32Result,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{f32, i32, f32}},
			exp: entryPointNoParamF32PlusMultiResult,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{f64}},
			exp: entryPointNoParamF64Result,
		},
		{
			tp:  &wasm.FunctionType{Results: []wasm.ValueType{f64, i32, f32}},
			exp: entryPointNoParamF64PlusMultiResult,
		},
		// With Params.
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}},
			exp: entryPointWithParamI32Result,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32, f32}},
			exp: entryPointWithParamI32PlusMultiResult,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i64}},
			exp: entryPointWithParamI64Result,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i64, i32, f32}},
			exp: entryPointWithParamI64PlusMultiResult,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{f32}},
			exp: entryPointWithParamF32Result,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{f32, i32, f32}},
			exp: entryPointWithParamF32PlusMultiResult,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{f64}},
			exp: entryPointWithParamF64Result,
		},
		{
			tp:  &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{f64, i32, f32}},
			exp: entryPointWithParamF64PlusMultiResult,
		},
	} {
		require.Equal(t, tc.exp, getEntryPoint(tc.tp))
	}
}
