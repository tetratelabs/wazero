package wasm

import (
	"reflect"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// wasiAPI simulates the real WASI api
type wasiAPI struct {
}

func ArgsSizesGet(ctx api.Module, resultArgc, resultArgvBufSize uint32) uint32 {
	return 0
}

func (a *wasiAPI) ArgsSizesGet(ctx api.Module, resultArgc, resultArgvBufSize uint32) uint32 {
	return 0
}

func (a *wasiAPI) FdWrite(ctx api.Module, fd, iovs, iovsCount, resultSize uint32) uint32 {
	return 0
}

func swap(x, y uint32) (uint32, uint32) {
	return y, x
}

func TestNewHostModule(t *testing.T) {
	i32 := ValueTypeI32

	a := wasiAPI{}
	functionArgsSizesGet := "args_sizes_get"
	fnArgsSizesGet := reflect.ValueOf(a.ArgsSizesGet)
	functionFdWrite := "fd_write"
	fnFdWrite := reflect.ValueOf(a.FdWrite)
	functionSwap := "swap"
	fnSwap := reflect.ValueOf(swap)

	tests := []struct {
		name, moduleName string
		nameToGoFunc     map[string]interface{}
		nameToMemory     map[string]*Memory
		nameToGlobal     map[string]*Global
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
			name:       "funcs",
			moduleName: "wasi_snapshot_preview1",
			nameToGoFunc: map[string]interface{}{
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
				ExportSection: []*Export{
					{Name: "args_sizes_get", Type: ExternTypeFunc, Index: 0},
					{Name: "fd_write", Type: ExternTypeFunc, Index: 1},
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
		{
			name:       "multi-value",
			moduleName: "swapper",
			nameToGoFunc: map[string]interface{}{
				functionSwap: swap,
			},
			expected: &Module{
				TypeSection:         []*FunctionType{{Params: []ValueType{i32, i32}, Results: []ValueType{i32, i32}}},
				FunctionSection:     []Index{0},
				HostFunctionSection: []*reflect.Value{&fnSwap},
				ExportSection:       []*Export{{Name: "swap", Type: ExternTypeFunc, Index: 0}},
				NameSection:         &NameSection{ModuleName: "swapper", FunctionNames: NameMap{{Index: 0, Name: "swap"}}},
			},
		},
		{
			name:         "memory",
			nameToMemory: map[string]*Memory{"memory": {Min: 1, Max: 2}},
			expected: &Module{
				MemorySection: &Memory{Min: 1, Max: 2},
				ExportSection: []*Export{{Name: "memory", Type: ExternTypeMemory, Index: 0}},
			},
		},
		{
			name: "globals",
			nameToGlobal: map[string]*Global{
				"g2": {
					Type: &GlobalType{ValType: i32},
					Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)},
				},
				"g1": {
					Type: &GlobalType{ValType: i32},
					Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
				},
			},
			expected: &Module{
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: i32},
						Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
					},
					{
						Type: &GlobalType{ValType: i32},
						Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: leb128.EncodeInt32(2)},
					},
				},
				ExportSection: []*Export{
					{Name: "g1", Type: ExternTypeGlobal, Index: 0},
					{Name: "g2", Type: ExternTypeGlobal, Index: 1},
				},
			},
		},
		{
			name:       "one of each",
			moduleName: "env",
			nameToGoFunc: map[string]interface{}{
				functionArgsSizesGet: a.ArgsSizesGet,
			},
			nameToMemory: map[string]*Memory{
				"memory": {Min: 1, Max: 1},
			},
			nameToGlobal: map[string]*Global{
				"g": {
					Type: &GlobalType{ValType: i32},
					Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
				},
			},
			expected: &Module{
				TypeSection: []*FunctionType{
					{Params: []ValueType{i32, i32}, Results: []ValueType{i32}},
				},
				FunctionSection:     []Index{0},
				HostFunctionSection: []*reflect.Value{&fnArgsSizesGet},
				GlobalSection: []*Global{
					{
						Type: &GlobalType{ValType: i32},
						Init: &ConstantExpression{Opcode: OpcodeI32Const, Data: const1},
					},
				},
				MemorySection: &Memory{Min: 1, Max: 1},
				ExportSection: []*Export{
					{Name: "args_sizes_get", Type: ExternTypeFunc, Index: 0},
					{Name: "memory", Type: ExternTypeMemory, Index: 0},
					{Name: "g", Type: ExternTypeGlobal, Index: 0},
				},
				NameSection: &NameSection{
					ModuleName: "env",
					FunctionNames: NameMap{
						{Index: 0, Name: "args_sizes_get"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := NewHostModule(
				tc.moduleName,
				tc.nameToGoFunc,
				tc.nameToMemory,
				tc.nameToGlobal,
				Features20191205|FeatureMultiValue,
			)
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
	tests := []struct {
		name, moduleName string
		nameToGoFunc     map[string]interface{}
		nameToMemory     map[string]*Memory
		nameToGlobal     map[string]*Global
		expectedErr      string
	}{
		{
			name:         "not a function",
			nameToGoFunc: map[string]interface{}{"fn": t},
			expectedErr:  "func[fn] kind != func: ptr",
		},
		{
			name:         "function has multiple results",
			nameToGoFunc: map[string]interface{}{"fn": func() (uint32, uint32) { return 0, 0 }},
			nameToMemory: map[string]*Memory{"mem": {Min: 1, Max: 1}},
			expectedErr:  "func[fn] multiple result types invalid as feature \"multi-value\" is disabled",
		},
		{
			name:         "func collides on memory name",
			nameToGoFunc: map[string]interface{}{"fn": ArgsSizesGet},
			nameToMemory: map[string]*Memory{"fn": {Min: 1, Max: 1}},
			expectedErr:  "func[fn] exports the same name as a memory",
		},
		{
			name:         "multiple memories",
			nameToMemory: map[string]*Memory{"memory": {Min: 1, Max: 1}, "mem": {Min: 2, Max: 2}},
			expectedErr:  "only one memory is allowed, but configured: mem, memory",
		},
		{
			name:         "memory max < min",
			nameToMemory: map[string]*Memory{"memory": {Min: 1, Max: 0}},
			expectedErr:  "memory[memory] min 1 pages (64 Ki) > max 0 pages (0 Ki)",
		},
		{
			name:         "func collides on global name",
			nameToGoFunc: map[string]interface{}{"fn": ArgsSizesGet},
			nameToGlobal: map[string]*Global{"fn": {}},
			expectedErr:  "func[fn] exports the same name as a global",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := NewHostModule(tc.moduleName, tc.nameToGoFunc, tc.nameToMemory, tc.nameToGlobal, Features20191205)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
