package adhoc

import (
	"context"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

func typedFuncRefsConfigs() []struct {
	name   string
	config wazero.RuntimeConfig
} {
	configs := []struct {
		name   string
		config wazero.RuntimeConfig
	}{
		{"interpreter", wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(
			api.CoreFeaturesV2 | experimental.CoreFeaturesTypedFunctionReferences,
		)},
	}
	if platform.CompilerSupported() {
		configs = append(configs, struct {
			name   string
			config wazero.RuntimeConfig
		}{"compiler", wazero.NewRuntimeConfigCompiler().WithCoreFeatures(
			api.CoreFeaturesV2 | experimental.CoreFeaturesTypedFunctionReferences,
		)})
	}
	return configs
}

// TestCallRefWithConcreteRefLocals reproduces the call_ref.wast "run" function
// which uses (local (ref null $ii)) concrete ref locals, sets them with ref.func,
// and calls through them with call_ref.
func TestCallRefWithConcreteRefLocals(t *testing.T) {
	buf, err := os.ReadFile("../spectest/typed-function-references/testdata/call_ref.0.wasm")
	if err != nil {
		t.Skipf("could not read call_ref.0.wasm: %v", err)
	}

	configs := []struct {
		name   string
		config wazero.RuntimeConfig
	}{
		{"interpreter", wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(
			api.CoreFeaturesV2 | experimental.CoreFeaturesTypedFunctionReferences | experimental.CoreFeaturesTailCall,
		)},
	}
	if platform.CompilerSupported() {
		configs = append(configs, struct {
			name   string
			config wazero.RuntimeConfig
		}{"compiler", wazero.NewRuntimeConfigCompiler().WithCoreFeatures(
			api.CoreFeaturesV2 | experimental.CoreFeaturesTypedFunctionReferences | experimental.CoreFeaturesTailCall,
		)})
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, tc.config)
			defer r.Close(ctx)

			inst, err := r.InstantiateWithConfig(ctx, buf, wazero.NewModuleConfig())
			require.NoError(t, err)

			fn := inst.ExportedFunction("run")
			require.NotNil(t, fn)

			results, err := fn.Call(ctx, 0)
			require.NoError(t, err)
			require.Equal(t, uint64(0), results[0])

			results, err = fn.Call(ctx, 3)
			require.NoError(t, err)
			expected := uint64(0xfffffff7) // -9 as i32
			require.Equal(t, expected, results[0])
		})
	}
}

// TestCallRefTrapOnNull verifies that call_ref on a null reference traps
// with ErrRuntimeNullReference.
func TestCallRefTrapOnNull(t *testing.T) {
	// (module
	//   (type $t (func (result i32)))
	//   (func (export "test") (result i32)
	//     ref.null $t
	//     call_ref $t
	//   )
	// )
	mod := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Results: []wasm.ValueType{wasm.ValueTypeI32}, ResultNumInUint64: 1},
		},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{
				wasm.OpcodeRefNull, 0x00, // ref.null type_index=0
				wasm.OpcodeCallRef, 0x00, // call_ref type_index=0
				wasm.OpcodeEnd,
			}},
		},
		ExportSection: []wasm.Export{{Name: "test", Type: wasm.ExternTypeFunc, Index: 0}},
	}

	for _, tc := range typedFuncRefsConfigs() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, tc.config)
			defer r.Close(ctx)

			buf := binaryencoding.EncodeModule(mod)
			inst, err := r.InstantiateWithConfig(ctx, buf, wazero.NewModuleConfig())
			require.NoError(t, err)

			_, err = inst.ExportedFunction("test").Call(ctx)
			require.ErrorIs(t, err, wasmruntime.ErrRuntimeNullReference)
		})
	}
}

