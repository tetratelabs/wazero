package modgen

import (
	"context"
	"encoding/hex"
	"math/rand"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

const (
	i32 = wasm.ValueTypeI32
	i64 = wasm.ValueTypeI64
	f32 = wasm.ValueTypeF32
	f64 = wasm.ValueTypeF64
)

// TestModGen is like a end-to-end test and verifies that our module generator only generates valid compilable modules.
func TestModGen(t *testing.T) {
	tested := map[string]struct{}{}
	rand := rand.New(rand.NewSource(0)) // use deterministic seed source for easy debugging.
	runtime := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfig().WithWasmCore2())
	for _, size := range []int{1, 2, 5, 10, 50, 100} {
		for i := 0; i < 100; i++ {
			seed := make([]byte, size)
			_, err := rand.Read(seed)
			require.NoError(t, err)
			encoded := hex.EncodeToString(seed)
			if _, ok := tested[encoded]; ok {
				continue
			}
			t.Run(encoded, func(t *testing.T) {
				numTypes, numFunctions, numImports, numExports, numGlobals, numElements, numData :=
					rand.Intn(size), rand.Intn(size), rand.Intn(size), rand.Intn(size), rand.Intn(size), rand.Intn(size), rand.Intn(size)
				needStartSection := rand.Intn(2) == 0

				// Generate a random WebAssembly module.
				m := Gen(seed, wasm.FeatureMultiValue,
					uint32(numTypes), uint32(numFunctions), uint32(numImports), uint32(numExports), uint32(numGlobals), uint32(numElements), uint32(numData), needStartSection)
				// Encode the generated module (*wasm.Module) as binary.
				bin := binary.EncodeModule(m)
				// Pass the generated binary into our compilers.
				code, err := runtime.CompileModule(testCtx, bin, wazero.NewCompileConfig())
				require.NoError(t, err)
				err = code.Close(testCtx)
				require.NoError(t, err)
			})
		}
	}
}

// testRand implements randm for testing.
type testRand struct {
	ints   []int
	intPos int
	bufs   [][]byte
	bufPos int
}

var _ random = &testRand{}

func (tr *testRand) Intn(n int) int {
	ret := tr.ints[tr.intPos] % n
	tr.intPos = (tr.intPos + 1) % len(tr.ints)
	return ret
}

func (tr *testRand) Read(p []byte) (n int, err error) {
	buf := tr.bufs[tr.bufPos]
	copy(p, buf)
	tr.bufPos = (tr.bufPos + 1) % len(tr.bufs)
	return
}

func newGenerator(size int, ints []int, bufs [][]byte) *generator {
	return &generator{size: size,
		rands: []random{&testRand{ints: ints, bufs: bufs}},
		m:     &wasm.Module{},
	}
}

func TestGenerator_newFunctionType(t *testing.T) {
	tests := []struct {
		params, results int
		exp             *wasm.FunctionType
	}{
		{params: 1, results: 1, exp: &wasm.FunctionType{
			Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i64}}},
		{params: 1, results: 2, exp: &wasm.FunctionType{
			Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i64, f32}}},
		{params: 2, results: 2, exp: &wasm.FunctionType{
			Params: []wasm.ValueType{i32, i64}, Results: []wasm.ValueType{f32, f64}}},
		{params: 0, results: 2, exp: &wasm.FunctionType{
			Results: []wasm.ValueType{i32, i64}}},
		{params: 2, results: 0, exp: &wasm.FunctionType{
			Params: []wasm.ValueType{i32, i64}}},
		{params: 6, results: 6, exp: &wasm.FunctionType{
			Params:  []wasm.ValueType{i32, i64, f32, f64, i32, i32},
			Results: []wasm.ValueType{i64, f32, f64, i32, i32, i64},
		},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.exp.String(), func(t *testing.T) {
			g := newGenerator(0, []int{0, 1, 2, 3, 4}, nil)
			actual := g.newFunctionType(tc.params, tc.results)
			require.Equal(t, tc.exp.String(), actual.String())
		})
	}
}

