package cranelift

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine(context.Background(), craneliftFeature, nil)
	require.NotNil(t, e)
}

const (
	i32      = wasm.ValueTypeI32
	i64      = wasm.ValueTypeI64
	f32      = wasm.ValueTypeF32
	f64      = wasm.ValueTypeF64
	end      = wasm.OpcodeEnd
	localGet = wasm.OpcodeLocalGet
	i32Const = wasm.OpcodeI32Const
)

func Test_CompileModule(t *testing.T) {
	e := NewEngine(context.Background(), craneliftFeature, nil).(*engine)
	require.NotNil(t, e)
	defer func() {
		require.NoError(t, e.Close())
	}()

	for _, tc := range []struct {
		name string
		m    *wasm.Module
	}{
		{
			name: "empty",
			m: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{LocalTypes: []byte{i32}, Body: []byte{end}}},
			},
		},
		{
			name: "one param",
			m: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{{}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: []byte{end}}},
			},
		},
		{
			name: "two params",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}}},
				CodeSection:     []*wasm.Code{{Body: []byte{end}}},
			},
		},
		{
			name: "one result",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Results: []wasm.ValueType{i32}}},
				CodeSection: []*wasm.Code{{Body: []byte{
					i32Const, 1,
					end,
				}}},
			},
		},
		{
			name: "two results",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Results: []wasm.ValueType{i32, i32}}},
				CodeSection: []*wasm.Code{{Body: []byte{
					i32Const, 1,
					i32Const, 2,
					end,
				}}},
			},
		},
		{
			name: "add one to param",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}},
				CodeSection: []*wasm.Code{{Body: []byte{
					localGet, 0,
					i32Const, 1,
					wasm.OpcodeI32Add,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			name: "add two params",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
				CodeSection: []*wasm.Code{{Body: []byte{
					localGet, 0,
					localGet, 1,
					wasm.OpcodeI32Add,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			name: "swap two params",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}}},
				CodeSection: []*wasm.Code{{Body: []byte{
					localGet, 1,
					localGet, 0,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			name: "return many",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection: []*wasm.FunctionType{
					{
						Results: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32},
					},
				},
				CodeSection: []*wasm.Code{{Body: []byte{
					i32Const, 100, i32Const, 19, i32Const, 18, i32Const, 17, i32Const, 16,
					i32Const, 15, i32Const, 14, i32Const, 13, i32Const, 12, i32Const, 11,
					i32Const, 10, i32Const, 9, i32Const, 8, i32Const, 7, i32Const, 6,
					i32Const, 5, i32Const, 4, i32Const, 3, i32Const, 2, i32Const, 1,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			name: "reverse all params",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection: []*wasm.FunctionType{
					{
						Params: []wasm.ValueType{
							i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, f32, i32, i32, i32, i32, i32, i32, i32, i32, i32, f32,
						},
						Results: []wasm.ValueType{f32, i32, i32, i32, i32, i32, i32, i32, i32, i32, f32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32},
					},
				},
				CodeSection: []*wasm.Code{{Body: []byte{
					localGet, 21, localGet, 20, localGet, 19, localGet, 18, localGet, 17, localGet, 16, localGet, 15,
					localGet, 14, localGet, 13, localGet, 12, localGet, 11, localGet, 10,
					localGet, 9, localGet, 8, localGet, 7, localGet, 6, localGet, 5,
					localGet, 4, localGet, 3, localGet, 2, localGet, 1, localGet, 0,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			name: "reverse all params 2",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection: []*wasm.FunctionType{
					{
						Params: []wasm.ValueType{
							i32, i32, i32, i32, i32, i32, i32, i32, i32, i32,
							f32, f32, f32, f32, f32, f32, f32, f32, f32, f32,
						},
						Results: []wasm.ValueType{
							f32, f32, f32, f32, f32, f32, f32, f32, f32, f32,
							i32, i32, i32, i32, i32, i32, i32, i32, i32, i32,
						},
					},
				},
				CodeSection: []*wasm.Code{{Body: []byte{
					localGet, 19, localGet, 18, localGet, 17, localGet, 16, localGet, 15,
					localGet, 14, localGet, 13, localGet, 12, localGet, 11, localGet, 10,
					localGet, 9, localGet, 8, localGet, 7, localGet, 6, localGet, 5,
					localGet, 4, localGet, 3, localGet, 2, localGet, 1, localGet, 0,
					wasm.OpcodeEnd,
				}}},
			},
		},
		{
			name: "recursive call",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}}},
				CodeSection: []*wasm.Code{{LocalTypes: []byte{}, Body: []byte{
					i32Const, 1, i32Const, 2,
					wasm.OpcodeCall, 0,
					end,
				}}},
			},
		},
		{
			name: "mutual call",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0, 0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{i32}}},
				CodeSection: []*wasm.Code{
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeI32Add,
						wasm.OpcodeCall, 1,
						end,
					}},
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeI32Mul,
						wasm.OpcodeCall, 0,
						end,
					}},
				},
			},
		},
		{
			name: "memory access",
			m: &wasm.Module{
				MemorySection:   &wasm.Memory{},
				FunctionSection: []wasm.Index{0},
				TypeSection:     []*wasm.FunctionType{{}},
				CodeSection: []*wasm.Code{
					{Body: []byte{
						wasm.OpcodeI32Const, 0,
						wasm.OpcodeI64Const, 12,
						wasm.OpcodeI64Store, 0x2, 0x0,
						end,
					}},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.m.ID = sha256.Sum256([]byte(tc.name))

			err := e.CompileModule(context.Background(), tc.m, nil)
			require.NoError(t, err, e.craneLiftInst.stderr.String())
			require.Zero(t, len(e.pendingCompiledFunctions))

			compiled, ok := e.modules[tc.m.ID]
			require.True(t, ok)

			require.Equal(t, len(tc.m.CodeSection), len(compiled.executableOffsets))

			t.Log(hex.EncodeToString(compiled.executable))
		})
	}
}

