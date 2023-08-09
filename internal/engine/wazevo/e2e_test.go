package wazevo_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestE2E(t *testing.T) {
	type callCase struct {
		params, expResults []uint64
		expErr             string
	}
	// const i32, i64, f32, f64 = wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeF32, wasm.ValueTypeF64
	for _, tc := range []struct {
		name        string
		imported, m *wasm.Module
		calls       []callCase
	}{
		{
			name: "swap", m: testcases.SwapParamAndReturn.Module,
			calls: []callCase{
				{params: []uint64{math.MaxUint32, math.MaxInt32}, expResults: []uint64{math.MaxInt32, math.MaxUint32}},
			},
		},
		{
			name: "consts", m: testcases.Constants.Module,
			calls: []callCase{
				{expResults: []uint64{1, 2, uint64(math.Float32bits(32.0)), math.Float64bits(64.0)}},
			},
		},
		{
			name: "unreachable", m: testcases.Unreachable.Module,
			calls: []callCase{{expErr: "unreachable"}},
		},
		{
			name: "fibonacci_recursive", m: testcases.FibonacciRecursive.Module,
			calls: []callCase{
				{params: []uint64{0}, expResults: []uint64{0}},
				{params: []uint64{1}, expResults: []uint64{1}},
				{params: []uint64{2}, expResults: []uint64{1}},
				{params: []uint64{10}, expResults: []uint64{55}},
				{params: []uint64{20}, expResults: []uint64{6765}},
				{params: []uint64{30}, expResults: []uint64{0xcb228}},
			},
		},
		{name: "call", m: testcases.Call.Module, calls: []callCase{{expResults: []uint64{45, 45}}}},
		{
			name: "stack overflow",
			m: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}},
				ExportSection:   []wasm.Export{{Name: "f", Index: 0, Type: wasm.ExternTypeFunc}},
			},
			calls: []callCase{
				{expErr: "stack overflow"}, {expErr: "stack overflow"}, {expErr: "stack overflow"}, {expErr: "stack overflow"},
			},
		},
		{
			name:     "call",
			imported: testcases.ImportedFunctionCall.Imported,
			m:        testcases.ImportedFunctionCall.Module,
			calls:    []callCase{{params: []uint64{45}, expResults: []uint64{90}}},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			config := wazero.NewRuntimeConfigCompiler()

			// Configure the new optimizing backend!
			configureWazevo(config)

			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, config)
			defer func() {
				require.NoError(t, r.Close(ctx))
			}()

			if tc.imported != nil {
				imported, err := r.CompileModule(ctx, binaryencoding.EncodeModule(tc.imported))
				require.NoError(t, err)

				_, err = r.InstantiateModule(ctx, imported, wazero.NewModuleConfig())
				require.NoError(t, err)
			}

			compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(tc.m))
			require.NoError(t, err)

			inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
			require.NoError(t, err)

			for _, cc := range tc.calls {
				t.Run(fmt.Sprintf("call%v", cc.params), func(t *testing.T) {
					f := inst.ExportedFunction(testcases.ExportName)
					require.NotNil(t, f)
					result, err := f.Call(ctx, cc.params...)
					if cc.expErr != "" {
						require.EqualError(t, err, cc.expErr)
					} else {
						require.NoError(t, err)
						require.Equal(t, cc.expResults, result)
					}
				})
			}
		})
	}
}

// configureWazevo modifies wazero.RuntimeConfig and sets the wazevo implementation.
// This is a hack to avoid modifying outside the wazevo package while testing it end-to-end.
func configureWazevo(config wazero.RuntimeConfig) {
	// This is the internal representation of interface in Go.
	// https://research.swtch.com/interfaces
	type iface struct {
		_    *byte
		data unsafe.Pointer
	}

	configInterface := (*iface)(unsafe.Pointer(&config))

	// This corresponds to the unexported wazero.runtimeConfig, and the target field newEngine exists
	// in the middle of the implementation.
	type newEngine func(context.Context, api.CoreFeatures, filecache.Cache) wasm.Engine
	type runtimeConfig struct {
		enabledFeatures       api.CoreFeatures
		memoryLimitPages      uint32
		memoryCapacityFromMax bool
		engineKind            int
		dwarfDisabled         bool
		newEngine
		// Other fields follow, but we don't care.
	}
	cm := (*runtimeConfig)(configInterface.data)
	// Insert the wazevo implementation.
	cm.newEngine = wazevo.NewEngine
}
