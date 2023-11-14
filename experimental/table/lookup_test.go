package table_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental/table"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestLookupFunction(t *testing.T) {
	const i32 = wasm.ValueTypeI32
	bytes := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Results: []wasm.ValueType{i32}},
			{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}},
		},
		FunctionSection: []wasm.Index{0, 1},
		CodeSection: []wasm.Code{
			{Body: []byte{
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeEnd,
			}},
			{Body: []byte{
				// Swap the two i32s params.
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeEnd,
			}},
		},
		ElementSection: []wasm.ElementSegment{
			{
				OffsetExpr: wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: []byte{0}},
				Init:       []wasm.Index{0, 1},
				Type:       wasm.RefTypeFuncref,
			},
		},
		TableSection: []wasm.Table{{Type: wasm.RefTypeFuncref, Min: 100}},
	})

	r := wazero.NewRuntime(context.Background())
	m, err := r.Instantiate(context.Background(), bytes)
	require.NoError(t, err)
	require.NotNil(t, m)

	t.Run("v_i32", func(t *testing.T) {
		f := table.LookupFunction(m, 0, 0, nil, []api.ValueType{i32})
		var result [1]uint64
		err = f.CallWithStack(context.Background(), result[:])
		require.NoError(t, err)
		require.Equal(t, uint64(1), result[0])
	})
	t.Run("i32i32_i32i32", func(t *testing.T) {
		f := table.LookupFunction(m, 0, 1, []api.ValueType{i32, i32}, []api.ValueType{i32, i32})
		stack := [2]uint64{100, 200}
		err = f.CallWithStack(context.Background(), stack[:])
		require.NoError(t, err)
		require.Equal(t, uint64(200), stack[0])
		require.Equal(t, uint64(100), stack[1])
	})

	t.Run("panics", func(t *testing.T) {
		err := require.CapturePanic(func() {
			table.LookupFunction(m, 0, 2000, nil, []api.ValueType{i32})
		})
		require.Equal(t, "invalid table access", err.Error())
		err = require.CapturePanic(func() {
			table.LookupFunction(m, 1000, 0, nil, []api.ValueType{i32})
		})
		require.Equal(t, "table index out of range", err.Error())
		err = require.CapturePanic(func() {
			table.LookupFunction(m, 0, 0, nil, []api.ValueType{api.ValueTypeF32})
		})
		require.Equal(t, "indirect call type mismatch", err.Error())
		err = require.CapturePanic(func() {
			table.LookupFunction(m, 0, 0, []api.ValueType{i32}, nil)
		})
		require.Equal(t, "indirect call type mismatch", err.Error())
		err = require.CapturePanic(func() {
			table.LookupFunction(m, 0, 1, []api.ValueType{i32, i32}, nil)
		})
		require.Equal(t, "indirect call type mismatch", err.Error())
		err = require.CapturePanic(func() {
			table.LookupFunction(m, 0, 1, []api.ValueType{i32, i32}, []api.ValueType{i32, api.ValueTypeF32})
		})
		require.Equal(t, "indirect call type mismatch", err.Error())
	})
}
