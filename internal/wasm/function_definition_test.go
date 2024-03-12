package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/hammer"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestModule_BuildFunctionDefinitions(t *testing.T) {
	imp := &Import{
		Type:     ExternTypeFunc,
		DescFunc: 2, // Index of type.
	}

	nopCode := Code{Body: []byte{OpcodeEnd}}
	fn := func(uint32) uint32 { return 1 }
	tests := []struct {
		name            string
		m               *Module
		expected        []FunctionDefinition
		expectedImports []api.FunctionDefinition
		expectedExports map[string]api.FunctionDefinition
	}{
		{
			name:            "no exports",
			m:               &Module{},
			expected:        []FunctionDefinition{},
			expectedExports: map[string]api.FunctionDefinition{},
		},
		{
			name:     "no functions",
			expected: []FunctionDefinition{},
			m: &Module{
				ExportSection: []Export{{Type: ExternTypeGlobal, Index: 0}},
				GlobalSection: []Global{{}},
			},
			expectedExports: map[string]api.FunctionDefinition{},
		},
		{
			name: "only imported functions",
			expected: []FunctionDefinition{
				{name: "fn", Debugname: ".fn", importDesc: &Import{Module: "foo", Name: "bar", Type: ExternTypeFunc}, Functype: &FunctionType{}},
			},
			m: &Module{
				ExportSection:       []Export{{Type: ExternTypeGlobal, Index: 0}},
				GlobalSection:       []Global{{}},
				TypeSection:         []FunctionType{{}},
				ImportFunctionCount: 1,
				ImportSection:       []Import{{Type: ExternTypeFunc, Name: "bar", Module: "foo"}},
				NameSection:         &NameSection{FunctionNames: NameMap{{Index: Index(0), Name: "fn"}}},
			},
			expectedImports: []api.FunctionDefinition{
				&FunctionDefinition{
					name: "fn", Debugname: ".fn", importDesc: &Import{Module: "foo", Name: "bar", Type: ExternTypeFunc},
					Functype: &FunctionType{},
				},
			},
			expectedExports: map[string]api.FunctionDefinition{},
		},
		{
			name: "host func go",
			m: &Module{
				TypeSection:     []FunctionType{i32_i32},
				FunctionSection: []Index{0},
				CodeSection:     []Code{MustParseGoReflectFuncCode(fn)},
				NameSection: &NameSection{
					ModuleName:    "m",
					FunctionNames: NameMap{{Index: Index(0), Name: "fn"}},
					LocalNames:    IndirectNameMap{{Index: Index(0), NameMap: NameMap{{Index: Index(0), Name: "x"}}}},
					ResultNames:   IndirectNameMap{{Index: Index(0), NameMap: NameMap{{Index: Index(0), Name: "y"}}}},
				},
			},
			expected: []FunctionDefinition{
				{
					index:       0,
					name:        "fn",
					moduleName:  "m",
					Debugname:   "m.fn",
					goFunc:      MustParseGoReflectFuncCode(fn).GoFunc,
					Functype:    &i32_i32,
					paramNames:  []string{"x"},
					resultNames: []string{"y"},
				},
			},
			expectedExports: map[string]api.FunctionDefinition{},
		},
		{
			name: "without imports",
			m: &Module{
				ExportSection: []Export{
					{Name: "function_index=0", Type: ExternTypeFunc, Index: 0},
					{Name: "function_index=2", Type: ExternTypeFunc, Index: 2},
					{Name: "", Type: ExternTypeGlobal, Index: 0},
					{Name: "function_index=1", Type: ExternTypeFunc, Index: 1},
				},
				GlobalSection:   []Global{{}},
				FunctionSection: []Index{1, 2, 0},
				CodeSection: []Code{
					{Body: []byte{OpcodeEnd}},
					{Body: []byte{OpcodeEnd}},
					{Body: []byte{OpcodeEnd}},
				},
				TypeSection: []FunctionType{
					v_v,
					f64i32_v128i64,
					f64f32_i64,
				},
			},
			expected: []FunctionDefinition{
				{
					index:       0,
					Debugname:   ".$0",
					exportNames: []string{"function_index=0"},
					Functype:    &f64i32_v128i64,
				},
				{
					index:       1,
					Debugname:   ".$1",
					exportNames: []string{"function_index=1"},
					Functype:    &f64f32_i64,
				},
				{
					index:       2,
					Debugname:   ".$2",
					exportNames: []string{"function_index=2"},
					Functype:    &v_v,
				},
			},
			expectedExports: map[string]api.FunctionDefinition{
				"function_index=0": &FunctionDefinition{
					index:       0,
					Debugname:   ".$0",
					exportNames: []string{"function_index=0"},
					Functype:    &f64i32_v128i64,
				},
				"function_index=1": &FunctionDefinition{
					index:       1,
					exportNames: []string{"function_index=1"},
					Debugname:   ".$1",
					Functype:    &f64f32_i64,
				},
				"function_index=2": &FunctionDefinition{
					index:       2,
					Debugname:   ".$2",
					exportNames: []string{"function_index=2"},
					Functype:    &v_v,
				},
			},
		},
		{
			name: "with imports",
			m: &Module{
				ImportFunctionCount: 1,
				ImportSection:       []Import{*imp},
				ExportSection: []Export{
					{Name: "imported_function", Type: ExternTypeFunc, Index: 0},
					{Name: "function_index=1", Type: ExternTypeFunc, Index: 1},
					{Name: "function_index=2", Type: ExternTypeFunc, Index: 2},
				},
				FunctionSection: []Index{1, 0},
				CodeSection:     []Code{{Body: []byte{OpcodeEnd}}, {Body: []byte{OpcodeEnd}}},
				TypeSection: []FunctionType{
					v_v,
					f64i32_v128i64,
					f64f32_i64,
				},
			},
			expected: []FunctionDefinition{
				{
					index:       0,
					Debugname:   ".$0",
					importDesc:  imp,
					exportNames: []string{"imported_function"},
					Functype:    &f64f32_i64,
				},
				{
					index:       1,
					Debugname:   ".$1",
					exportNames: []string{"function_index=1"},
					Functype:    &f64i32_v128i64,
				},
				{
					index:       2,
					Debugname:   ".$2",
					exportNames: []string{"function_index=2"},
					Functype:    &v_v,
				},
			},
			expectedImports: []api.FunctionDefinition{
				&FunctionDefinition{
					index:       0,
					Debugname:   ".$0",
					importDesc:  imp,
					exportNames: []string{"imported_function"},
					Functype:    &f64f32_i64,
				},
			},
			expectedExports: map[string]api.FunctionDefinition{
				"imported_function": &FunctionDefinition{
					index:       0,
					Debugname:   ".$0",
					importDesc:  imp,
					exportNames: []string{"imported_function"},
					Functype:    &f64f32_i64,
				},
				"function_index=1": &FunctionDefinition{
					index:       1,
					Debugname:   ".$1",
					exportNames: []string{"function_index=1"},
					Functype:    &f64i32_v128i64,
				},
				"function_index=2": &FunctionDefinition{
					index:       2,
					Debugname:   ".$2",
					exportNames: []string{"function_index=2"},
					Functype:    &v_v,
				},
			},
		},
		{
			name: "with names",
			m: &Module{
				ImportFunctionCount: 1,
				TypeSection:         []FunctionType{v_v},
				ImportSection:       []Import{{Module: "i", Name: "f", Type: ExternTypeFunc}},
				NameSection: &NameSection{
					ModuleName: "module",
					FunctionNames: NameMap{
						{Index: Index(2), Name: "two"},
						{Index: Index(4), Name: "four"},
						{Index: Index(5), Name: "five"},
					},
				},
				FunctionSection: []Index{0, 0, 0, 0, 0},
				CodeSection:     []Code{nopCode, nopCode, nopCode, nopCode, nopCode},
			},
			expected: []FunctionDefinition{
				{moduleName: "module", index: 0, Debugname: "module.$0", importDesc: &Import{Module: "i", Name: "f"}, Functype: &v_v},
				{moduleName: "module", index: 1, Debugname: "module.$1", Functype: &v_v},
				{moduleName: "module", index: 2, Debugname: "module.two", Functype: &v_v, name: "two"},
				{moduleName: "module", index: 3, Debugname: "module.$3", Functype: &v_v},
				{moduleName: "module", index: 4, Debugname: "module.four", Functype: &v_v, name: "four"},
				{moduleName: "module", index: 5, Debugname: "module.five", Functype: &v_v, name: "five"},
			},
			expectedImports: []api.FunctionDefinition{
				&FunctionDefinition{moduleName: "module", index: 0, Debugname: "module.$0", importDesc: &Import{Module: "i", Name: "f"}, Functype: &v_v},
			},
			expectedExports: map[string]api.FunctionDefinition{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.m.buildFunctionDefinitions()
			require.Equal(t, tc.expected, tc.m.FunctionDefinitionSection)
			require.Equal(t, tc.expectedImports, tc.m.ImportedFunctions())
			require.Equal(t, tc.expectedExports, tc.m.ExportedFunctions())
		})
	}

	// Execute the same tests with n=`concurrentCount` goroutines invoking `buildFunctionDefinitions()` at once.
	const nGoroutines = 100
	const nIterations = 10
	for _, tc := range tests {
		tc := tc
		testName := tc.name + " (concurrent)"
		t.Run(testName, func(t *testing.T) {
			hammer.NewHammer(t, nGoroutines, nIterations).
				Run(func(p, n int) {
					tc.m.buildFunctionDefinitions()
				}, nil)

			require.Equal(t, tc.expected, tc.m.FunctionDefinitionSection)
			require.Equal(t, tc.expectedImports, tc.m.ImportedFunctions())
			require.Equal(t, tc.expectedExports, tc.m.ExportedFunctions())
		})
	}
}
