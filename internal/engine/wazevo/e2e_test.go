package wazevo_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	i32  = wasm.ValueTypeI32
	i64  = wasm.ValueTypeI64
	f32  = wasm.ValueTypeF32
	f64  = wasm.ValueTypeF64
	v128 = wasm.ValueTypeV128
)

func TestE2E(t *testing.T) {
	tmp := t.TempDir()
	type callCase struct {
		funcName           string // defaults to testcases.ExportedFunctionName
		params, expResults []uint64
		expErr             string
	}
	for _, tc := range []struct {
		name        string
		imported, m *wasm.Module
		calls       []callCase
		features    api.CoreFeatures
		setupMemory func(mem api.Memory)
	}{
		{
			name: "empty", m: testcases.Empty.Module,
			calls: []callCase{{expResults: []uint64{}}},
		},
		{
			name: "only_return", m: testcases.OnlyReturn.Module,
			calls: []callCase{{expResults: []uint64{}}},
		},
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
			name: "add_sub_return", m: testcases.AddSubReturn.Module,
			calls: []callCase{
				{
					params:     []uint64{},
					expResults: []uint64{3, 3, 3, 3},
				},
			},
		},
		{
			name: "arithm_return", m: testcases.ArithmReturn.Module,
			calls: []callCase{
				{
					params: []uint64{
						21, 10, 0xf0,
						21, 10, 0xf0,
					},
					expResults: []uint64{
						21 * 10, 21 & 10, 21 | 10, 21 ^ 10,
						21 << 10, 21 >> 10, 0xf0 >> 10,
						88080384, 20971520,

						21 * 10, 21 & 10, 21 | 10, 21 ^ 10,
						21 << 10, 21 >> 10, 0xf0 >> 10,
						378302368699121664, 20971520,
					},
				},
			},
		},
		{
			name: "divrem_unsigned_return", m: testcases.DivUReturn32.Module,
			calls: []callCase{
				{
					params:     []uint64{21, 10, 21, 10},
					expResults: []uint64{21 / 10, 21 % 10},
				},
				{
					params: []uint64{21, 0, 1, 1},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{1, 1, 21, 0},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{
						0x80000000, 0xffffffff, 1, 1,
					},
					expResults: []uint64{0, 0},
				},
				{
					params: []uint64{
						1, 1, 0x80000000, 0xffffffff,
					},
					expResults: []uint64{1, 0x80000000},
				},
			},
		},
		{
			name: "divrem_unsigned_return", m: testcases.DivUReturn64.Module,
			calls: []callCase{
				{
					params:     []uint64{21, 10, 21, 10},
					expResults: []uint64{21 / 10, 21 % 10},
				},
				{
					params: []uint64{21, 0, 1, 1},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{1, 1, 21, 0},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params:     []uint64{0x80000000, 0xffffffff, 1, 1},
					expResults: []uint64{0, 0},
				},
				{
					params:     []uint64{1, 1, 0x80000000, 0xffffffff},
					expResults: []uint64{1, 0x80000000},
				},
			},
		},
		{
			name: "divrem_signed_return32", m: testcases.DivSReturn32.Module,
			calls: []callCase{
				{
					params:     []uint64{21, 10, 21, 10},
					expResults: []uint64{21 / 10, 21 % 10},
				},
				{
					params: []uint64{21, 0, 1, 1},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{1, 1, 21, 0},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{0x80000000, 0xffffffff, 1, 1},
					expErr: "wasm error: integer overflow",
				},
				{
					params:     []uint64{1, 1, 0x80000000, 0xffffffff},
					expResults: []uint64{1, 0},
				},
			},
		},

		{
			name: "divrem_signed_return32 inverted rem div order", m: testcases.DivSReturn32_weird.Module,
			calls: []callCase{
				{
					params: []uint64{21, 0, 1, 1},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{1, 1, 21, 0},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{0x80000000, 0xffffffff, 1, 1},
					expErr: "wasm error: integer overflow",
				},
				{
					params:     []uint64{1, 1, 0x80000000, 0xffffffff},
					expResults: []uint64{0, 1},
				},
			},
		},
		{
			name: "divrem_signed_return64", m: testcases.DivSReturn64.Module,
			calls: []callCase{
				{
					params:     []uint64{21, 10, 21, 10},
					expResults: []uint64{21 / 10, 21 % 10},
				},
				{
					params: []uint64{21, 0, 1, 1},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{1, 1, 21, 0},
					expErr: "wasm error: integer divide by zero",
				},
				{
					params: []uint64{0x8000000000000000, 0xffffffffffffffff, 1, 1},
					expErr: "wasm error: integer overflow",
				},
			},
		},
		{
			name: "integer bit counts", m: testcases.IntegerBitCounts.Module,
			calls: []callCase{{
				params: []uint64{10, 100},
				expResults: []uint64{
					28, 1, 2, 57, 2, 3,
				},
			}},
		},
		{
			name: "many_params_many_results",
			m:    testcases.ManyParamsManyResults.Module,
			calls: []callCase{
				{
					params: []uint64{
						1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
						1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
						1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
						1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
					},
					expResults: []uint64{
						10, 9, 8, 7, 6, 5, 4, 3, 2, 1,
						10, 9, 8, 7, 6, 5, 4, 3, 2, 1,
						10, 9, 8, 7, 6, 5, 4, 3, 2, 1,
						10, 9, 8, 7, 6, 5, 4, 3, 2, 1,
					},
				},
			},
		},
		{
			name: "float_arithm", m: testcases.FloatArithm.Module,
			calls: []callCase{
				{
					params: []uint64{
						math.Float64bits(25), math.Float64bits(5), math.Float64bits(1.4),
						uint64(math.Float32bits(25)), uint64(math.Float32bits(5)), uint64(math.Float32bits(1.4)),
					},
					expResults: []uint64{
						math.Float64bits(-25),
						math.Float64bits(25),

						math.Float64bits(5),
						math.Float64bits(30),
						math.Float64bits(20),
						math.Float64bits(125),
						math.Float64bits(5),

						math.Float64bits(1),
						math.Float64bits(1),
						math.Float64bits(2),
						math.Float64bits(1),

						math.Float64bits(-25),

						uint64(math.Float32bits(-25)),
						uint64(math.Float32bits(25)),

						uint64(math.Float32bits(5)),
						uint64(math.Float32bits(30)),
						uint64(math.Float32bits(20)),
						uint64(math.Float32bits(125)),
						uint64(math.Float32bits(5)),

						uint64(math.Float32bits(1)),
						uint64(math.Float32bits(1)),
						uint64(math.Float32bits(2)),
						uint64(math.Float32bits(1)),

						uint64(math.Float32bits(-25)),
					},
				},
			},
		},
		{
			name: "min_max_float", m: testcases.MinMaxFloat.Module,
			calls: []callCase{
				{
					params: []uint64{
						math.Float64bits(25), math.Float64bits(5),
						uint64(math.Float32bits(25)), uint64(math.Float32bits(5)),
					},
					expResults: []uint64{
						math.Float64bits(5),
						math.Float64bits(25),
						uint64(math.Float32bits(5)),
						uint64(math.Float32bits(25)),
					},
				},
				{
					// Left-hand side is NaN.
					params: []uint64{
						0x7ff8000000000001, math.Float64bits(5),
						uint64(0x7ff80001), uint64(math.Float32bits(5)),
					},
					expResults: []uint64{
						0x7ff8000000000001,
						0x7ff8000000000001,

						uint64(0x7ff80001),
						uint64(0x7ff80001),
					},
				},
				{
					// Both NaN.
					params: []uint64{
						0x7ff8000000000001, 0x7ff8000000000001,
						uint64(0x7ff80001), uint64(0x7ff80001),
					},
					expResults: []uint64{
						0x7ff8000000000001,
						0x7ff8000000000001,

						uint64(0x7ff80001),
						uint64(0x7ff80001),
					},
				},
				{
					// Negative zero and zero.
					params: []uint64{
						0x8000000000000000, 0,
						uint64(0), uint64(0x80000000),
					},
					expResults: []uint64{
						0x8000000000000000,
						0,

						uint64(0x80000000),
						uint64(0),
					},
				},
			},
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
		{name: "call_simple", m: testcases.CallSimple.Module, calls: []callCase{{expResults: []uint64{40}}}},
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
				{params: []uint64{uint64(wasm.MemoryPageSize) - 3, 0}, expErr: "out of bounds memory access"},
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
				{expResults: []uint64{0x80000000, 0x8000000000000000, 0x7f7fffff, 0x7fefffffffffffff, 1234, 5678}},
			},
		},
		{
			name: "globals_set",
			m:    testcases.GlobalsSet.Module,
			calls: []callCase{{expResults: []uint64{
				1, 2, uint64(math.Float32bits(3.0)), math.Float64bits(4.0), 10, 20,
			}}},
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
		{
			name: "vector_bit_select",
			m:    testcases.VecBitSelect.Module,
			calls: []callCase{
				{params: []uint64{1, 2, 3, 4, 5, 6}, expResults: []uint64{0x3, 0x2, 0x5, 0x6}},
			},
		},
		{
			name: "vector_shuffle",
			m:    testcases.VecShuffle.Module,
			calls: []callCase{
				{params: []uint64{0x01010101, 0x02020202, 0x03030303, 0x04040404}, expResults: []uint64{0x01010101, 0x04040404}},
				{params: []uint64{0x03030303, 0x04040404, 0x01010101, 0x02020202}, expResults: []uint64{0x03030303, 0x02020202}},
				{params: []uint64{0x00000000, 0xffffffff, 0xffffffff, 0x00000000}, expResults: []uint64{0x00000000, 0x00000000}},
				{params: []uint64{0xffffffff, 0x00000000, 0x00000000, 0xffffffff}, expResults: []uint64{0xffffffff, 0xffffffff}},
				{params: []uint64{0x00000000, 0x11111111, 0x11111111, 0xffffffff}, expResults: []uint64{0x00000000, 0xffffffff}},
			},
		},
		{
			name: "vector_shuffle (1st only)",
			m:    testcases.VecShuffleWithLane(1, 1, 1, 1, 0, 0, 0, 0, 10, 10, 10, 10, 0, 0, 0, 0),
			calls: []callCase{
				{params: []uint64{0x0000000000000b0a, 0x0c0000, 0xffffffffffffffff, 0xffffffffffffffff}, expResults: []uint64{0x0a0a0a0a0b0b0b0b, 0x0a0a0a0a0c0c0c0c}},
				{params: []uint64{0x01010101, 0x02020202, 0x03030303, 0x04040404}, expResults: []uint64{0x0101010101010101, 0x101010102020202}},
				{params: []uint64{0x03030303, 0x04040404, 0x01010101, 0x02020202}, expResults: []uint64{0x0303030303030303, 0x303030304040404}},
				{params: []uint64{0x00000000, 0xffffffff, 0xffffffff, 0x00000000}, expResults: []uint64{0x0000000000000000, 0x0000000ffffffff}},
				{params: []uint64{0xffffffff, 0x00000000, 0x00000000, 0xffffffff}, expResults: []uint64{0xffffffffffffffff, 0xffffffff00000000}},
				{params: []uint64{0x00000000, 0x11111111, 0x11111111, 0xffffffff}, expResults: []uint64{0x0000000000000000, 0x0000000011111111}},
			},
		},
		{
			name: "vector_shuffle (2nd only)",
			m:    testcases.VecShuffleWithLane(17, 17, 17, 17, 16, 16, 16, 16, 26, 26, 26, 26, 16, 16, 16, 16),
			calls: []callCase{
				{params: []uint64{0xffffffffffffffff, 0xffffffffffffffff, 0x0000000000000b0a, 0x0c0000}, expResults: []uint64{0x0a0a0a0a0b0b0b0b, 0x0a0a0a0a0c0c0c0c}},
				{params: []uint64{0x01010101, 0x02020202, 0x03030303, 0x04040404}, expResults: []uint64{0x303030303030303, 0x303030304040404}},
				{params: []uint64{0x03030303, 0x04040404, 0x01010101, 0x02020202}, expResults: []uint64{0x101010101010101, 0x101010102020202}},
				{params: []uint64{0x00000000, 0xffffffff, 0xffffffff, 0x00000000}, expResults: []uint64{0xffffffffffffffff, 0xffffffff00000000}},
				{params: []uint64{0xffffffff, 0x00000000, 0x00000000, 0xffffffff}, expResults: []uint64{0x0000000000000000, 0xffffffff}},
				{params: []uint64{0x00000000, 0x11111111, 0x11111111, 0xffffffff}, expResults: []uint64{0x1111111111111111, 0x11111111ffffffff}},
			},
		},
		{
			name: "vector_shuffle (mixed)",
			m:    testcases.VecShuffleWithLane(0, 17, 2, 19, 4, 21, 6, 23, 8, 25, 10, 27, 12, 29, 14, 31),
			calls: []callCase{
				{params: []uint64{0xff08ff07ff06ff05, 0xff04ff03ff02ff01, 0x18ff17ff16ff15ff, 0x14ff13ff12ff11ff}, expResults: []uint64{0x1808170716061505, 0x1404130312021101}},
				{params: []uint64{0x01010101, 0x02020202, 0x03030303, 0x04040404}, expResults: []uint64{0x3010301, 0x4020402}},
				{params: []uint64{0x03030303, 0x04040404, 0x01010101, 0x02020202}, expResults: []uint64{0x1030103, 0x2040204}},
				{params: []uint64{0x00000000, 0xffffffff, 0xffffffff, 0x00000000}, expResults: []uint64{0xff00ff00, 0xff00ff}},
				{params: []uint64{0xffffffff, 0x00000000, 0x00000000, 0xffffffff}, expResults: []uint64{0xff00ff, 0xff00ff00}},
				{params: []uint64{0x00000000, 0x11111111, 0x11111111, 0xffffffff}, expResults: []uint64{0x11001100, 0xff11ff11}},
			},
		},
		{
			name:     "memory_wait32",
			m:        testcases.MemoryWait32.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{0x0, 0xbeef, 0xffffffff}, expResults: []uint64{1}}, // exp not equal, returns 1
				{params: []uint64{0x1, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x2, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x3, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x4, 0xbeef, 0xffffffff}, expResults: []uint64{1}}, // exp not equal, returns 1

				{params: []uint64{0xffffffff, 0xbeef, 0xffffffff}, expErr: "out of bounds memory access"},
			},
		},
		{
			name:     "memory_wait64",
			m:        testcases.MemoryWait64.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{0x0, 0xbeef, 0xffffffff}, expResults: []uint64{1}}, // exp not equal, returns 1
				{params: []uint64{0x1, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x2, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x3, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x4, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x5, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x6, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x7, 0xbeef, 0xffffffff}, expErr: "unaligned atomic"},
				{params: []uint64{0x8, 0xbeef, 0xffffffff}, expResults: []uint64{1}}, // exp not equal, returns 1

				{params: []uint64{0xffffffff, 0xbeef, 0xffffffff}, expErr: "out of bounds memory access"},
			},
		},
		{
			name:     "memory_notify",
			m:        testcases.MemoryNotify.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{0x0, 0x1}, expResults: []uint64{0}}, // no waiters, returns 0
				{params: []uint64{0x1, 0x1}, expErr: "unaligned atomic"},
				{params: []uint64{0x2, 0x1}, expErr: "unaligned atomic"},
				{params: []uint64{0x3, 0x1}, expErr: "unaligned atomic"},
				{params: []uint64{0x4, 0x1}, expResults: []uint64{0}}, // no waiters, returns 0
			},
		},
		{
			name:     "atomic_rmw_add",
			m:        testcases.AtomicRmwAdd.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{1, 2, 3, 4, 5, 6, 7}, expResults: []uint64{0, 0, 0, 0, 0, 0, 0}},
				{params: []uint64{1, 2, 3, 4, 5, 6, 7}, expResults: []uint64{1, 2, 3, 4, 5, 6, 7}},
				{params: []uint64{1, 2, 3, 4, 5, 6, 7}, expResults: []uint64{2, 4, 6, 8, 10, 12, 14}},
			},
		},
		{
			name:     "atomic_rmw_sub",
			m:        testcases.AtomicRmwSub.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{1, 2, 3, 4, 5, 6, 7}, expResults: []uint64{0, 0, 0, 0, 0, 0, 0}},
				{
					params: []uint64{1, 2, 3, 4, 5, 6, 7},
					expResults: []uint64{
						api.EncodeI32(-1) & 0xff,
						api.EncodeI32(-2) & 0xffff,
						api.EncodeI32(-3),
						api.EncodeI64(-4) & 0xff,
						api.EncodeI64(-5) & 0xffff,
						api.EncodeI64(-6) & 0xffffffff,
						api.EncodeI64(-7),
					},
				},
				{
					params: []uint64{1, 2, 3, 4, 5, 6, 7},
					expResults: []uint64{
						api.EncodeI32(-2) & 0xff,
						api.EncodeI32(-4) & 0xffff,
						api.EncodeI32(-6),
						api.EncodeI64(-8) & 0xff,
						api.EncodeI64(-10) & 0xffff,
						api.EncodeI64(-12) & 0xffffffff,
						api.EncodeI64(-14),
					},
				},
			},
		},
		{
			name:     "atomic_rmw_and",
			m:        testcases.AtomicRmwAnd.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			setupMemory: func(mem api.Memory) {
				mem.WriteUint32Le(0, 0xffffffff)
				mem.WriteUint32Le(8, 0xffffffff)
				mem.WriteUint32Le(16, 0xffffffff)
				mem.WriteUint64Le(24, 0xffffffffffffffff)
				mem.WriteUint64Le(32, 0xffffffffffffffff)
				mem.WriteUint64Le(40, 0xffffffffffffffff)
				mem.WriteUint64Le(48, 0xffffffffffffffff)
			},
			calls: []callCase{
				{
					params:     []uint64{0xfffffffe, 0xfffffffe, 0xfffffffe, 0xfffffffffffffffe, 0xfffffffffffffffe, 0xfffffffffffffffe, 0xfffffffffffffffe},
					expResults: []uint64{0xff, 0xffff, 0xffffffff, 0xff, 0xffff, 0xffffffff, 0xffffffffffffffff},
				},
				{
					params:     []uint64{0xffffffee, 0xffffffee, 0xffffffee, 0xffffffffffffffee, 0xffffffffffffffee, 0xffffffffffffffee, 0xffffffffffffffee},
					expResults: []uint64{0xfe, 0xfffe, 0xfffffffe, 0xfe, 0xfffe, 0xfffffffe, 0xfffffffffffffffe},
				},
				{
					params:     []uint64{0xffffffee, 0xffffffee, 0xffffffee, 0xffffffffffffffee, 0xffffffffffffffee, 0xffffffffffffffee, 0xffffffffffffffee},
					expResults: []uint64{0xee, 0xffee, 0xffffffee, 0xee, 0xffee, 0xffffffee, 0xffffffffffffffee},
				},
			},
		},
		{
			name:     "atomic_rmw_or",
			m:        testcases.AtomicRmwOr.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{
					params:     []uint64{0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
				{
					params:     []uint64{0xaf, 0xaf, 0xaf, 0xaf, 0xaf, 0xaf, 0xaf},
					expResults: []uint64{0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f},
				},
				{
					params:     []uint64{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
					expResults: []uint64{0xaf, 0xaf, 0xaf, 0xaf, 0xaf, 0xaf, 0xaf},
				},
			},
		},
		{
			name:     "atomic_rmw_xor",
			m:        testcases.AtomicRmwXor.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{
					params:     []uint64{0, 0, 0, 0, 0, 0, 0},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
				{
					params:     []uint64{0xffffffff, 0xffffffff, 0xffffffff, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
				{
					params:     []uint64{0xffffffff, 0xffffffff, 0xffffffff, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff},
					expResults: []uint64{0xff, 0xffff, 0xffffffff, 0xff, 0xffff, 0xffffffff, 0xffffffffffffffff},
				},
				{
					params:     []uint64{0xffffffff, 0xffffffff, 0xffffffff, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff, 0xffffffffffffffff},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
			},
		},
		{
			name:     "atomic_rmw_xchg",
			m:        testcases.AtomicRmwXchg.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{
					params:     []uint64{1, 2, 3, 4, 5, 6, 7},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
				{
					params:     []uint64{2, 3, 4, 5, 6, 7, 8},
					expResults: []uint64{1, 2, 3, 4, 5, 6, 7},
				},
				{
					params:     []uint64{2, 3, 4, 5, 6, 7, 8},
					expResults: []uint64{2, 3, 4, 5, 6, 7, 8},
				},
			},
		},
		{
			name:     "atomic_cas",
			m:        testcases.AtomicCas.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				// no store
				{
					params:     []uint64{1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
				// store
				{
					params:     []uint64{0, 2, 0, 2, 0, 2, 0, 2, 0, 2, 0, 2, 0, 2},
					expResults: []uint64{0, 0, 0, 0, 0, 0, 0},
				},
				// store
				{
					params:     []uint64{2, 3, 2, 3, 2, 3, 2, 3, 2, 3, 2, 3, 2, 3},
					expResults: []uint64{2, 2, 2, 2, 2, 2, 2},
				},
				// no store
				{
					params:     []uint64{2, 4, 2, 4, 2, 4, 2, 4, 2, 4, 2, 4, 2, 4},
					expResults: []uint64{3, 3, 3, 3, 3, 3, 3},
				},
			},
		},
		{
			// Checks if load works when comparison value is zero. It wouldn't if
			// the zero register gets used.
			name:     "atomic_cas_const0",
			m:        testcases.AtomicCasConst0.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			setupMemory: func(mem api.Memory) {
				mem.WriteUint32Le(0, 1)
				mem.WriteUint32Le(8, 2)
				mem.WriteUint32Le(16, 3)
				mem.WriteUint64Le(24, 4)
				mem.WriteUint64Le(32, 5)
				mem.WriteUint64Le(40, 6)
				mem.WriteUint64Le(48, 7)
			},
			calls: []callCase{
				{
					params:     []uint64{8, 9, 10, 11, 12, 13, 14},
					expResults: []uint64{1, 2, 3, 4, 5, 6, 7},
				},
			},
		},
		{
			name:     "atomic_store_load",
			m:        testcases.AtomicStoreLoad.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{1, 2, 3, 4, 5, 6, 7}, expResults: []uint64{1, 2, 3, 4, 5, 6, 7}},
				{params: []uint64{10, 20, 30, 40, 50, 60, 70}, expResults: []uint64{10, 20, 30, 40, 50, 60, 70}},
			},
		},
		{
			name:     "atomic_fence",
			m:        testcases.AtomicFence.Module,
			features: api.CoreFeaturesV2 | experimental.CoreFeaturesThreads,
			calls: []callCase{
				{params: []uint64{}, expResults: []uint64{}},
			},
		},
		{
			name: "float_le",
			m:    testcases.FloatLe.Module,
			calls: []callCase{
				{params: []uint64{math.Float64bits(1.0)}, expResults: []uint64{1, 1}},
				{params: []uint64{math.Float64bits(0.0)}, expResults: []uint64{1, 1}},
				{params: []uint64{math.Float64bits(1.1)}, expResults: []uint64{0, 0}},
				{params: []uint64{math.Float64bits(math.NaN())}, expResults: []uint64{0, 0}},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 2; i++ {
				var name string
				if i == 0 {
					name = "no cache"
				} else {
					name = "with cache"
				}
				t.Run(name, func(t *testing.T) {
					cache, err := wazero.NewCompilationCacheWithDir(tmp)
					require.NoError(t, err)
					config := wazero.NewRuntimeConfigCompiler().WithCompilationCache(cache)
					if tc.features != 0 {
						config = config.WithCoreFeatures(tc.features)
					}

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

					if tc.setupMemory != nil {
						tc.setupMemory(inst.Memory())
					}

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
								require.Contains(t, err.Error(), cc.expErr)
							} else {
								require.NoError(t, err)
								require.Equal(t, len(cc.expResults), len(result))
								for i := range cc.expResults {
									if cc.expResults[i] != result[i] {
										t.Errorf("result[%d]: exp %x, got %x", i, cc.expResults[i], result[i])
									}
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestE2E_host_functions(t *testing.T) {
	var buf bytes.Buffer
	ctx := experimental.WithFunctionListenerFactory(context.Background(), logging.NewLoggingListenerFactory(&buf))

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "listener", ctx: ctx},
		{name: "no listener", ctx: context.Background()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx

			config := wazero.NewRuntimeConfigCompiler()

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
		})
	}

	require.Equal(t, `
--> .$2(2,100,15,35)
	==> env.square(2,100,15,35)
	<== (4,10000,225,1225)
	==> env.root(35)
	<== 5.916079783099616
<-- (4,10000,225,1225,5.916079783099616)
`, "\n"+buf.String())
}

func TestE2E_stores(t *testing.T) {
	config := wazero.NewRuntimeConfigCompiler()

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

func TestStackUnwind_panic_in_host(t *testing.T) {
	unreachable := &wasm.Module{
		ImportFunctionCount: 1,
		ImportSection:       []wasm.Import{{Module: "host", Name: "cause_unreachable", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		TypeSection:         []wasm.FunctionType{{}},
		ExportSection:       []wasm.Export{{Name: "main", Type: wasm.ExternTypeFunc, Index: 1}},
		FunctionSection:     []wasm.Index{0, 0, 0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 2, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeCall, 3, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // call host.cause_unreachable.
		},
		NameSection: &wasm.NameSection{
			FunctionNames: wasm.NameMap{
				wasm.NameAssoc{Index: 0, Name: "host.unreachable"},
				wasm.NameAssoc{Index: 1, Name: "main"},
				wasm.NameAssoc{Index: 2, Name: "one"},
				wasm.NameAssoc{Index: 3, Name: "two"},
			},
		},
	}

	config := wazero.NewRuntimeConfigCompiler()

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	callUnreachable := func() {
		panic("panic in host function")
	}

	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().WithFunc(callUnreachable).Export("cause_unreachable").
		Instantiate(ctx)
	require.NoError(t, err)

	module, err := r.Instantiate(ctx, binaryencoding.EncodeModule(unreachable))
	require.NoError(t, err)
	defer module.Close(ctx)

	_, err = module.ExportedFunction("main").Call(ctx)
	exp := `panic in host function (recovered by wazero)
wasm stack trace:
	host.cause_unreachable()
	.two()
	.one()
	.main()`
	require.Equal(t, exp, err.Error())
}

func TestStackUnwind_unreachable(t *testing.T) {
	unreachable := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		ExportSection:   []wasm.Export{{Name: "main", Type: wasm.ExternTypeFunc, Index: 0}},
		FunctionSection: []wasm.Index{0, 0, 0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 1, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeCall, 2, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeUnreachable, wasm.OpcodeEnd}},
		},
		NameSection: &wasm.NameSection{
			FunctionNames: wasm.NameMap{
				wasm.NameAssoc{Index: 0, Name: "main"},
				wasm.NameAssoc{Index: 1, Name: "one"},
				wasm.NameAssoc{Index: 2, Name: "two"},
			},
		},
	}

	config := wazero.NewRuntimeConfigCompiler()
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	module, err := r.Instantiate(ctx, binaryencoding.EncodeModule(unreachable))
	require.NoError(t, err)
	defer module.Close(ctx)

	_, err = module.ExportedFunction("main").Call(ctx)
	exp := `wasm error: unreachable
wasm stack trace:
	.two()
	.one()
	.main()`
	require.Equal(t, exp, err.Error())
}

func TestListener_local(t *testing.T) {
	var buf bytes.Buffer
	config := wazero.NewRuntimeConfigCompiler()
	ctx := experimental.WithFunctionListenerFactory(context.Background(), logging.NewLoggingListenerFactory(&buf))

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(testcases.CallIndirect.Module))
	require.NoError(t, err)

	inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	res, err := inst.ExportedFunction(testcases.ExportedFunctionName).Call(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []uint64{10}, res)

	require.Equal(t, `
--> .$0(1)
	--> .$2()
	<-- 10
<-- 10
`, "\n"+buf.String())
}

func TestListener_imported(t *testing.T) {
	var buf bytes.Buffer
	config := wazero.NewRuntimeConfigCompiler()
	ctx := experimental.WithFunctionListenerFactory(context.Background(), logging.NewLoggingListenerFactory(&buf))

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	_, err := r.Instantiate(ctx, binaryencoding.EncodeModule(testcases.ImportedFunctionCall.Imported))
	require.NoError(t, err)

	compiled, err := r.CompileModule(ctx, binaryencoding.EncodeModule(testcases.ImportedFunctionCall.Module))
	require.NoError(t, err)

	inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	require.NoError(t, err)

	res, err := inst.ExportedFunction(testcases.ExportedFunctionName).Call(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, []uint64{10000}, res)

	require.Equal(t, `
--> .$1(100)
	--> env.$0(100,100)
	<-- 10000
<-- 10000
`, "\n"+buf.String())
}

func TestListener_long(t *testing.T) {
	pickOneParam := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{{Results: []wasm.ValueType{i32}, Params: []wasm.ValueType{
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
			i32, i32, f32, f64, i64, i32, i32, v128, f32,
		}}},
		ExportSection:   []wasm.Export{{Name: "main", Type: wasm.ExternTypeFunc, Index: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 10, wasm.OpcodeEnd}},
		},
	})

	var buf bytes.Buffer
	config := wazero.NewRuntimeConfigCompiler()
	ctx := experimental.WithFunctionListenerFactory(context.Background(), logging.NewLoggingListenerFactory(&buf))

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	inst, err := r.Instantiate(ctx, pickOneParam)
	require.NoError(t, err)

	f := inst.ExportedFunction("main")
	require.NotNil(t, f)
	param := make([]uint64, 100)
	for i := range param {
		param[i] = uint64(i)
	}
	res, err := f.Call(ctx, param...)
	require.NoError(t, err)
	require.Equal(t, []uint64{0xb}, res)

	require.Equal(t, `
--> .$0(0,1,3e-45,1.5e-323,4,5,6,00000000000000070000000000000008,1.3e-44,10,11,1.7e-44,6.4e-323,14,15,16,00000000000000110000000000000012,2.7e-44,20,21,3.1e-44,1.14e-322,24,25,26,000000000000001b000000000000001c,4e-44,30,31,4.5e-44,1.63e-322,34,35,36,00000000000000250000000000000026,5.5e-44,40,41,5.9e-44,2.1e-322,44,45,46,000000000000002f0000000000000030,6.9e-44,50,51,7.3e-44,2.6e-322,54,55,56,0000000000000039000000000000003a,8.3e-44,60,61,8.7e-44,3.1e-322,64,65,66,00000000000000430000000000000044,9.7e-44,70,71,1.01e-43,3.6e-322,74,75,76,000000000000004d000000000000004e,1.11e-43,80,81,1.15e-43,4.1e-322,84,85,86,00000000000000570000000000000058,1.25e-43,90,91,1.29e-43,4.6e-322,94,95,96,00000000000000610000000000000062,1.39e-43)
<-- 11
`, "\n"+buf.String())
}

func TestListener_long_as_is(t *testing.T) {
	params := []wasm.ValueType{
		i32, i64, i32, i64, i32, i64, i32, i64, i32, i64,
		i32, i64, i32, i64, i32, i64, i32, i64, i32, i64,
	}

	const paramNum = 20

	var body []byte
	for i := 0; i < paramNum; i++ {
		body = append(body, wasm.OpcodeLocalGet)
		body = append(body, leb128.EncodeUint32(uint32(i))...)
	}
	body = append(body, wasm.OpcodeEnd)

	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{Results: params, Params: params}},
		ExportSection:   []wasm.Export{{Name: "main", Type: wasm.ExternTypeFunc, Index: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection:     []wasm.Code{{Body: body}},
	})

	var buf bytes.Buffer
	config := wazero.NewRuntimeConfigCompiler()
	ctx := experimental.WithFunctionListenerFactory(context.Background(), logging.NewLoggingListenerFactory(&buf))

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	inst, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	f := inst.ExportedFunction("main")
	require.NotNil(t, f)
	param := make([]uint64, paramNum)
	for i := range param {
		param[i] = uint64(i)
	}
	res, err := f.Call(ctx, param...)
	require.NoError(t, err)
	require.Equal(t, param, res)

	require.Equal(t, `
--> .$0(0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19)
<-- (0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19)
`, "\n"+buf.String())
}

func TestListener_long_many_consts(t *testing.T) {
	const paramNum = 61

	var exp []uint64
	var body []byte
	var resultTypes []wasm.ValueType
	for i := 0; i < paramNum; i++ {
		exp = append(exp, uint64(i))
		resultTypes = append(resultTypes, i32)
		body = append(body, wasm.OpcodeI32Const)
		body = append(body, leb128.EncodeInt32(int32(i))...)
	}
	body = append(body, wasm.OpcodeEnd)

	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{Results: resultTypes}},
		ExportSection:   []wasm.Export{{Name: "main", Type: wasm.ExternTypeFunc, Index: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection:     []wasm.Code{{Body: body}},
	})

	var buf bytes.Buffer
	config := wazero.NewRuntimeConfigCompiler()
	ctx := experimental.WithFunctionListenerFactory(context.Background(), logging.NewLoggingListenerFactory(&buf))

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer func() {
		require.NoError(t, r.Close(ctx))
	}()

	inst, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	f := inst.ExportedFunction("main")
	require.NotNil(t, f)
	res, err := f.Call(ctx)
	require.NoError(t, err)
	require.Equal(t, exp, res)

	require.Equal(t, `
--> .$0()
<-- (0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60)
`, "\n"+buf.String())
}

// TestDWARF verifies that the DWARF based stack traces work as expected before/after compilation cache.
func TestDWARF(t *testing.T) {
	config := wazero.NewRuntimeConfigCompiler()
	ctx := context.Background()

	bin := dwarftestdata.ZigWasm

	dir := t.TempDir()

	var expErr error
	{
		cc, err := wazero.NewCompilationCacheWithDir(dir)
		require.NoError(t, err)
		rc := config.WithCompilationCache(cc)

		r := wazero.NewRuntimeWithConfig(ctx, rc)
		_, expErr = r.Instantiate(ctx, bin)
		require.Error(t, expErr)

		err = r.Close(ctx)
		require.NoError(t, err)
	}

	cc, err := wazero.NewCompilationCacheWithDir(dir)
	require.NoError(t, err)
	rc := config.WithCompilationCache(cc)
	r := wazero.NewRuntimeWithConfig(ctx, rc)
	_, err = r.Instantiate(ctx, bin)
	require.Error(t, err)
	require.Equal(t, expErr.Error(), err.Error())

	err = r.Close(ctx)
	require.NoError(t, err)
}
