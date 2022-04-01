package internalwasm

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/api"
)

// wasiAPI simulates the real WASI api
type wasiAPI struct {
}

func (a *wasiAPI) ArgsSizesGet(ctx api.Module, resultArgc, resultArgvBufSize uint32) api.Errno {
	return 0
}

func (a *wasiAPI) FdWrite(ctx api.Module, fd, iovs, iovsCount, resultSize uint32) api.Errno {
	return 0
}

func TestNewHostModule(t *testing.T) {
	i32 := ValueTypeI32

	a := wasiAPI{}
	functionArgsSizesGet := "args_sizes_get"
	fnArgsSizesGet := reflect.ValueOf(a.ArgsSizesGet)
	functionFdWrite := "fd_write"
	fnFdWrite := reflect.ValueOf(a.FdWrite)

	tests := []struct {
		name, moduleName string
		goFuncs          map[string]interface{}
		expected         *Module
	}{
		{
			name:     "empty",
			expected: &Module{},
		},
		{
			name:       "only name",
			moduleName: "test",
			expected:   &Module{NameSection: &NameSection{ModuleName: "test"}},
		},
		{
			name:       "two struct funcs",
			moduleName: "wasi_snapshot_preview1",
			goFuncs: map[string]interface{}{
				functionArgsSizesGet: a.ArgsSizesGet,
				functionFdWrite:      a.FdWrite,
			},
			expected: &Module{
				TypeSection: []*FunctionType{
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
					{Params: []ValueType{i32, i32, i32, i32}, Results: []ValueType{i32}},
				},
				FunctionSection:     []Index{0, 1},
				HostFunctionSection: []*reflect.Value{&fnArgsSizesGet, &fnFdWrite},
				ExportSection: map[string]*Export{
					"args_sizes_get": {Name: "args_sizes_get", Type: ExternTypeFunc, Index: 0},
					"fd_write":       {Name: "fd_write", Type: ExternTypeFunc, Index: 1},
				},
				NameSection: &NameSection{
					ModuleName: "wasi_snapshot_preview1",
					FunctionNames: NameMap{
						{Index: 0, Name: "args_sizes_get"},
						{Index: 1, Name: "fd_write"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := NewHostModule(tc.moduleName, tc.goFuncs)
			require.NoError(t, e)
			requireHostModuleEquals(t, tc.expected, m)
		})
	}
}

func requireHostModuleEquals(t *testing.T, expected, actual *Module) {
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

func TestNewHostModule_Errors(t *testing.T) {
	t.Run("Adds export name to error message", func(t *testing.T) {
		_, err := NewHostModule("test", map[string]interface{}{"fn": "hello"})
		require.EqualError(t, err, "func[fn] kind != func: string")
	})
}

func TestModule_validateHostFunctions(t *testing.T) {
	notFn := reflect.ValueOf(t)
	fn := reflect.ValueOf(func(api.Module) {})

	t.Run("ok", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			FunctionSection:     []uint32{0},
			HostFunctionSection: []*reflect.Value{&fn},
		}
		err := m.validateHostFunctions()
		require.NoError(t, err)
	})
	t.Run("function, but no host function", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			FunctionSection:     []Index{0},
			HostFunctionSection: nil,
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, "host function count (0) != function count (1)")
	})
	t.Run("function out of range of host functions", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			FunctionSection:     []Index{1},
			HostFunctionSection: []*reflect.Value{&fn},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, "host_function[0] type section index out of range: 1")
	})
	t.Run("mismatch params", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{Params: []ValueType{ValueTypeF32}}},
			FunctionSection:     []Index{0},
			HostFunctionSection: []*reflect.Value{&fn},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, "host_function[0] signature doesn't match type section: v_v != f32_v")
	})
	t.Run("mismatch results", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{Results: []ValueType{ValueTypeF32}}},
			FunctionSection:     []Index{0},
			HostFunctionSection: []*reflect.Value{&fn},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, "host_function[0] signature doesn't match type section: v_v != v_f32")
	})
	t.Run("not a function", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			FunctionSection:     []Index{0},
			HostFunctionSection: []*reflect.Value{&notFn},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, "host_function[0] is not a valid go func: kind != func: ptr")
	})
	t.Run("not a function - exported", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			FunctionSection:     []Index{0},
			HostFunctionSection: []*reflect.Value{&notFn},
			ExportSection:       map[string]*Export{"f1": {Name: "f1", Type: ExternTypeFunc, Index: 0}},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, `host_function[0] export["f1"] is not a valid go func: kind != func: ptr`)
	})
	t.Run("not a function  - exported after import", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			ImportSection:       []*Import{{Type: ExternTypeFunc}},
			FunctionSection:     []Index{1},
			HostFunctionSection: []*reflect.Value{&notFn},
			ExportSection:       map[string]*Export{"f1": {Name: "f1", Type: ExternTypeFunc, Index: 1}},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, `host_function[0] export["f1"] is not a valid go func: kind != func: ptr`)
	})
	t.Run("not a function - exported twice", func(t *testing.T) {
		m := Module{
			TypeSection:         []*FunctionType{{}},
			FunctionSection:     []Index{0},
			HostFunctionSection: []*reflect.Value{&notFn},
			ExportSection: map[string]*Export{
				"f1": {Name: "f1", Type: ExternTypeFunc, Index: 0},
				"f2": {Name: "f2", Type: ExternTypeFunc, Index: 0},
			},
		}
		err := m.validateHostFunctions()
		require.Error(t, err)
		require.EqualError(t, err, `host_function[0] export["f1","f2"] is not a valid go func: kind != func: ptr`)
	})
}
