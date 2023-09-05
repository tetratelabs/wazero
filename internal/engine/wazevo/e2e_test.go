package wazevo_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	v1 "github.com/tetratelabs/wazero/internal/integration_test/spectest/v1"
	v2 "github.com/tetratelabs/wazero/internal/integration_test/spectest/v2"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	i32 = wasm.ValueTypeI32
	i64 = wasm.ValueTypeI64
	f32 = wasm.ValueTypeF32
	f64 = wasm.ValueTypeF64
)

func TestSpectestV1(t *testing.T) {
	config := wazero.NewRuntimeConfigCompiler().WithCoreFeatures(api.CoreFeaturesV1)
	// Configure the new optimizing backend!
	configureWazevo(config)

	// TODO: migrate to integration_test/spectest/v1/spec_test.go by the time when closing https://github.com/tetratelabs/wazero/issues/1496
	for _, tc := range []struct {
		name string
	}{
		{name: "address"},
		{name: "align"},
		{name: "br"},
		{name: "br_if"},
		{name: "br_table"},
		{name: "break-drop"},
		{name: "block"},
		{name: "binary"},
		{name: "binary-leb128"},
		{name: "call"},
		{name: "call_indirect"},
		{name: "comments"},
		{name: "custom"},
		{name: "conversions"},
		{name: "const"},
		{name: "data"},
		{name: "elem"},
		{name: "endianness"},
		{name: "exports"},
		{name: "f32"},
		{name: "f32_bitwise"},
		{name: "f32_cmp"},
		{name: "f64"},
		{name: "f64_bitwise"},
		{name: "f64_cmp"},
		{name: "fac"},
		{name: "float_exprs"},
		{name: "float_literals"},
		{name: "float_memory"},
		{name: "float_misc"},
		{name: "func"},
		{name: "func_ptrs"},
		{name: "forward"},
		{name: "globals"},
		{name: "if"},
		{name: "imports"},
		{name: "inline-module"},
		{name: "i32"},
		{name: "i64"},
		{name: "int_exprs"},
		{name: "int_literals"},
		{name: "labels"},
		{name: "left-to-right"},
		{name: "linking"},
		{name: "load"},
		{name: "loop"},
		{name: "local_get"},
		{name: "local_set"},
		{name: "local_tee"},
		{name: "memory"},
		{name: "memory_size"},
		{name: "memory_grow"},
		{name: "memory_redundancy"},
		{name: "memory_trap"},
		{name: "names"},
		{name: "nop"},
		{name: "return"},
		{name: "select"},
		{name: "start"},
		{name: "stack"},
		{name: "store"},
		{name: "switch"},
		{name: "skip-stack-guard-page"},
		{name: "token"},
		{name: "type"},
		{name: "traps"},
		{name: "unreachable"},
		{name: "unreached-invalid"},
		{name: "unwind"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("normal", func(t *testing.T) {
				spectest.RunCase(t, v1.Testcases, tc.name, context.Background(), config,
					-1, 0, math.MaxInt)
			})
			t.Run("reg high pressure", func(t *testing.T) {
				ctx := wazevoapi.EnableHighRegisterPressure(context.Background())
				spectest.RunCase(t, v1.Testcases, tc.name, ctx, config,
					-1, 0, math.MaxInt)
			})
		})
	}
}

func TestSpectestV2(t *testing.T) {
	config := wazero.NewRuntimeConfigCompiler().WithCoreFeatures(api.CoreFeaturesV2)
	// Configure the new optimizing backend!
	configureWazevo(config)

	for _, tc := range []struct {
		name string
	}{
		// {"conversions"}, includes non-trapping conversions.
	} {
		t.Run(tc.name, func(t *testing.T) {
			spectest.RunCase(t, v2.Testcases, tc.name, context.Background(), config,
				-1, 0, math.MaxInt)
		})
	}
}