func TestGenerator_newValueType(t *testing.T) {
	g := newGenerator(0, []int{0, 1, 2, 3, 4, 5}, nil)
	require.Equal(t,
		[]wasm.ValueType{i32, i64, f32, f64, i32},
		[]wasm.ValueType{g.newValueType(), g.newValueType(), g.newValueType(), g.newValueType(), g.newValueType()},
	)
}

func TestGenerator_typeSection(t *testing.T) {
	g := newGenerator(100, []int{
		// Params = 1, Reults = 1, i32, i32
		1, 1, 0, 0,
		// Params = 2, Results = 2, i32f32, i64f64
		2, 2, 0, 2, 1, 3,
		// Params = 0, Results = 4, f64f32i64i32
		0, 4, 3, 2, 1, 0,
	}, nil)
	g.enabledFeature = wasm.FeatureMultiValue
	g.numTypes = 3
	g.genTypeSection()

	expected := []*wasm.FunctionType{
		{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}},
		{Params: []wasm.ValueType{i32, f32}, Results: []wasm.ValueType{i64, f64}},
		{Params: []wasm.ValueType{}, Results: []wasm.ValueType{f64, f32, i64, i32}},
	}

	require.Equal(t, len(expected), len(g.m.TypeSection))
	for i := range expected {
		require.Equal(t, expected[i].String(), g.m.TypeSection[i].String())
	}
}

func TestGenerator_importSection(t *testing.T) {
	five := uint32(5)
	tests := []struct {
		ints       []int
		numImports uint32
		types      []*wasm.FunctionType
		exp        []*wasm.Import
	}{
		{
			types:      []*wasm.FunctionType{{}},
			numImports: 2,
			ints: []int{
				// function type with type of index = 0  x 2
				0, 0,
				0, 0,
			},
			exp: []*wasm.Import{
				{Type: wasm.ExternTypeFunc, DescFunc: 0, Module: "module-0", Name: "0"},
				{Type: wasm.ExternTypeFunc, DescFunc: 0, Module: "module-1", Name: "1"},
			},
		},
		{
			types:      []*wasm.FunctionType{{}, {}},
			numImports: 2,
			ints: []int{
				// function type with type of index = 1
				0, 1,
				// function type with type of index = 0
				0, 0,
			},
			exp: []*wasm.Import{
				{Type: wasm.ExternTypeFunc, DescFunc: 1, Module: "module-0", Name: "0"},
				{Type: wasm.ExternTypeFunc, DescFunc: 0, Module: "module-1", Name: "1"},
			},
		},
		{
			types:      []*wasm.FunctionType{{}, {}},
			numImports: 2,
			ints: []int{
				// function type with type of index = 1
				0, 1,
				// global type with type f32, and mutable
				1, 2, 0,
			},
			exp: []*wasm.Import{
				{Type: wasm.ExternTypeFunc, DescFunc: 1, Module: "module-0", Name: "0"},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{
					Mutable: true, ValType: f32,
				}, Module: "module-1", Name: "1"},
			},
		},
		{
			types:      []*wasm.FunctionType{{}, {}},
			numImports: 2,
			ints: []int{
				// function type with type of index = 1
				0, 1,
				// global type with type f64, and immutable
				1, 3, 1,
			},
			exp: []*wasm.Import{
				{Type: wasm.ExternTypeFunc, DescFunc: 1, Module: "module-0", Name: "0"},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{
					Mutable: false, ValType: f64,
				}, Module: "module-1", Name: "1"},
			},
		},
		{
			numImports: 2,
			ints: []int{
				// global type with type i32, and mutable
				0 /* func types are not given so 0 should be global */, 0, 0,
				// global type with type f64, and immutable
				1, 3, 1,
			},
			exp: []*wasm.Import{
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{
					Mutable: true, ValType: i32,
				}, Module: "module-0", Name: "0"},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{
					Mutable: false, ValType: f64,
				}, Module: "module-1", Name: "1"},
			},
		},
		{
			types:      []*wasm.FunctionType{{}, {}},
			numImports: 3,
			ints: []int{
				// Memory import with min=1,max=3,
				2, 1, 2, /* max is rand + min = 2 + 1 = 3 */
				// Table import with min=1,max=5
				2, 1, 4, /* max is rand + min = 4 + 1 = 5 */
				// function type with type of index = 1
				2, 1,
			},
			exp: []*wasm.Import{
				{Type: wasm.ExternTypeMemory, DescMem: &wasm.Memory{
					Min: 1, Max: 3, IsMaxEncoded: true,
				}, Module: "module-0", Name: "0"},
				{Type: wasm.ExternTypeTable, DescTable: &wasm.Table{
					Min: 1, Max: &five,
				}, Module: "module-1", Name: "1"},
				{Type: wasm.ExternTypeFunc, DescFunc: 1, Module: "module-2", Name: "2"},
			},
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			g := newGenerator(100, tc.ints, nil)
			g.numImports = uint32(tc.numImports)
			g.m.TypeSection = tc.types

			g.genImportSection()
			require.Equal(t, len(tc.exp), len(g.m.ImportSection))

			for i := range tc.exp {
				require.Equal(t, tc.exp[i], g.m.ImportSection[i])
			}
		})
	}
}

