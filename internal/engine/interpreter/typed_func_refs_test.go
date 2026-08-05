package interpreter

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestCompile_LocalSetWithMultipleLocals_I32 tests that local.set depth
// calculations are correct with param + 2 locals (i32 baseline).
func TestCompile_LocalSetWithMultipleLocals_I32(t *testing.T) {
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{i32_i32},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{{
			LocalTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
			Body: []byte{
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeLocalSet, 1,
				wasm.OpcodeI32Const, 20,
				wasm.OpcodeLocalSet, 2,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			},
		}},
	}
	for _, tp := range module.TypeSection {
		tp.CacheNumInUint64()
	}
	c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
	require.NoError(t, err)

	result, err := c.Next()
	require.NoError(t, err)

	require.Equal(t, []unionOperation{
		newOperationConstI32(0),                            // default local 1
		newOperationConstI32(0),                            // default local 2
		newOperationConstI32(10),                           // i32.const 10
		newOperationSet(2, false),                          // local.set 1
		newOperationConstI32(20),                           // i32.const 20
		newOperationSet(1, false),                          // local.set 2
		newOperationPick(1, false),                         // local.get 1
		newOperationPick(1, false),                         // local.get 2
		newOperationAdd(unsignedTypeI32),                   // i32.add
		newOperationDrop(inclusiveRange{Start: 1, End: 3}), // drop locals+param, keep result
		newOperationBr(newLabel(labelKindReturn, 0)),       // return
	}, result.Operations)
}

// TestCompile_LocalSetWithMultipleConcreteRefLocals tests that local.set depth
// calculations are correct with param + 2 concrete ref locals.
func TestCompile_LocalSetWithMultipleConcreteRefLocals(t *testing.T) {
	concreteRefNullable := wasm.ValueTypeConcreteRef(0, true)
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{i32_i32},
		FunctionSection: []wasm.Index{0, 0, 0},
		CodeSection: []wasm.Code{
			{
				LocalTypes: []wasm.ValueType{concreteRefNullable, concreteRefNullable},
				Body: []byte{
					wasm.OpcodeRefFunc, 1,
					wasm.OpcodeLocalSet, 1,
					wasm.OpcodeRefFunc, 2,
					wasm.OpcodeLocalSet, 2,
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeLocalGet, 1,
					wasm.OpcodeCallRef, 0,
					wasm.OpcodeLocalGet, 2,
					wasm.OpcodeCallRef, 0,
					wasm.OpcodeEnd,
				},
			},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeI32Const, 42, wasm.OpcodeEnd}},
		},
	}
	for i := range module.TypeSection {
		module.TypeSection[i].CacheNumInUint64()
	}

	features := api.CoreFeaturesV2 | experimental.CoreFeaturesTypedFunctionReferences | experimental.CoreFeaturesTailCall
	c, err := newCompiler(features, 0, module, false)
	require.NoError(t, err)

	result, err := c.Next()
	require.NoError(t, err)

	require.Equal(t, []unionOperation{
		newOperationConstI64(0),                            // default local 1 (concrete ref)
		newOperationConstI64(0),                            // default local 2 (concrete ref)
		newOperationRefFunc(1),                             // ref.func 1
		newOperationSet(2, false),                          // local.set 1
		newOperationRefFunc(2),                             // ref.func 2
		newOperationSet(1, false),                          // local.set 2
		newOperationPick(2, false),                         // local.get 0 (param)
		newOperationPick(2, false),                         // local.get 1
		newOperationCallRef(0),                             // call_ref 0
		newOperationPick(1, false),                         // local.get 2
		newOperationCallRef(0),                             // call_ref 0
		newOperationDrop(inclusiveRange{Start: 1, End: 3}), // drop locals+param, keep result
		newOperationBr(newLabel(labelKindReturn, 0)),       // return
	}, result.Operations)
}

// TestCompile_LocalSetWithMultipleFuncrefLocals tests local.set with
// funcref locals (not concrete refs) as a comparison.
func TestCompile_LocalSetWithMultipleFuncrefLocals(t *testing.T) {
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{i32_i32},
		FunctionSection: []wasm.Index{0, 0, 0},
		CodeSection: []wasm.Code{
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeFuncref, wasm.ValueTypeFuncref},
				Body: []byte{
					wasm.OpcodeRefFunc, 1,
					wasm.OpcodeLocalSet, 1,
					wasm.OpcodeRefFunc, 2,
					wasm.OpcodeLocalSet, 2,
					wasm.OpcodeLocalGet, 1,
					wasm.OpcodeRefIsNull,
					wasm.OpcodeLocalGet, 2,
					wasm.OpcodeRefIsNull,
					wasm.OpcodeI32Add,
					wasm.OpcodeEnd,
				},
			},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeI32Const, 42, wasm.OpcodeEnd}},
		},
	}
	for i := range module.TypeSection {
		module.TypeSection[i].CacheNumInUint64()
	}
	c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
	require.NoError(t, err)

	result, err := c.Next()
	require.NoError(t, err)

	require.Equal(t, []unionOperation{
		newOperationConstI64(0),                            // default local 1 (funcref)
		newOperationConstI64(0),                            // default local 2 (funcref)
		newOperationRefFunc(1),                             // ref.func 1
		newOperationSet(2, false),                          // local.set 1
		newOperationRefFunc(2),                             // ref.func 2
		newOperationSet(1, false),                          // local.set 2
		newOperationPick(1, false),                         // local.get 1
		newOperationEqz(unsignedInt64),                     // ref.is_null
		newOperationPick(1, false),                         // local.get 2
		newOperationEqz(unsignedInt64),                     // ref.is_null
		newOperationAdd(unsignedTypeI32),                   // i32.add
		newOperationDrop(inclusiveRange{Start: 1, End: 3}), // drop locals+param, keep result
		newOperationBr(newLabel(labelKindReturn, 0)),       // return
	}, result.Operations)
}
