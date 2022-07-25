package wazero

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestNewModuleBuilder_Compile only covers a few scenarios to avoid duplicating tests in internal/wasm/host_test.go
func TestNewModuleBuilder_Compile(t *testing.T) {
	i32, i64 := api.ValueTypeI32, api.ValueTypeI64

	uint32_uint32 := func(uint32) uint32 {
		return 0
	}
	uint64_uint32 := func(uint64) uint32 {
		return 0
	}

	tests := []struct {
		name     string
		input    func(Runtime) ModuleBuilder
		expected *wasm.Module
	}{
		{
			name: "empty",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("")
			},
			expected: &wasm.Module{},
		},
		{
			name: "only name",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("env")
			},
			expected: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "env"}},
		},
		{
			name: "ExportFunction",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportFunction("1", uint32_uint32)
			},
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []api.ValueType{i32}, Results: []api.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{wasm.MustParseGoFuncCode(uint32_uint32)},
				ExportSection: []*wasm.Export{
					{Name: "1", Type: wasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "1"}},
				},
			},
		},
		{
			name: "ExportFunction with names",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportFunction("1", uint32_uint32, "get", "x")
			},
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []api.ValueType{i32}, Results: []api.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{wasm.MustParseGoFuncCode(uint32_uint32)},
				ExportSection: []*wasm.Export{
					{Name: "1", Type: wasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "get"}},
					LocalNames:    []*wasm.NameMapAssoc{{Index: 0, NameMap: wasm.NameMap{{Index: 0, Name: "x"}}}},
				},
			},
		},
		{
			name: "ExportFunction overwrites existing",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportFunction("1", uint32_uint32).ExportFunction("1", uint64_uint32)
			},
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []api.ValueType{i64}, Results: []api.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{wasm.MustParseGoFuncCode(uint64_uint32)},
				ExportSection: []*wasm.Export{
					{Name: "1", Type: wasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "1"}},
				},
			},
		},
		{
			name: "ExportFunction twice",
			input: func(r Runtime) ModuleBuilder {
				// Intentionally out of order
				return r.NewModuleBuilder("").ExportFunction("2", uint64_uint32).ExportFunction("1", uint32_uint32)
			},
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []api.ValueType{i32}, Results: []api.ValueType{i32}},
					{Params: []api.ValueType{i64}, Results: []api.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0, 1},
				CodeSection:     []*wasm.Code{wasm.MustParseGoFuncCode(uint32_uint32), wasm.MustParseGoFuncCode(uint64_uint32)},
				ExportSection: []*wasm.Export{
					{Name: "1", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "2", Type: wasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}},
				},
			},
		},
		{
			name: "ExportFunctions",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportFunctions(map[string]interface{}{
					"1": uint32_uint32,
					"2": uint64_uint32,
				})
			},
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []api.ValueType{i32}, Results: []api.ValueType{i32}},
					{Params: []api.ValueType{i64}, Results: []api.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0, 1},
				CodeSection:     []*wasm.Code{wasm.MustParseGoFuncCode(uint32_uint32), wasm.MustParseGoFuncCode(uint64_uint32)},
				ExportSection: []*wasm.Export{
					{Name: "1", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "2", Type: wasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}},
				},
			},
		},
		{
			name: "ExportFunctions overwrites",
			input: func(r Runtime) ModuleBuilder {
				b := r.NewModuleBuilder("").ExportFunction("1", uint64_uint32)
				return b.ExportFunctions(map[string]interface{}{
					"1": uint32_uint32,
					"2": uint64_uint32,
				})
			},
			expected: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []api.ValueType{i32}, Results: []api.ValueType{i32}},
					{Params: []api.ValueType{i64}, Results: []api.ValueType{i32}},
				},
				FunctionSection: []wasm.Index{0, 1},
				CodeSection:     []*wasm.Code{wasm.MustParseGoFuncCode(uint32_uint32), wasm.MustParseGoFuncCode(uint64_uint32)},
				ExportSection: []*wasm.Export{
					{Name: "1", Type: wasm.ExternTypeFunc, Index: 0},
					{Name: "2", Type: wasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &wasm.NameSection{
					FunctionNames: wasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}},
				},
			},
		},
		{
			name: "ExportMemory",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportMemory("memory", 1)
			},
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: wasm.MemoryLimitPages},
				ExportSection: []*wasm.Export{
					{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "ExportMemory overwrites",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportMemory("memory", 1).ExportMemory("memory", 2)
			},
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 2, Cap: 2, Max: wasm.MemoryLimitPages},
				ExportSection: []*wasm.Export{
					{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "ExportMemoryWithMax",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportMemoryWithMax("memory", 1, 1)
			},
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: 1, IsMaxEncoded: true},
				ExportSection: []*wasm.Export{
					{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "ExportMemoryWithMax overwrites",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportMemoryWithMax("memory", 1, 1).ExportMemoryWithMax("memory", 1, 2)
			},
			expected: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: 2, IsMaxEncoded: true},
				ExportSection: []*wasm.Export{
					{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalI32",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalI32("canvas_width", 1024)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI32},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(1024)},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "canvas_width", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalI32 overwrites",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalI32("canvas_width", 1024).ExportGlobalI32("canvas_width", math.MaxInt32)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI32},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeUint32(math.MaxInt32)},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "canvas_width", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalI64",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalI64("start_epoch", 1620216263544)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI64},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeUint64(1620216263544)},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "start_epoch", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalI64 overwrites",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalI64("start_epoch", 1620216263544).ExportGlobalI64("start_epoch", math.MaxInt64)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI64},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(math.MaxInt64)},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "start_epoch", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalF32",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalF32("math/pi", 3.1415926536)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeF32},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeF32Const, Data: u64.LeBytes(api.EncodeF32(3.1415926536))},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "math/pi", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalF32 overwrites",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalF32("math/pi", 3.1415926536).ExportGlobalF32("math/pi", math.MaxFloat32)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeF32},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeF32Const, Data: u64.LeBytes(api.EncodeF32(math.MaxFloat32))},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "math/pi", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalF64",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalF64("math/pi", math.Pi)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeF64},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeF64Const, Data: u64.LeBytes(api.EncodeF64(math.Pi))},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "math/pi", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
		{
			name: "ExportGlobalF64 overwrites",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportGlobalF64("math/pi", math.Pi).ExportGlobalF64("math/pi", math.MaxFloat64)
			},
			expected: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeF64},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeF64Const, Data: u64.LeBytes(api.EncodeF64(math.MaxFloat64))},
					},
				},
				ExportSection: []*wasm.Export{
					{Name: "math/pi", Type: wasm.ExternTypeGlobal, Index: 0},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			b := tc.input(NewRuntime()).(*moduleBuilder)
			compiled, err := b.Compile(testCtx, NewCompileConfig())
			require.NoError(t, err)
			m := compiled.(*compiledModule)

			requireHostModuleEquals(t, tc.expected, m.module)

			require.Equal(t, b.r.store.Engine, m.compiledEngine)

			// Built module must be instantiable by Engine.
			mod, err := b.r.InstantiateModule(testCtx, m, NewModuleConfig())
			require.NoError(t, err)

			// Closing the module shouldn't remove the compiler cache
			require.NoError(t, mod.Close(testCtx))
			require.Equal(t, uint32(1), b.r.store.Engine.CompiledModuleCount())
		})
	}
}