func TestE2E(t *testing.T) {
	type callCase struct {
		funcName           string // defaults to testcases.ExportedFunctionName
		params, expResults []uint64
		expErr             string
	}
	for _, tc := range []struct {
		name        string
		imported, m *wasm.Module
		calls       []callCase
	}{
		{
			name: "selects", m: testcases.Selects.Module,
			calls: []callCase{
				{
					params: []uint64{
						0, 1, // i32,
						200, 100, // i64,
						uint64(math.Float32bits(3.0)), uint64(math.Float32bits(10.0)),
						math.Float64bits(-123.4), math.Float64bits(-10000000000.0),
					},
					expResults: []uint64{
						1,
						200,
						uint64(math.Float32bits(3.0)),
						math.Float64bits(-123.4),
					},
				},
			},
		},
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
				ExportSection:   []wasm.Export{{Name: testcases.ExportedFunctionName, Index: 0, Type: wasm.ExternTypeFunc}},
			},
			calls: []callCase{
				{expErr: "stack overflow"}, {expErr: "stack overflow"}, {expErr: "stack overflow"}, {expErr: "stack overflow"},
			},
		},
		{
			name:     "imported_function_call",
			imported: testcases.ImportedFunctionCall.Imported,
			m:        testcases.ImportedFunctionCall.Module,
			calls: []callCase{
				{params: []uint64{0}, expResults: []uint64{0}},
				{params: []uint64{2}, expResults: []uint64{2 * 2}},
				{params: []uint64{45}, expResults: []uint64{45 * 45}},
				{params: []uint64{90}, expResults: []uint64{90 * 90}},
				{params: []uint64{100}, expResults: []uint64{100 * 100}},
				{params: []uint64{100, 200}, expResults: []uint64{100 * 200}, funcName: "imported_exported"},
			},
		},
		{
			name: "memory_store_basic",
			m:    testcases.MemoryStoreBasic.Module,
			calls: []callCase{
				{params: []uint64{0, 0xf}, expResults: []uint64{0xf}},
				{params: []uint64{256, 0xff}, expResults: []uint64{0xff}},
				{params: []uint64{100, 0xffffffff}, expResults: []uint64{0xffffffff}},
				// We load I32, so we can't load from the last 3 bytes.
				{params: []uint64{uint64(wasm.MemoryPageSize) - 3}, expErr: "out of bounds memory access"},
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
		{
			name: "globals_get",
			m:    testcases.GlobalsGet.Module,
			calls: []callCase{
				{expResults: []uint64{0x80000000, 0x8000000000000000, 0x7f7fffff, 0x7fefffffffffffff}},
			},
		},
		{
			name:  "globals_set",
			m:     testcases.GlobalsSet.Module,
			calls: []callCase{{expResults: []uint64{1, 2, uint64(math.Float32bits(3.0)), math.Float64bits(4.0)}}},
		},
		{
			name: "globals_mutable",
			m:    testcases.GlobalsMutable.Module,
			calls: []callCase{{expResults: []uint64{
				100, 200, uint64(math.Float32bits(300.0)), math.Float64bits(400.0),
				1, 2, uint64(math.Float32bits(3.0)), math.Float64bits(4.0),
			}}},
		},
		{
			name:  "memory_size_grow",
			m:     testcases.MemorySizeGrow.Module,
			calls: []callCase{{expResults: []uint64{1, 2, 0xffffffff}}},
		},
		{
			name:     "imported_memory_grow",
			imported: testcases.ImportedMemoryGrow.Imported,
			m:        testcases.ImportedMemoryGrow.Module,
			calls:    []callCase{{expResults: []uint64{1, 1, 11, 11}}},
		},
		{
			name: "call_indirect",
			m:    testcases.CallIndirect.Module,
			// parameter == table offset.
			calls: []callCase{
				{params: []uint64{0}, expErr: "indirect call type mismatch"},
				{params: []uint64{1}, expResults: []uint64{10}},
				{params: []uint64{2}, expErr: "indirect call type mismatch"},
				{params: []uint64{10}, expErr: "invalid table access"},             // Null pointer.
				{params: []uint64{math.MaxUint32}, expErr: "invalid table access"}, // Out of bounds.
			},
		},
		{
			name: "br_table",
			m:    testcases.BrTable.Module,
			calls: []callCase{
				{params: []uint64{0}, expResults: []uint64{11}},
				{params: []uint64{1}, expResults: []uint64{12}},
				{params: []uint64{2}, expResults: []uint64{13}},
				{params: []uint64{3}, expResults: []uint64{14}},
				{params: []uint64{4}, expResults: []uint64{15}},
				{params: []uint64{5}, expResults: []uint64{16}},
				// Out of range --> default.
				{params: []uint64{6}, expResults: []uint64{11}},
				{params: []uint64{1000}, expResults: []uint64{11}},
			},
		},
		{
			name: "br_table_with_args",
			m:    testcases.BrTableWithArg.Module,
			calls: []callCase{
				{params: []uint64{0, 100}, expResults: []uint64{11 + 100}},
				{params: []uint64{1, 100}, expResults: []uint64{12 + 100}},
				{params: []uint64{2, 100}, expResults: []uint64{13 + 100}},
				{params: []uint64{3, 100}, expResults: []uint64{14 + 100}},
				{params: []uint64{4, 100}, expResults: []uint64{15 + 100}},
				{params: []uint64{5, 100}, expResults: []uint64{16 + 100}},
				// Out of range --> default.
				{params: []uint64{6, 200}, expResults: []uint64{11 + 200}},
				{params: []uint64{1000, 300}, expResults: []uint64{11 + 300}},
			},
		},
		{
			name: "multi_predecessor_local_ref",
			m:    testcases.MultiPredecessorLocalRef.Module,
			calls: []callCase{
				{params: []uint64{0, 100}, expResults: []uint64{100}},
				{params: []uint64{1, 100}, expResults: []uint64{1}},
				{params: []uint64{1, 200}, expResults: []uint64{1}},
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
				name := cc.funcName
				if name == "" {
					name = testcases.ExportedFunctionName
				}
				t.Run(fmt.Sprintf("call_%s%v", name, cc.params), func(t *testing.T) {
					f := inst.ExportedFunction(name)
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

func TestE2E_host_functions(t *testing.T) {
	config := wazero.NewRuntimeConfigCompiler()

	// Configure the new optimizing backend!
	configureWazevo(config)

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	var expectedMod api.Module

	b := r.NewHostModuleBuilder("env")
	b.NewFunctionBuilder().WithFunc(func(ctx2 context.Context, d float64) float64 {
		require.Equal(t, ctx, ctx2)
		require.Equal(t, 35.0, d)
		return math.Sqrt(d)
	}).Export("root")
	b.NewFunctionBuilder().WithFunc(func(ctx2 context.Context, mod api.Module, a uint32, b uint64, c float32, d float64) (uint32, uint64, float32, float64) {
		require.Equal(t, expectedMod, mod)
		require.Equal(t, ctx, ctx2)
		require.Equal(t, uint32(2), a)
		require.Equal(t, uint64(100), b)
		require.Equal(t, float32(15.0), c)
		require.Equal(t, 35.0, d)
		return a * a, b * b, c * c, d * d
	}).Export("square")

	_, err := b.Instantiate(ctx)
	require.NoError(t, err)

	m := &wasm.Module{
		ImportFunctionCount: 2,
		ImportSection: []wasm.Import{
			{Module: "env", Name: "root", Type: wasm.ExternTypeFunc, DescFunc: 0},
			{Module: "env", Name: "square", Type: wasm.ExternTypeFunc, DescFunc: 1},
		},
		TypeSection: []wasm.FunctionType{
			{Results: []wasm.ValueType{f64}, Params: []wasm.ValueType{f64}},
			{Results: []wasm.ValueType{i32, i64, f32, f64}, Params: []wasm.ValueType{i32, i64, f32, f64}},
			{Results: []wasm.ValueType{i32, i64, f32, f64, f64}, Params: []wasm.ValueType{i32, i64, f32, f64}},
		},
		FunctionSection: []wasm.Index{2},
		CodeSection: []wasm.Code{{
			Body: []byte{
				wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
				wasm.OpcodeCall, 1,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeCall, 0,
				wasm.OpcodeEnd,
			},
		}},
		ExportSection: []wasm.Export{{Name: testcases.ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 2}},
	}

	compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(m))
	require.NoError(t, err)

	inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	expectedMod = inst

	f := inst.ExportedFunction(testcases.ExportedFunctionName)

	res, err := f.Call(ctx, []uint64{2, 100, uint64(math.Float32bits(15.0)), math.Float64bits(35.0)}...)
	require.NoError(t, err)
	require.Equal(t, []uint64{
		2 * 2, 100 * 100, uint64(math.Float32bits(15.0 * 15.0)), math.Float64bits(35.0 * 35.0),
		math.Float64bits(math.Sqrt(35.0)),
	}, res)
}

func TestE2E_stores(t *testing.T) {
	config := wazero.NewRuntimeConfigCompiler()

	// Configure the new optimizing backend!
	configureWazevo(config)

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(testcases.MemoryStores.Module))
	require.NoError(t, err)

	inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	f := inst.ExportedFunction(testcases.ExportedFunctionName)

	mem, ok := inst.Memory().Read(0, wasm.MemoryPageSize)
	require.True(t, ok)
	for _, tc := range []struct {
		i32 uint32
		i64 uint64
		f32 float32
		f64 float64
	}{
		{0, 0, 0, 0},
		{1, 2, 3.0, 4.0},
		{math.MaxUint32, math.MaxUint64, float32(math.NaN()), math.NaN()},
		{1 << 31, 1 << 63, 3.0, 4.0},
	} {
		t.Run(fmt.Sprintf("i32=%#x,i64=%#x,f32=%#x,f64=%#x", tc.i32, tc.i64, tc.f32, tc.f64), func(t *testing.T) {
			_, err = f.Call(ctx, []uint64{uint64(tc.i32), tc.i64, uint64(math.Float32bits(tc.f32)), math.Float64bits(tc.f64)}...)
			require.NoError(t, err)

			offset := 0
			require.Equal(t, binary.LittleEndian.Uint32(mem[offset:]), tc.i32)
			offset += 8
			require.Equal(t, binary.LittleEndian.Uint64(mem[offset:]), tc.i64)
			offset += 8
			require.Equal(t, math.Float32bits(tc.f32), binary.LittleEndian.Uint32(mem[offset:]))
			offset += 8
			require.Equal(t, math.Float64bits(tc.f64), binary.LittleEndian.Uint64(mem[offset:]))
			offset += 8

			// i32.store_8
			view := binary.LittleEndian.Uint64(mem[offset:])
			require.Equal(t, uint64(tc.i32)&0xff, view)
			offset += 8
			// i32.store_16
			view = binary.LittleEndian.Uint64(mem[offset:])
			require.Equal(t, uint64(tc.i32)&0xffff, view)
			offset += 8
			// i64.store_8
			view = binary.LittleEndian.Uint64(mem[offset:])
			require.Equal(t, tc.i64&0xff, view)
			offset += 8
			// i64.store_16
			view = binary.LittleEndian.Uint64(mem[offset:])
			require.Equal(t, tc.i64&0xffff, view)
			offset += 8
			// i64.store_32
			view = binary.LittleEndian.Uint64(mem[offset:])
			require.Equal(t, tc.i64&0xffffffff, view)
		})
	}
}

func TestE2E_reexported_memory(t *testing.T) {
	m1 := &wasm.Module{
		ExportSection: []wasm.Export{{Name: "mem", Type: wasm.ExternTypeMemory, Index: 0}},
		MemorySection: &wasm.Memory{Min: 1},
		NameSection:   &wasm.NameSection{ModuleName: "m1"},
	}
	m2 := &wasm.Module{
		ImportMemoryCount: 1,
		ExportSection:     []wasm.Export{{Name: "mem2", Type: wasm.ExternTypeMemory, Index: 0}},
		ImportSection:     []wasm.Import{{Module: "m1", Name: "mem", Type: wasm.ExternTypeMemory, DescMem: &wasm.Memory{Min: 1}}},
		NameSection:       &wasm.NameSection{ModuleName: "m2"},
	}
	m3 := &wasm.Module{
		ImportMemoryCount: 1,
		ImportSection:     []wasm.Import{{Module: "m2", Name: "mem2", Type: wasm.ExternTypeMemory, DescMem: &wasm.Memory{Min: 1}}},
		TypeSection:       []wasm.FunctionType{{Results: []wasm.ValueType{i32}}},
		ExportSection:     []wasm.Export{{Name: testcases.ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
		FunctionSection:   []wasm.Index{0},
		CodeSection:       []wasm.Code{{Body: []byte{wasm.OpcodeI32Const, 10, wasm.OpcodeMemoryGrow, 0, wasm.OpcodeEnd}}},
	}

	config := wazero.NewRuntimeConfigCompiler()

	// Configure the new optimizing backend!
	configureWazevo(config)

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	m1Inst, err := r.Instantiate(ctx, binaryencoding.EncodeModule(m1))
	require.NoError(t, err)

	m2Inst, err := r.Instantiate(ctx, binaryencoding.EncodeModule(m2))
	require.NoError(t, err)

	m3Inst, err := r.Instantiate(ctx, binaryencoding.EncodeModule(m3))
	require.NoError(t, err)

	f := m3Inst.ExportedFunction(testcases.ExportedFunctionName)
	result, err := f.Call(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), result[0])
	mem := m1Inst.Memory()
	require.Equal(t, mem, m3Inst.Memory())
	require.Equal(t, mem, m2Inst.Memory())
	require.Equal(t, uint32(11), mem.Size()/65536)
}