func TestCallingConventions(t *testing.T) {
	e := NewEngine(context.Background(), craneliftFeature, nil).(*engine)
	require.NotNil(t, e)
	defer func() {
		require.NoError(t, e.Close())
	}()

	tests := []struct {
		name         string
		typ          *wasm.FunctionType
		body         []byte
		params       []uint64
		checkResults func(t *testing.T, results []uint64)
	}{
		{
			name:         "v_v",
			typ:          &wasm.FunctionType{},
			body:         []byte{i32Const, 1, wasm.OpcodeDrop, end},
			checkResults: func(*testing.T, []uint64) {},
		},
		{
			name: "v_i32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{i32}},
			body: []byte{i32Const, 1, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint32(1), uint32(results[0]))
			},
		},
		{
			name: "v_i32i32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{i32, i32}},
			body: []byte{i32Const, 5, i32Const, 9, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint32(5), uint32(results[0]))
				require.Equal(t, uint32(9), uint32(results[1]))
			},
		},
		{
			name: "v_i32i32f64",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{i32, i32, f32}},
			body: []byte{i32Const, 5, i32Const, 9, wasm.OpcodeF32Const, 1, 2, 3, 4, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint32(5), uint32(results[0]))
				require.Equal(t, uint32(9), uint32(results[1]))
				require.Equal(t,
					math.Float32frombits(binary.LittleEndian.Uint32([]byte{1, 2, 3, 4})),
					math.Float32frombits(uint32(results[2])),
				)
			},
		},
		{
			name: "v_i64",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{i64}},
			body: []byte{wasm.OpcodeI64Const, 1, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint64(1), results[0])
			},
		},
		{
			name: "v_i64i32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{i64, i32}},
			body: []byte{wasm.OpcodeI64Const, 10, i32Const, 5, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint64(10), results[0])
				require.Equal(t, uint32(5), uint32(results[1]))
			},
		},
		{
			name: "v_f32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{f32}},
			body: []byte{wasm.OpcodeF32Const, 1, 2, 3, 4, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t,
					math.Float32frombits(binary.LittleEndian.Uint32([]byte{1, 2, 3, 4})),
					math.Float32frombits(uint32(results[0])),
				)
			},
		},
		{
			name: "v_f32i32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{f32, i32}},
			body: []byte{wasm.OpcodeF32Const, 1, 2, 3, 4, i32Const, 5, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t,
					math.Float32frombits(binary.LittleEndian.Uint32([]byte{1, 2, 3, 4})),
					math.Float32frombits(uint32(results[0])),
				)
				require.Equal(t, uint32(5), uint32(results[1]))
			},
		},
		{
			name: "v_f32f32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{f32, f32}},
			body: []byte{
				wasm.OpcodeF32Const, 1, 2, 3, 4,
				wasm.OpcodeF32Const, 5, 4, 3, 2,
				end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t,
					math.Float32frombits(binary.LittleEndian.Uint32([]byte{1, 2, 3, 4})),
					math.Float32frombits(uint32(results[0])),
				)
				require.Equal(t,
					math.Float32frombits(binary.LittleEndian.Uint32([]byte{5, 4, 3, 2})),
					math.Float32frombits(uint32(results[1])),
				)
			},
		},
		{
			name: "v_f64",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{f64}},
			body: []byte{wasm.OpcodeF64Const, 1, 2, 3, 4, 5, 6, 7, 8, end},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t,
					math.Float64frombits(binary.LittleEndian.Uint64([]byte{1, 2, 3, 4, 5, 6, 7, 8})),
					math.Float64frombits(results[0]),
				)
			},
		},
		{
			name: "v_f64i32",
			typ:  &wasm.FunctionType{Results: []wasm.ValueType{f64, i32}},
			body: []byte{
				wasm.OpcodeF64Const, 1, 2, 3, 4, 5, 4, 3, 2,
				i32Const, 11, end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t,
					math.Float64frombits(binary.LittleEndian.Uint64([]byte{1, 2, 3, 4, 5, 4, 3, 2})),
					math.Float64frombits(results[0]),
				)
				require.Equal(t, uint32(11), uint32(results[1]))
			},
		},
		{
			name: "return many",
			typ: &wasm.FunctionType{Results: []wasm.ValueType{
				i32, i32, i32, i32, i32, i32, i32, i32, i32, i32,
				i32, i32, i32, i32, i32, i32, i32, i32, i32, i32,
			}},
			body: []byte{
				i32Const, 20, i32Const, 19, i32Const, 18, i32Const, 17, i32Const, 16,
				i32Const, 15, i32Const, 14, i32Const, 13, i32Const, 12, i32Const, 11,
				i32Const, 10, i32Const, 9, i32Const, 8, i32Const, 7, i32Const, 6,
				i32Const, 5, i32Const, 4, i32Const, 3, i32Const, 2, i32Const, 1,
				wasm.OpcodeEnd,
			},
			checkResults: func(t *testing.T, results []uint64) {
				for i := 0; i < 20; i++ {
					require.Equal(t, uint32(20-i), uint32(results[i]))
				}
			},
		},
		{
			name: "i32i32_v",
			typ: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32},
				Results: []wasm.ValueType{},
			},
			params: []uint64{1, 2},
			body: []byte{
				localGet, 0, localGet, 1, wasm.OpcodeI32Add, wasm.OpcodeDrop,
				end,
			},
			checkResults: func(*testing.T, []uint64) {},
		},
		{
			name: "i32i32_i32",
			typ: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32},
				Results: []wasm.ValueType{i32},
			},
			params: []uint64{1, 10},
			body: []byte{
				localGet, 0, localGet, 1, wasm.OpcodeI32Add,
				end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint32(11), uint32(results[0]))
			},
		},
		{
			name: "i64i64_i32",
			typ: &wasm.FunctionType{
				Params:  []wasm.ValueType{i64, i64},
				Results: []wasm.ValueType{i32},
			},
			params: []uint64{math.MaxUint64, 10},
			body: []byte{
				localGet, 0, localGet, 1, wasm.OpcodeI64Add, wasm.OpcodeI32WrapI64,
				end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				u64max := uint64(math.MaxUint64)
				require.Equal(t, uint32(u64max+10), uint32(results[0]))
			},
		},
		{
			name: "i32i32i32i32i32",
			typ: &wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i32, i32, i32},
				Results: []wasm.ValueType{i32},
			},
			params: []uint64{1, 2, 3, 4, 5},
			body: []byte{
				localGet, 0, localGet, 1, localGet, 2, localGet, 3,
				localGet, 4,
				wasm.OpcodeI32Add,
				wasm.OpcodeI32Add,
				wasm.OpcodeI32Add,
				wasm.OpcodeI32Add,
				end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, uint32(15), uint32(results[0]))
			},
		},
		{
			name: "f32f32_f32",
			typ: &wasm.FunctionType{
				Params:  []wasm.ValueType{f32, f32},
				Results: []wasm.ValueType{f32},
			},
			params: []uint64{uint64(math.Float32bits(1.2)), uint64(math.Float32bits(3.14))},
			body: []byte{
				localGet, 0, localGet, 1, wasm.OpcodeF32Add,
				end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, float32(1.2)+float32(3.14), math.Float32frombits(uint32(results[0])))
			},
		},
		{
			name: "f64f64_f64",
			typ: &wasm.FunctionType{
				Params:  []wasm.ValueType{f64, f64},
				Results: []wasm.ValueType{f64},
			},
			params: []uint64{math.Float64bits(1.2), math.Float64bits(3.14)},
			body: []byte{
				localGet, 0, localGet, 1, wasm.OpcodeF64Add,
				end,
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, 1.2+3.14, math.Float64frombits(results[0]))
			},
		},
		// TODO: add multi results and/or many (> # of param registers) cases.
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			m := &wasm.Module{
				TypeSection:     []*wasm.FunctionType{tc.typ},
				ID:              sha256.Sum256([]byte(tc.name)),
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: tc.body}},
			}
			initCacheNumInUint64(m)

			err := e.CompileModule(ctx, m, nil)
			require.NoError(t, err, e.craneLiftInst.stderr.String())
			require.Zero(t, len(e.pendingCompiledFunctions))

			compiled, ok := e.modules[m.ID]
			require.True(t, ok)

			require.Equal(t, len(m.CodeSection), len(compiled.executableOffsets))

			t.Logf(hex.EncodeToString(compiled.executable))

			inst := moduleInstance(m)
			inst.Engine, err = e.NewModuleEngine(tc.name, m, inst.Functions)
			require.NoError(t, err)

			ce, err := inst.Engine.NewCallEngine(nil, &inst.Functions[0])
			require.NoError(t, err)

			results, err := ce.Call(context.Background(), nil, tc.params)
			require.NoError(t, err)
			tc.checkResults(t, results)
		})
	}
}