func TestGenerator_functionSection(t *testing.T) {
	t.Run("with types", func(t *testing.T) {
		types := make([]*wasm.FunctionType, 10)
		g := newGenerator(100, []int{
			100001, 105, 102, 100000003, 104, 6, 9, 8, 7, 10000,
		}, nil)
		g.numFunctions = 10
		g.m.TypeSection = types
		g.genFunctionSection()
		require.Equal(t, []uint32{1, 5, 2, 3, 4, 6, 9, 8, 7, 0}, g.m.FunctionSection)
	})
	t.Run("without types", func(t *testing.T) {
		g := newGenerator(100, []int{10}, nil)
		g.genFunctionSection()
		require.Equal(t, []uint32(nil), g.m.FunctionSection)
	})
}

func TestGenerator_tableSection(t *testing.T) {
	t.Run("with table import", func(t *testing.T) {
		g := newGenerator(100, []int{10}, nil)
		g.m.ImportSection = append(g.m.ImportSection, &wasm.Import{Type: wasm.ExternTypeTable})

		g.genTableSection()
		require.Nil(t, g.m.TableSection)
	})
	t.Run("without table import", func(t *testing.T) {
		g := newGenerator(100, []int{1, 100}, nil)
		g.genTableSection()
		expMax := uint32(101)
		require.Equal(t, []*wasm.Table{{Min: 1, Max: &expMax}}, g.m.TableSection)
	})
}

func TestGenerator_memorySection(t *testing.T) {
	t.Run("with memory import", func(t *testing.T) {
		g := newGenerator(100, []int{10}, nil)
		g.m.ImportSection = append(g.m.ImportSection, &wasm.Import{Type: wasm.ExternTypeMemory})

		g.genMemorySection()
		require.Nil(t, g.m.MemorySection)
	})
	t.Run("without memory import", func(t *testing.T) {
		g := newGenerator(100, []int{1, 100}, nil)
		g.genMemorySection()
		require.Equal(t, &wasm.Memory{Min: 1, Max: 101, IsMaxEncoded: true}, g.m.MemorySection)
	})
}