// TestRefAsNonNullTrapOnNull verifies that ref.as_non_null on a null funcref
// traps with ErrRuntimeNullReference.
func TestRefAsNonNullTrapOnNull(t *testing.T) {
	// (module
	//   (type $t (func))
	//   (func (export "test")
	//     ref.null func
	//     ref.as_non_null
	//     drop
	//   )
	// )
	mod := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{
				wasm.OpcodeRefNull, wasm.ValueTypeFuncref.Kind(), // ref.null func
				wasm.OpcodeRefAsNonNull, // trap if null
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			}},
		},
		ExportSection: []wasm.Export{{Name: "test", Type: wasm.ExternTypeFunc, Index: 0}},
	}

	for _, tc := range typedFuncRefsConfigs() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, tc.config)
			defer r.Close(ctx)

			buf := binaryencoding.EncodeModule(mod)
			inst, err := r.InstantiateWithConfig(ctx, buf, wazero.NewModuleConfig())
			require.NoError(t, err)

			_, err = inst.ExportedFunction("test").Call(ctx)
			require.ErrorIs(t, err, wasmruntime.ErrRuntimeNullReference)
		})
	}
}

// TestBrOnNull verifies that br_on_null branches when the ref is null
// and falls through (pushing the ref) when non-null.
func TestBrOnNull(t *testing.T) {
	buf, err := os.ReadFile("../spectest/typed-function-references/testdata/br_on_null.0.wasm")
	if err != nil {
		t.Skipf("could not read br_on_null.0.wasm: %v", err)
	}

	for _, tc := range typedFuncRefsConfigs() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, tc.config)
			defer r.Close(ctx)

			inst, err := r.InstantiateWithConfig(ctx, buf, wazero.NewModuleConfig())
			require.NoError(t, err)

			// null ref → branches, returns -1
			fn := inst.ExportedFunction("nullable-null")
			require.NotNil(t, fn)
			results, err := fn.Call(ctx)
			require.NoError(t, err)
			require.Equal(t, uint64(0xffffffff), results[0]) // -1 as i32

			// non-null ref → falls through, calls $f, returns 7
			fn = inst.ExportedFunction("nullable-f")
			require.NotNil(t, fn)
			results, err = fn.Call(ctx)
			require.NoError(t, err)
			require.Equal(t, uint64(7), results[0])
		})
	}
}

// TestBrOnNonNull verifies that br_on_non_null branches (carrying the ref)
// when the ref is non-null, and falls through when null.
func TestBrOnNonNull(t *testing.T) {
	// (module
	//   (type $t (func (result i32)))
	//   (func $f (type $t) (i32.const 42))
	//   (func (export "null") (result i32)
	//     (block $l (result i32)
	//       ref.null 0
	//       br_on_non_null $l   ;; null → fall through (no ref pushed)
	//       i32.const 1
	//     )
	//   )
	//   ... see below for "non_null"
	// )

	// For br_on_non_null targeting a block that expects (ref $t) on branch,
	// we need the block's type signature to accept the ref. Using a type index
	// for the block type is complex in binary encoding.
	// Instead, test via a pre-compiled spec wasm.
	buf, err := os.ReadFile("../spectest/typed-function-references/testdata/br_on_non_null.0.wasm")
	if err != nil {
		t.Skipf("could not read br_on_non_null.0.wasm: %v", err)
	}

	for _, tc := range typedFuncRefsConfigs() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, tc.config)
			defer r.Close(ctx)

			inst, err := r.InstantiateWithConfig(ctx, buf, wazero.NewModuleConfig())
			require.NoError(t, err)

			// null ref → falls through, returns -1
			fn := inst.ExportedFunction("nullable-null")
			require.NotNil(t, fn)
			results, err := fn.Call(ctx)
			require.NoError(t, err)
			require.Equal(t, uint64(0xffffffff), results[0]) // -1 as i32

			// non-null ref → branches carrying ref, calls $f, returns 7
			fn = inst.ExportedFunction("nullable-f")
			require.NotNil(t, fn)
			results, err = fn.Call(ctx)
			require.NoError(t, err)
			require.Equal(t, uint64(7), results[0])
		})
	}
}
