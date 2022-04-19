package modgen

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

const (
	i32 = wasm.ValueTypeI32
	i64 = wasm.ValueTypeI64
	f32 = wasm.ValueTypeF32
	f64 = wasm.ValueTypeF64
)

func TestModGen(t *testing.T) {
	for _, size := range []int{0, 1, 2, 5, 10, 100} {
		r := rand.New(rand.NewSource(0))
		for i := 0; i < 10; i++ {
			buf := make([]byte, size)
			_, err := r.Read(buf)
			require.NoError(t, err)
			t.Run(fmt.Sprintf("%d (size=%d)", i, size), func(t *testing.T) {
				m := Gen(buf)
				// Generating with the same seed must result in the same module.
				require.Equal(t, m, Gen(buf))
				buf := binary.EncodeModule(m)

				r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfig().WithFeatureMultiValue(true))
				_, err := r.CompileModule(buf)
				require.NoError(t, err)
			})
		}
	}
}

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
	for _, tc := range []struct {
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
	} {
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
		3, // type section size == 3
		// Params = 1, Reults = 1, i32, i32
		1, 1, 0, 0,
		// Params = 2, Results = 2, i32f32, i64f64
		2, 2, 0, 2, 1, 3,
		// Params = 0, Results = 4, f64f32i64i32
		0, 4, 3, 2, 1, 0,
	}, nil)
	g.typeSection()

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
	for i, tc := range []struct {
		ints  []int
		types []*wasm.FunctionType
		exp   []*wasm.Import
	}{
		{
			types: []*wasm.FunctionType{{}},
			ints: []int{
				2, // import section size
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
			types: []*wasm.FunctionType{{}, {}},
			ints: []int{
				2, // import section size
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
			types: []*wasm.FunctionType{{}, {}},
			ints: []int{
				2, // import section size
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
			types: []*wasm.FunctionType{{}, {}},
			ints: []int{
				2, // import section size
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
			ints: []int{
				2, // import section size
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
			types: []*wasm.FunctionType{{}, {}},
			ints: []int{
				3, // import section size
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
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			g := newGenerator(100, tc.ints, nil)
			g.m.TypeSection = tc.types

			g.importSection()
			require.Equal(t, len(tc.exp), len(g.m.ImportSection))

			for i := range tc.exp {
				fmt.Println(tc.exp[i].DescTable)
				fmt.Println(g.m.ImportSection[i].DescTable)
				require.Equal(t, tc.exp[i], g.m.ImportSection[i])
			}
		})
	}
}

func TestGenerator_functionSection(t *testing.T) {
	t.Run("with types", func(t *testing.T) {
		types := make([]*wasm.FunctionType, 10)
		g := newGenerator(100, []int{
			10, // number of functions.
			100001, 105, 102, 100000003, 104, 6, 9, 8, 7, 10000,
		}, nil)

		g.m.TypeSection = types
		g.functionSection()
		require.Equal(t, []uint32{1, 5, 2, 3, 4, 6, 9, 8, 7, 0}, g.m.FunctionSection)
	})
	t.Run("without types", func(t *testing.T) {
		g := newGenerator(100, []int{10}, nil)
		g.functionSection()
		require.Equal(t, []uint32(nil), g.m.FunctionSection)
	})
}

func TestGenerator_tableSection(t *testing.T) {
	t.Run("with table import", func(t *testing.T) {
		g := newGenerator(100, []int{10}, nil)
		g.m.ImportSection = append(g.m.ImportSection, &wasm.Import{Type: wasm.ExternTypeTable})

		g.tableSection()
		require.Nil(t, g.m.TableSection)
	})
	t.Run("without table import", func(t *testing.T) {
		g := newGenerator(100, []int{1, 100}, nil)
		g.tableSection()
		expMax := uint32(101)
		require.Equal(t, &wasm.Table{Min: 1, Max: &expMax}, g.m.TableSection)
	})
}

func TestGenerator_memorySection(t *testing.T) {
	t.Run("with memory import", func(t *testing.T) {
		g := newGenerator(100, []int{10}, nil)
		g.m.ImportSection = append(g.m.ImportSection, &wasm.Import{Type: wasm.ExternTypeMemory})

		g.memorySection()
		require.Nil(t, g.m.MemorySection)
	})
	t.Run("without memory import", func(t *testing.T) {
		g := newGenerator(100, []int{1, 100}, nil)
		g.memorySection()
		require.Equal(t, &wasm.Memory{Min: 1, Max: 101}, g.m.MemorySection)
	})
}

func TestGenerator_newConstExpr(t *testing.T) {
	for i, tc := range []struct {
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
	} {
		tc := tc
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
			3, // number of globals.
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

	g.globalSection()

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