func TestGenerator_newConstExpr(t *testing.T) {
	tests := []struct {
		ints    []int
		bufs    [][]byte
		imports []*wasm.Import
		exp     *wasm.ConstantExpression
		expType wasm.ValueType
	}{
		{
			ints: []int{
				0,   // i32.const
				100, // 100 literal
				1,   // non-negative
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(100)},
			expType: i32,
		},
		{
			ints: []int{
				0,   // i2.const
				100, // 100 literal
				0,   // negative
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(-100)},
			expType: i32,
		},
		{
			ints: []int{
				1,   // i64.const
				100, // 100 literal
				1,   // non-negative
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(100)},
			expType: i64,
		},
		{
			ints: []int{
				1,   // i64.const
				100, // 100 literal
				0,   // negative
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(-100)},
			expType: i64,
		},
		{
			ints:    []int{2}, // f32.const
			bufs:    [][]byte{{1, 2, 3, 4}},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeF32Const, Data: []byte{1, 2, 3, 4}},
			expType: f32,
		},
		{
			ints:    []int{3}, // f64.const
			bufs:    [][]byte{{1, 2, 3, 4, 5, 6, 7, 8}},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeF64Const, Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}},
			expType: f64,
		},
		{
			ints: []int{
				4, // globa.get
				1, // 2nd global.
			},
			imports: []*wasm.Import{
				{},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{ValType: i32}},
				{},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{ValType: f32}},
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeGlobalGet, Data: []byte{1}},
			expType: f32,
		},
		{
			ints: []int{
				4, // globa.get
				0, // 1st global.
			},
			imports: []*wasm.Import{
				{},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{ValType: i32}},
				{},
				{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{ValType: f32}},
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeGlobalGet, Data: []byte{0}},
			expType: i32,
		},
		{
			ints: []int{
				4,   // there's no imports, so this should result in i32.const.
				100, // 100 literal
				1,   // non-negative
			},
			exp:     &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(100)},
			expType: i32,
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			g := newGenerator(100, tc.ints, tc.bufs)
			g.m.ImportSection = tc.imports
			actualExpr, actualType := g.newConstExpr()
			require.Equal(t, tc.expType, actualType)
			require.Equal(t, tc.exp, actualExpr)
		})
	}
}

func TestGenerator_globalSection(t *testing.T) {
	g := newGenerator(100,
		[]int{
			// i32.const 123, mutable
			0 /* i32.const */, 123, 1 /* non negative */, 0, /* mutable */
			3 /* f64.const */, 1, /* immutable */
			4 /* global.get */, 0 /* global index */, 1, /* immutable */
		},
		[][]byte{{1, 2, 3, 4, 5, 6, 7, 8}},
	)

	g.m.ImportSection = []*wasm.Import{{}, {},
		{Type: wasm.ExternTypeGlobal, DescGlobal: &wasm.GlobalType{ValType: f32}},
	}
	g.numGlobals = 3

	g.genGlobalSection()

	expected := []*wasm.Global{
		{
			Type: &wasm.GlobalType{ValType: i32, Mutable: true},
			Init: &wasm.ConstantExpression{
				Opcode: wasm.OpcodeI32Const,
				Data:   leb128.EncodeInt32(123),
			},
		},
		{
			Type: &wasm.GlobalType{ValType: f64, Mutable: false},
			Init: &wasm.ConstantExpression{
				Opcode: wasm.OpcodeF64Const,
				Data:   []byte{1, 2, 3, 4, 5, 6, 7, 8},
			},
		},
		{
			Type: &wasm.GlobalType{ValType: f32, Mutable: false},
			Init: &wasm.ConstantExpression{
				Opcode: wasm.OpcodeGlobalGet,
				Data:   []byte{0},
			},
		},
	}

	actual := g.m.GlobalSection
	require.Equal(t, len(expected), len(actual))

	for i := range actual {
		require.Equal(t, expected[i], actual[i])
	}
}

