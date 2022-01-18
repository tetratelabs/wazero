package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionType_String(t *testing.T) {
	tp := FunctionType{}
	require.Equal(t, "null_null", tp.String())

	// With params.
	tp = FunctionType{Params: []ValueType{ValueTypeI32}}
	require.Equal(t, "i32_null", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "i32f64_null", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "f32i32f64_null", tp.String())

	// With results.
	tp = FunctionType{Results: []ValueType{ValueTypeI64}}
	require.Equal(t, "null_i64", tp.String())
	tp = FunctionType{Results: []ValueType{ValueTypeI64, ValueTypeF32}}
	require.Equal(t, "null_i64f32", tp.String())
	tp = FunctionType{Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "null_f32i32f64", tp.String())

	// With params and results.
	tp = FunctionType{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}}
	require.Equal(t, "i32_i64", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32}, Results: []ValueType{ValueTypeI64, ValueTypeF32}}
	require.Equal(t, "i64f32_i64f32", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32, ValueTypeF64}, Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "i64f32f64_f32i32f64", tp.String())
}
