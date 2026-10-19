package adhoc

import (
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestCopysignSelectRegalloc is a regression test for #2555 where an amd64 compiler
// operand lowering bug caused copysign with a select operand (or spilled operand)
// to overwrite sign bit mask registers, corrupting the sign of the result.
func TestCopysignSelectRegalloc(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())
	defer r.Close(ctx)

	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			{
				Params:            []wasm.ValueType{wasm.ValueTypeF32},
				Results:           []wasm.ValueType{wasm.ValueTypeF32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 1,
			},
			{
				Params:            []wasm.ValueType{wasm.ValueTypeF64},
				Results:           []wasm.ValueType{wasm.ValueTypeF64},
				ParamNumInUint64:  1,
				ResultNumInUint64: 1,
			},
		},
		FunctionSection: []wasm.Index{0, 0, 1, 1},
		ExportSection: []wasm.Export{
			{Name: "f32", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "f64", Type: wasm.ExternTypeFunc, Index: 3},
		},
		CodeSection: []wasm.Code{
			// func 0: dummy f32
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}},
			// func 1: f32 repro
			{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeCall, 0,
				wasm.OpcodeLocalSet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeCall, 0,
				wasm.OpcodeF32Const, 0, 0, 0, 0,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeSelect,
				wasm.OpcodeF32Copysign,
				wasm.OpcodeEnd,
			}},
			// func 2: dummy f64
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}},
			// func 3: f64 repro
			{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeCall, 2,
				wasm.OpcodeLocalSet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeCall, 2,
				wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0, 0,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeSelect,
				wasm.OpcodeF64Copysign,
				wasm.OpcodeEnd,
			}},
		},
	})

	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)
	defer mod.Close(ctx)

	f32Fn := mod.ExportedFunction("f32")
	require.NotNil(t, f32Fn)

	res32, err := f32Fn.Call(ctx, api.EncodeF32(1.5))
	require.NoError(t, err)
	require.Equal(t, float32(1.5), api.DecodeF32(res32[0]))
	require.False(t, math.Signbit(float64(api.DecodeF32(res32[0]))))

	f64Fn := mod.ExportedFunction("f64")
	require.NotNil(t, f64Fn)

	res64, err := f64Fn.Call(ctx, api.EncodeF64(1.5))
	require.NoError(t, err)
	require.Equal(t, 1.5, api.DecodeF64(res64[0]))
	require.False(t, math.Signbit(api.DecodeF64(res64[0])))
}