func TestEngine_local_function_calls(t *testing.T) {
	e := NewEngine(context.Background(), craneliftFeature, nil).(*engine)
	require.NotNil(t, e)
	defer func() {
		require.NoError(t, e.Close())
	}()

	for _, tc := range []struct {
		name         string
		m            *wasm.Module
		callIndex    int
		params       []uint64
		checkResults func(t *testing.T, results []uint64)
	}{
		{
			name: "simple call",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0, 1},
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
				CodeSection: []*wasm.Code{
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeI32Add,
						end,
					}},
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeCall, 0, // call ^.
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeCall, 0, // call ^.
						wasm.OpcodeI32Add,
						wasm.OpcodeLocalGet, 2,
						wasm.OpcodeLocalGet, 2,
						wasm.OpcodeCall, 0, // call ^.
						wasm.OpcodeI32Add,
						wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 3,
						wasm.OpcodeCall, 0, // call ^.
						wasm.OpcodeI32Add,
						end,
					}},
				},
			},
			callIndex:    1,
			params:       []uint64{1, 2, 3, 4},
			checkResults: func(t *testing.T, results []uint64) { require.Equal(t, uint32(1*2+2*2+3*2+4*2), uint32(results[0])) },
		},
		{
			name: "folding",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0, 1, 2, 3, 4},
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{f64}, Results: []wasm.ValueType{f64}},
					{Params: []wasm.ValueType{f64, f64}, Results: []wasm.ValueType{f64}},
					{Params: []wasm.ValueType{f64, f64, f64}, Results: []wasm.ValueType{f64}},
					{Params: []wasm.ValueType{f64, f64, f64, f64}, Results: []wasm.ValueType{f64}},
					{Params: []wasm.ValueType{f64, f64, f64, f64, f64}, Results: []wasm.ValueType{f64}},
				},
				CodeSection: []*wasm.Code{
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeF64Mul,
						end,
					}},
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeF64Add,
						wasm.OpcodeCall, 0, // call ^.
						end,
					}},
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeLocalGet, 2,
						wasm.OpcodeF64Add,
						wasm.OpcodeCall, 1, // call ^.
						end,
					}},
					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeLocalGet, 2,
						wasm.OpcodeLocalGet, 3,
						wasm.OpcodeF64Add,
						wasm.OpcodeCall, 2, // call ^.
						end,
					}},

					{Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeLocalGet, 1,
						wasm.OpcodeLocalGet, 2,
						wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 4,
						wasm.OpcodeF64Add,
						wasm.OpcodeCall, 3, // call ^.
						end,
					}},
				},
			},
			callIndex: 4,
			params: []uint64{
				math.Float64bits(1.4), math.Float64bits(2.4),
				math.Float64bits(1.4234124), math.Float64bits(151431.4), math.Float64bits(13331.412),
			},
			checkResults: func(t *testing.T, results []uint64) {
				require.Equal(t, math.Float64bits(math.Pow(1.4+2.4+1.4234124+151431.4+13331.412, 2)), results[0])
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initCacheNumInUint64(tc.m)

			tc.m.ID = sha256.Sum256([]byte(tc.name))

			err := e.CompileModule(context.Background(), tc.m, nil)
			require.NoError(t, err, e.craneLiftInst.stderr.String())
			require.Zero(t, len(e.pendingCompiledFunctions))

			compiled, ok := e.modules[tc.m.ID]
			require.True(t, ok)

			require.Equal(t, len(tc.m.CodeSection), len(compiled.executableOffsets))

			t.Logf(hex.EncodeToString(compiled.executable))

			inst := moduleInstance(tc.m)
			inst.Engine, err = e.NewModuleEngine(tc.name, tc.m, inst.Functions)
			require.NoError(t, err)

			ce, err := inst.Engine.NewCallEngine(nil, &inst.Functions[tc.callIndex])
			require.NoError(t, err)

			results, err := ce.Call(context.Background(), nil, tc.params)
			require.NoError(t, err)
			tc.checkResults(t, results)
		})
	}
}

