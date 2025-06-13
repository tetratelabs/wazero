package adhoc

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestE2E_tail_call_import implements a test case similar to testcases.TailCallManyParams,
// exercising the fall back to a traditional call in case of a large number of parameters,
// but the tail call invokes a host function.
func TestE2E_tail_call_import(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		cfg  wazero.RuntimeConfig
	}{
		{"interpreter", wazero.NewRuntimeConfigInterpreter()},
		{"default", wazero.NewRuntimeConfig()},
	} {

		config := tc.cfg.WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesTailCall)

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntimeWithConfig(ctx, config)
			defer func() {
				require.NoError(t, r.Close(ctx))
			}()

			var expectedMod api.Module

			b := r.NewHostModuleBuilder("env")
			b.NewFunctionBuilder().WithFunc(func(ctx2 context.Context, mod api.Module, a, b, c, d, e, f, g, h int32) int32 {
				require.Equal(t, expectedMod, mod)
				require.Equal(t, ctx, ctx2)
				return a + b + c + d + e + f + g + h
			}).Export("imported_callee")

			_, err := b.Instantiate(ctx)
			require.NoError(t, err)

			m := &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},      // type 0: (i32 x 7) -> i32
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32, i32}, Results: []wasm.ValueType{i32}}, // type 1: (i32 x 8) -> i32 (for imported function)
				},
				ImportFunctionCount: 1,
				ImportSection: []wasm.Import{{
					Module:   "env",
					Name:     "imported_callee",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 1, // Points to the type in TypeSection
				}},
				FunctionSection: []wasm.Index{0, 0}, // entry (type 0), caller (type 0)
				CodeSection: []wasm.Code{
					{ // entry(a,b,c,d,e,f,g) -> caller(a,b,c,d,e,f,g)
						Body: []byte{
							wasm.OpcodeLocalGet, 0, // a
							wasm.OpcodeLocalGet, 1, // b
							wasm.OpcodeLocalGet, 2, // c
							wasm.OpcodeLocalGet, 3, // d
							wasm.OpcodeLocalGet, 4, // e
							wasm.OpcodeLocalGet, 5, // f
							wasm.OpcodeLocalGet, 6, // g
							wasm.OpcodeCall, 2, // call caller(a,b,c,d,e,f,g)
							wasm.OpcodeEnd,
						},
					},
					// The following reproduces a similar case found in the SQLite codebase
					{ // caller(a,b,c,d,e,f,g) -> tail call imported_callee(a,b,c,d|128,0,e,f,g)
						Body: []byte{
							wasm.OpcodeLocalGet, 0, // a
							wasm.OpcodeLocalGet, 1, // b
							wasm.OpcodeLocalGet, 2, // c
							wasm.OpcodeLocalGet, 3, // d
							wasm.OpcodeI32Const, 31, // const 31 (like SQLite)
							wasm.OpcodeI32And,            // d & 31
							wasm.OpcodeI32Const, 0x80, 1, // const 128 (like SQLite)
							wasm.OpcodeI32Or,       // (d & 31) | 128
							wasm.OpcodeI32Const, 0, // const 0 (like SQLite)
							wasm.OpcodeLocalGet, 4, // e
							wasm.OpcodeLocalGet, 5, // f
							wasm.OpcodeLocalGet, 6, // g
							wasm.OpcodeTailCallReturnCall, 0, // tail call imported_callee (index 0 in imported functions)
							wasm.OpcodeEnd,
						},
					},
				},
				ExportSection: []wasm.Export{{
					Name:  testcases.ExportedFunctionName,
					Type:  wasm.ExternTypeFunc,
					Index: 1, // entry function (index 0 is imported function)
				}},
			}

			compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(m))
			require.NoError(t, err)

			inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
			require.NoError(t, err)

			expectedMod = inst

			f := inst.ExportedFunction(testcases.ExportedFunctionName)
			require.NotNil(t, f)

			// entry(1,2,3,4,5,6,7) -> caller(1,2,3,4,5,6,7) -> imported_callee(1,2,3,132,0,5,6,7) = 1+2+3+132+0+5+6+7 = 156
			res, err := f.Call(ctx, 1, 2, 3, 4, 5, 6, 7)
			require.NoError(t, err)
			require.Equal(t, []uint64{156}, res)
		})
	}
}