func TestGenerator_exportSection(t *testing.T) {
	m := &wasm.Module{
		FunctionSection: make([]uint32, 10),
		GlobalSection: []*wasm.Global{
			{Type: &wasm.GlobalType{}},
			{Type: &wasm.GlobalType{}},
			{Type: &wasm.GlobalType{}},
			{Type: &wasm.GlobalType{}},
			{Type: &wasm.GlobalType{}},
		},
		TableSection:  []*wasm.Table{{}},
		MemorySection: &wasm.Memory{},
	}

	g := newGenerator(100, []int{
		2,  // funcs[2]
		13, // globals[3]
		15, // table
		16, // memory
		7,  // funcs[7]
	}, nil)
	g.m = m

	g.numExports = 5
	g.genExportSection()

	expected := []*wasm.Export{
		{Type: wasm.ExternTypeFunc, Index: 2, Name: "0"},
		{Type: wasm.ExternTypeGlobal, Index: 3, Name: "1"},
		{Type: wasm.ExternTypeTable, Index: 0, Name: "2"},
		{Type: wasm.ExternTypeMemory, Index: 0, Name: "3"},
		{Type: wasm.ExternTypeFunc, Index: 7, Name: "4"},
	}

	actual := g.m.ExportSection

	require.Equal(t, len(expected), len(actual))
	for i := range actual {
		require.Equal(t, expected[i], actual[i])
	}
}

func TestGenerator_startSection(t *testing.T) {
	t.Run("with cand", func(t *testing.T) {

		m := &wasm.Module{
			ImportSection: []*wasm.Import{
				{Type: wasm.ExternTypeFunc, DescFunc: 0},
				{Type: wasm.ExternTypeFunc, DescFunc: 1}, // candidate
				{Type: wasm.ExternTypeFunc, DescFunc: 2},
				{Type: wasm.ExternTypeFunc, DescFunc: 1}, // candidate
			},
			FunctionSection: []uint32{
				0, 0, 1 /* candidate */, 0,
			},
			TypeSection: []*wasm.FunctionType{
				{Params: []wasm.ValueType{i32}},
				{},
				{Params: []wasm.ValueType{f32}},
				{Params: []wasm.ValueType{f64}},
			},
		}

		takePtr := func(v uint32) *uint32 { return &v }

		tests := []struct {
			ints []int
			exp  *wasm.Index
		}{
			{ints: []int{0}, exp: takePtr(1)},
			{ints: []int{1}, exp: takePtr(3)},
			{ints: []int{2}, exp: takePtr(6)},
			{ints: []int{3}, exp: takePtr(1)},
		}

		for i, tt := range tests {
			tc := tt
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				g := newGenerator(100, tc.ints, nil)
				g.needStartSection = true
				g.m = m

				g.genStartSection()
				require.Equal(t, tc.exp, g.m.StartSection)
			})
		}
	})

	t.Run("no cand", func(t *testing.T) {
		m := &wasm.Module{
			ImportSection: []*wasm.Import{
				{Type: wasm.ExternTypeFunc, DescFunc: 0},
				{Type: wasm.ExternTypeFunc, DescFunc: 1},
				{Type: wasm.ExternTypeFunc, DescFunc: 2},
				{Type: wasm.ExternTypeFunc, DescFunc: 1},
			},
			FunctionSection: []uint32{
				0, 1, 2, 0, 1, 2,
			},
			TypeSection: []*wasm.FunctionType{
				{Params: []wasm.ValueType{i32}},
				{Params: []wasm.ValueType{f32}},
				{Params: []wasm.ValueType{f64}},
			},
		}
		g := newGenerator(100, []int{0}, nil)
		g.m = m

		g.genStartSection()
		require.Nil(t, g.m.StartSection)
	})
}