// TestNewModuleBuilder_Compile_Errors only covers a few scenarios to avoid duplicating tests in internal/wasm/host_test.go
func TestNewModuleBuilder_Compile_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       func(Runtime) ModuleBuilder
		config      CompileConfig
		expectedErr string
	}{
		{
			name: "memory min > limit", // only one test to avoid duplicating tests in module_test.go
			input: func(rt Runtime) ModuleBuilder {
				return rt.NewModuleBuilder("").ExportMemory("memory", math.MaxUint32)
			},
			config:      NewCompileConfig(),
			expectedErr: "memory[memory] min 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name: "memory cap < min", // only one test to avoid duplicating tests in module_test.go
			input: func(rt Runtime) ModuleBuilder {
				return rt.NewModuleBuilder("").ExportMemory("memory", 2)
			},
			config: NewCompileConfig().WithMemorySizer(func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
				return 2, 1, 2
			}),
			expectedErr: "memory[memory] capacity 1 pages (64 Ki) less than minimum 2 pages (128 Ki)",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := tc.input(NewRuntime()).Compile(testCtx, tc.config)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}

// TestNewModuleBuilder_Instantiate ensures Runtime.InstantiateModule is called on success.
func TestNewModuleBuilder_Instantiate(t *testing.T) {
	r := NewRuntime()
	m, err := r.NewModuleBuilder("env").Instantiate(testCtx, r)
	require.NoError(t, err)

	// If this was instantiated, it would be added to the store under the same name
	require.Equal(t, r.(*runtime).ns.Module("env"), m)

	// Closing the module should remove the compiler cache
	require.NoError(t, m.Close(testCtx))
	require.Zero(t, r.(*runtime).store.Engine.CompiledModuleCount())
}

// TestNewModuleBuilder_Instantiate_Errors ensures errors propagate from Runtime.InstantiateModule
func TestNewModuleBuilder_Instantiate_Errors(t *testing.T) {
	r := NewRuntime()
	_, err := r.NewModuleBuilder("env").Instantiate(testCtx, r)
	require.NoError(t, err)

	_, err = r.NewModuleBuilder("env").Instantiate(testCtx, r)
	require.EqualError(t, err, "module[env] has already been instantiated")
}

// requireHostModuleEquals is redefined from internal/wasm/host_test.go to avoid an import cycle extracting it.
func requireHostModuleEquals(t *testing.T, expected, actual *wasm.Module) {
	// `require.Equal(t, expected, actual)` fails reflect pointers don't match, so brute compare:
	for _, tp := range expected.TypeSection {
		tp.CacheNumInUint64()
	}
	require.Equal(t, expected.TypeSection, actual.TypeSection)
	require.Equal(t, expected.ImportSection, actual.ImportSection)
	require.Equal(t, expected.FunctionSection, actual.FunctionSection)
	require.Equal(t, expected.TableSection, actual.TableSection)
	require.Equal(t, expected.MemorySection, actual.MemorySection)
	require.Equal(t, expected.GlobalSection, actual.GlobalSection)
	require.Equal(t, expected.ExportSection, actual.ExportSection)
	require.Equal(t, expected.StartSection, actual.StartSection)
	require.Equal(t, expected.ElementSection, actual.ElementSection)
	require.Equal(t, expected.DataSection, actual.DataSection)
	require.Equal(t, expected.NameSection, actual.NameSection)

	// Special case because reflect.Value can't be compared with Equals
	// TODO: This is copy/paste with /internal/wasm/host_test.go
	require.Equal(t, len(expected.CodeSection), len(actual.CodeSection))
	for i, c := range expected.CodeSection {
		actualCode := actual.CodeSection[i]
		require.True(t, actualCode.IsHostFunction)
		require.Equal(t, c.Kind, actualCode.Kind)
		require.Equal(t, c.GoFunc.Type(), actualCode.GoFunc.Type())

		// Not wasm
		require.Nil(t, actualCode.Body)
		require.Nil(t, actualCode.LocalTypes)
	}
}