// TestE2E_tail_call_import_indirect implements a test case similar to testcases.TailCallManyParams,
// exercising the fall back to a traditional call in case of a large number of parameters,
// but the tail call invokes a host function and it is an indirect call.
func TestE2E_tail_call_import_indirect(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		cfg  wazero.RuntimeConfig
	}{
		{"interpreter", wazero.NewRuntimeConfigInterpreter()},
		{"default", wazero.NewRuntimeConfig()},
	} {

		config := tc.cfg.WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesTailCall)

		t.Run(tc.name, func(t *testing.T) {
			r := wazero.NewRuntimeWithConfig(ctx, config)
			defer func() {
				require.NoError(t, r.Close(ctx))
			}()

			var expectedMod api.Module

			b := r.NewHostModuleBuilder("env")
			b.NewFunctionBuilder().WithFunc(func(ctx2 context.Context, mod api.Module, a, b, c, d, e, f, g, h int32) int32 {
				require.Equal(t, expectedMod, mod)
				require.Equal(t, ctx, ctx2)
				return a + b + c + d + e + f + g + h
			}).Export("imported_callee")

			_, err := b.Instantiate(ctx)
			require.NoError(t, err)

			// Then define the main module
			m := &wasm.Module{
				TypeSection: []wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},      // type 0: (i32 x 7) -> i32
					{Params: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32, i32}, Results: []wasm.ValueType{i32}}, // type 1: (i32 x 8) -> i32 (for imported function)
				},
				ImportFunctionCount: 1,
				ImportSection: []wasm.Import{{
					Module:   "env",
					Name:     "imported_callee",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 1, // Points to the type in TypeSection
				}},
				FunctionSection: []wasm.Index{0, 0}, // entry (type 0), caller (type 0)
				TableSection: []wasm.Table{{
					Type: wasm.RefTypeFuncref,
					Min:  1, Max: &[]uint32{1}[0], // Table with 1 element
				}},
				ElementSection: []wasm.ElementSegment{{
					OffsetExpr: wasm.ConstantExpression{
						Opcode: wasm.OpcodeI32Const,
						Data:   []byte{0}, // Initialize at index 0
					},
					Init: []wasm.Index{0}, // Put imported function (index 0) at table[0]
					Mode: wasm.ElementModeActive,
				}},
				CodeSection: []wasm.Code{
					{ // entry(a,b,c,d,e,f,g) -> caller(a,b,c,d,e,f,g)
						Body: []byte{
							wasm.OpcodeLocalGet, 0, // a
							wasm.OpcodeLocalGet, 1, // b
							wasm.OpcodeLocalGet, 2, // c
							wasm.OpcodeLocalGet, 3, // d
							wasm.OpcodeLocalGet, 4, // e
							wasm.OpcodeLocalGet, 5, // f
							wasm.OpcodeLocalGet, 6, // g
							wasm.OpcodeCall, 2, // call caller(a,b,c,d,e,f,g)
							wasm.OpcodeEnd,
						},
					},
					// The following reproduces a similar case found in the SQLite codebase
					{ // caller(a,b,c,d,e,f,g) -> tail call indirect imported_callee(a,b,c,d|128,0,e,f,g)
						Body: []byte{
							wasm.OpcodeLocalGet, 0, // a
							wasm.OpcodeLocalGet, 1, // b
							wasm.OpcodeLocalGet, 2, // c
							wasm.OpcodeLocalGet, 3, // d
							wasm.OpcodeI32Const, 31, // const 31 (like SQLite)
							wasm.OpcodeI32And,            // d & 31
							wasm.OpcodeI32Const, 0x80, 1, // const 128 (like SQLite)
							wasm.OpcodeI32Or,       // (d & 31) | 128
							wasm.OpcodeI32Const, 0, // const 0 (like SQLite)
							wasm.OpcodeLocalGet, 4, // e
							wasm.OpcodeLocalGet, 5, // f
							wasm.OpcodeLocalGet, 6, // g
							wasm.OpcodeI32Const, 0, // table index 0
							wasm.OpcodeTailCallReturnCallIndirect, 1, 0, // tail call indirect type 1, table 0
							wasm.OpcodeEnd,
						},
					},
				},
				ExportSection: []wasm.Export{{
					Name:  testcases.ExportedFunctionName,
					Type:  wasm.ExternTypeFunc,
					Index: 1, // entry function (index 0 is imported function)
				}},
			}

			compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(m))
			require.NoError(t, err)

			inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
			require.NoError(t, err)

			expectedMod = inst

			f := inst.ExportedFunction(testcases.ExportedFunctionName)
			require.NotNil(t, f)

			// Test with some sample values
			// entry(1,2,3,4,5,6,7) -> caller(1,2,3,4,5,6,7) -> imported_callee(1,2,3,132,0,5,6,7) = 1+2+3+132+0+5+6+7 = 156
			res, err := f.Call(ctx, 1, 2, 3, 4, 5, 6, 7)
			require.NoError(t, err)
			require.Equal(t, []uint64{156}, res)
		})
	}
}