func TestGenerator_elementSection(t *testing.T) {
	uint32Ptr := func(in uint32) *uint32 {
		return &in
	}

	t.Run("without table", func(t *testing.T) {
		g := newGenerator(100, nil, nil)
		g.genElementSection()
		require.Nil(t, g.m.ElementSection)
	})
	t.Run("without function", func(t *testing.T) {
		g := newGenerator(100, nil, nil)
		g.m.TableSection = []*wasm.Table{{}}
		g.genElementSection()
		require.Nil(t, g.m.ElementSection)
	})
	t.Run("ok", func(t *testing.T) {
		tests := []struct {
			ints        []int
			numElements uint32
			exps        []*wasm.ElementSegment
		}{
			{
				numElements: 1,
				ints: []int{
					2 /* number of inedxes */, 0, 50, 98, /* offset */
				},
				exps: []*wasm.ElementSegment{
					{
						OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(98)},
						Init:       []*wasm.Index{uint32Ptr(0), uint32Ptr(50)},
						Type:       wasm.RefTypeFuncref,
					},
				},
			},
			{
				numElements: 3,
				ints: []int{
					2 /* number of inedxes */, 25, 75, 0, /* offset */
					1 /* number of inedxes */, 3, 99, /* offset */
					10 /* number of inedxes */, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 /* offset */, 90,
				},
				exps: []*wasm.ElementSegment{
					{
						OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(0)},
						Type:       wasm.RefTypeFuncref,
						Init:       []*wasm.Index{uint32Ptr(25), uint32Ptr(75)},
					},
					{
						OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(99)},
						Type:       wasm.RefTypeFuncref,
						Init:       []*wasm.Index{uint32Ptr(3)},
					},
					{
						OffsetExpr: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(90)},
						Type:       wasm.RefTypeFuncref,
						Init: []*wasm.Index{uint32Ptr(1), uint32Ptr(2), uint32Ptr(3), uint32Ptr(4),
							uint32Ptr(5), uint32Ptr(6), uint32Ptr(7), uint32Ptr(8), uint32Ptr(9), uint32Ptr(10)},
					},
				},
			},
		}

		for i, tt := range tests {
			tc := tt
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				g := newGenerator(100, tc.ints, nil)
				g.m = &wasm.Module{
					TableSection:    []*wasm.Table{{Min: 100}},
					FunctionSection: make([]uint32, 100),
				}
				g.numElements = tc.numElements

				g.genElementSection()
				actual := g.m.ElementSection
				require.Equal(t, len(tc.exps), len(actual))
				for i := range actual {
					require.Equal(t, tc.exps[i], actual[i])
				}
			})
		}
	})
}

func TestGenerator_dataSection(t *testing.T) {
	t.Run("without memory", func(t *testing.T) {
		g := newGenerator(100, nil, nil)
		g.genDataSection()
		require.Nil(t, g.m.DataSection)
	})
	t.Run("min=0", func(t *testing.T) {
		g := newGenerator(100, nil, nil)
		g.m.MemorySection = &wasm.Memory{Min: 0}
		g.genDataSection()
		require.Nil(t, g.m.DataSection)
	})
	t.Run("ok", func(t *testing.T) {
		tests := []struct {
			ints    []int
			bufs    [][]byte
			numData uint32
			exps    []*wasm.DataSegment
		}{
			{
				numData: 1,
				ints: []int{
					5, // offset
					2, // size of inits
				},
				bufs: [][]byte{{0x1, 0x2}},
				exps: []*wasm.DataSegment{
					{
						OffsetExpression: &wasm.ConstantExpression{
							Opcode: wasm.OpcodeI32Const,
							Data:   leb128.EncodeUint32(5),
						},
						Init: []byte{0x1, 0x2},
					},
				},
			},
			{
				numData: 1,
				ints: []int{
					int(wasm.MemoryLimitPages) - 1, // offset
					1,                              // size of inits
				},
				bufs: [][]byte{{0x1}},
				exps: []*wasm.DataSegment{
					{
						OffsetExpression: &wasm.ConstantExpression{
							Opcode: wasm.OpcodeI32Const,
							Data:   leb128.EncodeUint32(uint32(wasm.MemoryLimitPages) - 1),
						},
						Init: []byte{0x1},
					},
				},
			},
		}

		for i, tt := range tests {
			tc := tt
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				g := newGenerator(100, tc.ints, tc.bufs)
				g.m.MemorySection = &wasm.Memory{Min: 1}
				g.numData = tc.numData
				g.genDataSection()
				actual := g.m.DataSection
				require.Equal(t, len(tc.exps), len(actual))
				for i := range actual {
					require.Equal(t, tc.exps[i], actual[i])
				}
			})
		}
	})
}
