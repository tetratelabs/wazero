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
			calls: []callCase{
				{params: []uint64{0}, expResults: []uint64{0}},
				{params: []uint64{2}, expResults: []uint64{2 * 2}},
				{params: []uint64{45}, expResults: []uint64{45 * 45}},
				{params: []uint64{90}, expResults: []uint64{90 * 90}},
				{params: []uint64{100}, expResults: []uint64{100 * 100}},
			},
		},
		{
			name: "memory_load_basic",
			m:    testcases.MemoryLoadBasic.Module,
			calls: []callCase{
				{params: []uint64{0}, expResults: []uint64{0x03_02_01_00}},
				{params: []uint64{256}, expResults: []uint64{0x03_02_01_00}},
				{params: []uint64{100}, expResults: []uint64{103<<24 | 102<<16 | 101<<8 | 100}},
				// Last 4 bytes.
				{params: []uint64{uint64(wasm.MemoryPageSize) - 4}, expResults: []uint64{0xfffefdfc}},
			},
		},
		{
			name: "memory out of bounds",
			m:    testcases.MemoryLoadBasic.Module,
			calls: []callCase{
				{params: []uint64{uint64(wasm.MemoryPageSize)}, expErr: "out of bounds memory access"},
				// We load I32, so we can't load from the last 3 bytes.
				{params: []uint64{uint64(wasm.MemoryPageSize) - 3}, expErr: "out of bounds memory access"},
			},
		},
		{
			name: "memory_loads",
			m:    testcases.MemoryLoads.Module,
			calls: []callCase{
				// These expected results are derived by commenting out `configureWazevo(config)` below to run the old compiler, assuming that it is correct.
				{params: []uint64{0}, expResults: []uint64{0x3020100, 0x706050403020100, 0x3020100, 0x706050403020100, 0x1211100f, 0x161514131211100f, 0x1211100f, 0x161514131211100f, 0x0, 0xf, 0x0, 0xf, 0x100, 0x100f, 0x100, 0x100f, 0x0, 0xf, 0x0, 0xf, 0x100, 0x100f, 0x100, 0x100f, 0x3020100, 0x1211100f, 0x3020100, 0x1211100f}},
				{params: []uint64{1}, expResults: []uint64{0x4030201, 0x807060504030201, 0x4030201, 0x807060504030201, 0x13121110, 0x1716151413121110, 0x13121110, 0x1716151413121110, 0x1, 0x10, 0x1, 0x10, 0x201, 0x1110, 0x201, 0x1110, 0x1, 0x10, 0x1, 0x10, 0x201, 0x1110, 0x201, 0x1110, 0x4030201, 0x13121110, 0x4030201, 0x13121110}},
				{params: []uint64{8}, expResults: []uint64{0xb0a0908, 0xf0e0d0c0b0a0908, 0xb0a0908, 0xf0e0d0c0b0a0908, 0x1a191817, 0x1e1d1c1b1a191817, 0x1a191817, 0x1e1d1c1b1a191817, 0x8, 0x17, 0x8, 0x17, 0x908, 0x1817, 0x908, 0x1817, 0x8, 0x17, 0x8, 0x17, 0x908, 0x1817, 0x908, 0x1817, 0xb0a0908, 0x1a191817, 0xb0a0908, 0x1a191817}},
				{params: []uint64{0xb}, expResults: []uint64{0xe0d0c0b, 0x1211100f0e0d0c0b, 0xe0d0c0b, 0x1211100f0e0d0c0b, 0x1d1c1b1a, 0x21201f1e1d1c1b1a, 0x1d1c1b1a, 0x21201f1e1d1c1b1a, 0xb, 0x1a, 0xb, 0x1a, 0xc0b, 0x1b1a, 0xc0b, 0x1b1a, 0xb, 0x1a, 0xb, 0x1a, 0xc0b, 0x1b1a, 0xc0b, 0x1b1a, 0xe0d0c0b, 0x1d1c1b1a, 0xe0d0c0b, 0x1d1c1b1a}},
				{params: []uint64{0xc}, expResults: []uint64{0xf0e0d0c, 0x131211100f0e0d0c, 0xf0e0d0c, 0x131211100f0e0d0c, 0x1e1d1c1b, 0x2221201f1e1d1c1b, 0x1e1d1c1b, 0x2221201f1e1d1c1b, 0xc, 0x1b, 0xc, 0x1b, 0xd0c, 0x1c1b, 0xd0c, 0x1c1b, 0xc, 0x1b, 0xc, 0x1b, 0xd0c, 0x1c1b, 0xd0c, 0x1c1b, 0xf0e0d0c, 0x1e1d1c1b, 0xf0e0d0c, 0x1e1d1c1b}},
				{params: []uint64{0xd}, expResults: []uint64{0x100f0e0d, 0x14131211100f0e0d, 0x100f0e0d, 0x14131211100f0e0d, 0x1f1e1d1c, 0x232221201f1e1d1c, 0x1f1e1d1c, 0x232221201f1e1d1c, 0xd, 0x1c, 0xd, 0x1c, 0xe0d, 0x1d1c, 0xe0d, 0x1d1c, 0xd, 0x1c, 0xd, 0x1c, 0xe0d, 0x1d1c, 0xe0d, 0x1d1c, 0x100f0e0d, 0x1f1e1d1c, 0x100f0e0d, 0x1f1e1d1c}},
				{params: []uint64{0xe}, expResults: []uint64{0x11100f0e, 0x1514131211100f0e, 0x11100f0e, 0x1514131211100f0e, 0x201f1e1d, 0x24232221201f1e1d, 0x201f1e1d, 0x24232221201f1e1d, 0xe, 0x1d, 0xe, 0x1d, 0xf0e, 0x1e1d, 0xf0e, 0x1e1d, 0xe, 0x1d, 0xe, 0x1d, 0xf0e, 0x1e1d, 0xf0e, 0x1e1d, 0x11100f0e, 0x201f1e1d, 0x11100f0e, 0x201f1e1d}},
				{params: []uint64{0xf}, expResults: []uint64{0x1211100f, 0x161514131211100f, 0x1211100f, 0x161514131211100f, 0x21201f1e, 0x2524232221201f1e, 0x21201f1e, 0x2524232221201f1e, 0xf, 0x1e, 0xf, 0x1e, 0x100f, 0x1f1e, 0x100f, 0x1f1e, 0xf, 0x1e, 0xf, 0x1e, 0x100f, 0x1f1e, 0x100f, 0x1f1e, 0x1211100f, 0x21201f1e, 0x1211100f, 0x21201f1e}},
			},
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
						require.Equal(t, len(cc.expResults), len(result))
						require.Equal(t, cc.expResults, result)
						for i := range cc.expResults {
							if cc.expResults[i] != result[i] {
								t.Errorf("result[%d]: exp %d, got %d", i, cc.expResults[i], result[i])
							}
						}
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
