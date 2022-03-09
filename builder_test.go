package wazero

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasm"
)

// TestNewModuleBuilder_Build only covers a few scenarios to avoid duplicating tests in internal/wasm/host_test.go
func TestNewModuleBuilder_Build(t *testing.T) {
	i32, i64 := wasm.ValueTypeI32, wasm.ValueTypeI64

	uint32_uint32 := func(uint32) uint32 {
		return 0
	}
	fnUint32_uint32 := reflect.ValueOf(uint32_uint32)
	uint64_uint32 := func(uint64) uint32 {
		return 0
	}
	fnUint64_uint32 := reflect.ValueOf(uint64_uint32)

	tests := []struct {
		name     string
		input    func(Runtime) ModuleBuilder
		expected *internalwasm.Module
	}{
		{
			name: "empty",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("")
			},
			expected: &internalwasm.Module{},
		},
		{
			name: "only name",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("env")
			},
			expected: &internalwasm.Module{NameSection: &internalwasm.NameSection{ModuleName: "env"}},
		},
		{
			name: "ExportFunction",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportFunction("1", uint32_uint32)
			},
			expected: &internalwasm.Module{
				TypeSection: []*internalwasm.FunctionType{
					{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection:     []internalwasm.Index{0},
				HostFunctionSection: []*reflect.Value{&fnUint32_uint32},
				ExportSection: map[string]*internalwasm.Export{
					"1": {Name: "1", Type: internalwasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &internalwasm.NameSection{
					FunctionNames: internalwasm.NameMap{{Index: 0, Name: "1"}},
				},
			},
		},
		{
			name: "ExportFunction overwrites existing",
			input: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder("").ExportFunction("1", uint32_uint32).ExportFunction("1", uint64_uint32)
			},
			expected: &internalwasm.Module{
				TypeSection: []*internalwasm.FunctionType{
					{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection:     []internalwasm.Index{0},
				HostFunctionSection: []*reflect.Value{&fnUint64_uint32},
				ExportSection: map[string]*internalwasm.Export{
					"1": {Name: "1", Type: internalwasm.ExternTypeFunc, Index: 0},
				},
				NameSection: &internalwasm.NameSection{
					FunctionNames: internalwasm.NameMap{{Index: 0, Name: "1"}},
				},
			},
		},
		{
			name: "ExportFunction twice",
			input: func(r Runtime) ModuleBuilder {
				// Intentionally out of order
				return r.NewModuleBuilder("").ExportFunction("2", uint64_uint32).ExportFunction("1", uint32_uint32)
			},
			expected: &internalwasm.Module{
				TypeSection: []*internalwasm.FunctionType{
					{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection:     []internalwasm.Index{0, 1},
				HostFunctionSection: []*reflect.Value{&fnUint32_uint32, &fnUint64_uint32},
				ExportSection: map[string]*internalwasm.Export{
					"1": {Name: "1", Type: internalwasm.ExternTypeFunc, Index: 0},
					"2": {Name: "2", Type: internalwasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &internalwasm.NameSection{
					FunctionNames: internalwasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}},
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
			expected: &internalwasm.Module{
				TypeSection: []*internalwasm.FunctionType{
					{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection:     []internalwasm.Index{0, 1},
				HostFunctionSection: []*reflect.Value{&fnUint32_uint32, &fnUint64_uint32},
				ExportSection: map[string]*internalwasm.Export{
					"1": {Name: "1", Type: internalwasm.ExternTypeFunc, Index: 0},
					"2": {Name: "2", Type: internalwasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &internalwasm.NameSection{
					FunctionNames: internalwasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}},
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
			expected: &internalwasm.Module{
				TypeSection: []*internalwasm.FunctionType{
					{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i64}, Results: []wasm.ValueType{i32}},
				},
				FunctionSection:     []internalwasm.Index{0, 1},
				HostFunctionSection: []*reflect.Value{&fnUint32_uint32, &fnUint64_uint32},
				ExportSection: map[string]*internalwasm.Export{
					"1": {Name: "1", Type: internalwasm.ExternTypeFunc, Index: 0},
					"2": {Name: "2", Type: internalwasm.ExternTypeFunc, Index: 1},
				},
				NameSection: &internalwasm.NameSection{
					FunctionNames: internalwasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := tc.input(NewRuntime()).Build()
			require.NoError(t, e)
			requireHostModuleEquals(t, tc.expected, m.module)
		})
	}
}

// TestNewModuleBuilder_InstantiateModule ensures Runtime.InstantiateModule is called on success.
func TestNewModuleBuilder_InstantiateModule(t *testing.T) {
	r := NewRuntime()
	m, err := r.NewModuleBuilder("env").Instantiate()
	require.NoError(t, err)

	// If this was instantiated, it would be added to the store under the same name
	require.Equal(t, r.(*runtime).store.Module("env"), m)
}

// TestNewModuleBuilder_InstantiateModule_Errors ensures errors propagate from Runtime.InstantiateModule
func TestNewModuleBuilder_InstantiateModule_Errors(t *testing.T) {
	r := NewRuntime()
	_, err := r.NewModuleBuilder("env").Instantiate()
	require.NoError(t, err)

	_, err = r.NewModuleBuilder("env").Instantiate()
	require.EqualError(t, err, "module env has already been instantiated")
}

// requireHostModuleEquals is redefined from internal/wasm/host_test.go to avoid an import cycle extracting it.
func requireHostModuleEquals(t *testing.T, expected, actual *internalwasm.Module) {
	// `require.Equal(t, expected, actual)` fails reflect pointers don't match, so brute compare:
	require.Equal(t, expected.TypeSection, actual.TypeSection)
	require.Equal(t, expected.ImportSection, actual.ImportSection)
	require.Equal(t, expected.FunctionSection, actual.FunctionSection)
	require.Equal(t, expected.TableSection, actual.TableSection)
	require.Equal(t, expected.MemorySection, actual.MemorySection)
	require.Equal(t, expected.GlobalSection, actual.GlobalSection)
	require.Equal(t, expected.ExportSection, actual.ExportSection)
	require.Equal(t, expected.StartSection, actual.StartSection)
	require.Equal(t, expected.ElementSection, actual.ElementSection)
	require.Nil(t, actual.CodeSection) // Host functions are implemented in Go, not Wasm!
	require.Equal(t, expected.DataSection, actual.DataSection)
	require.Equal(t, expected.NameSection, actual.NameSection)

	// Special case because reflect.Value can't be compared with Equals
	require.Equal(t, len(expected.HostFunctionSection), len(actual.HostFunctionSection))
	for i := range expected.HostFunctionSection {
		require.Equal(t, (*expected.HostFunctionSection[i]).Type(), (*actual.HostFunctionSection[i]).Type())
	}
}