func TestEngine_local_memory_access(t *testing.T) {
	e := NewEngine(context.Background(), craneliftFeature, nil).(*engine)
	require.NotNil(t, e)
	defer func() {
		require.NoError(t, e.Close())
	}()

	for _, tc := range []struct {
		name         string
		m            *wasm.Module
		checkResults func(t *testing.T, memBuf []byte)
	}{
		{
			name: "simple",
			m: &wasm.Module{
				FunctionSection: []wasm.Index{0, 0},
				TypeSection:     []*wasm.FunctionType{{Params: []wasm.ValueType{}}},
				MemorySection:   &wasm.Memory{Min: 1, Max: 1},
				CodeSection: []*wasm.Code{
					{Body: []byte{
						wasm.OpcodeI32Const, 5,
						wasm.OpcodeI64Const, 12,
						// Write []byte{12} at 0 + 5.
						wasm.OpcodeI64Store, 0x0 /* memory index */, 0x1, /* constant offset */
						wasm.OpcodeCall, 1,
						end,
					}},
					{Body: []byte{
						wasm.OpcodeI32Const, 10, // offset
						wasm.OpcodeF32Const, 1, 2, 3, 4,
						// Write []byte{1, 2, 3, 4} at 10 + 2.
						wasm.OpcodeF32Store, 0x0 /* memory index */, 0x5, /* constant offset */
						end,
					}},
				},
			},
			checkResults: func(t *testing.T, memBuf []byte) {
				require.Equal(t, byte(12), memBuf[6])
				require.Equal(t, []byte{1, 2, 3, 4}, memBuf[15:15+4])
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initCacheNumInUint64(tc.m)

			tc.m.ID = sha256.Sum256([]byte(tc.name))

			err := e.CompileModule(context.Background(), tc.m, nil)
			require.NoError(t, err, e.craneLiftInst.stderr.String())
			require.Zero(t, len(e.pendingCompiledFunctions))

			compiled, ok := e.modules[tc.m.ID]
			require.True(t, ok)

			require.Equal(t, len(tc.m.CodeSection), len(compiled.executableOffsets))

			t.Logf(hex.EncodeToString(compiled.executable))

			inst := moduleInstance(tc.m)
			inst.Engine, err = e.NewModuleEngine(tc.name, tc.m, inst.Functions)
			require.NoError(t, err)

			ce, err := inst.Engine.NewCallEngine(nil, &inst.Functions[0])
			require.NoError(t, err)

			_, err = ce.Call(context.Background(), nil, nil)
			require.NoError(t, err)

			tc.checkResults(t, inst.Memory.Buffer)
		})
	}
}

func initCacheNumInUint64(m *wasm.Module) {
	for _, tp := range m.TypeSection {
		tp.CacheNumInUint64()
	}
}

func moduleInstance(m *wasm.Module) *wasm.ModuleInstance {
	inst := &wasm.ModuleInstance{
		Functions: make([]wasm.FunctionInstance, len(m.CodeSection)),
	}

	for i := range m.CodeSection {
		inst.Functions[i].Type = m.TypeSection[m.FunctionSection[i]]
		inst.Functions[i].Module = inst
		inst.Functions[i].Idx = wasm.Index(i)
	}

	if m.MemorySection != nil {
		inst.Memory = &wasm.MemoryInstance{
			Buffer: make([]byte, wasm.MemoryPagesToBytesNum(m.MemorySection.Min)),
			Min:    m.MemorySection.Min,
			Max:    m.MemorySection.Max,
		}
	}
	return inst
}
