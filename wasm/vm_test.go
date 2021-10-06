package wasm

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVirtualMachine_ExecExportedFunction(t *testing.T) {
	vm := &VirtualMachine{
		InnerModule: &Module{
			ExportSection: map[string]*ExportSegment{
				"a": {Desc: &ExportDesc{Index: 0, Kind: ExportKindFunction}},
				"b": {Desc: &ExportDesc{Index: 0, Kind: ExportKindGlobal}},
				"c": {Desc: &ExportDesc{Index: 100, Kind: ExportKindFunction}},
			},
		},
		Functions: []VirtualMachineFunction{&HostFunction{
			function: reflect.ValueOf(func(in int64) int64 {
				return in * 2
			}),
			Signature: &FunctionType{
				InputTypes:  []ValueType{ValueTypeI64},
				ReturnTypes: []ValueType{ValueTypeI64},
			},
		}},
		OperandStack: NewVirtualMachineOperandStack(),
	}

	ret, retTypes, err := vm.ExecExportedFunction("a", 1)
	require.NoError(t, err)
	require.Len(t, retTypes, 1)
	require.Len(t, ret, 1)
	require.Equal(t, retTypes[0], ValueTypeI64)
	require.Equal(t, int64(2), int64(ret[0]))

	_, _, err = vm.ExecExportedFunction("a")
	require.Error(t, err)
	_, _, err = vm.ExecExportedFunction("b")
	require.Error(t, err)
	_, _, err = vm.ExecExportedFunction("c")
	require.Error(t, err)
}

func TestVirtualMachine_FetchInt32(t *testing.T) {
	vm := &VirtualMachine{
		ActiveContext: &NativeFunctionContext{
			PC:       1,
			Function: &NativeFunction{Body: []byte{0x00, 0xFF, 0x00}},
		},
	}
	actual := vm.FetchInt32()
	require.Equal(t, int32(127), actual)
	require.Equal(t, uint64(2), vm.ActiveContext.PC)
}

func TestVirtualMachine_FetchInt64(t *testing.T) {
	vm := &VirtualMachine{
		ActiveContext: &NativeFunctionContext{
			PC:       1,
			Function: &NativeFunction{Body: []byte{0x00, 0xFF, 0x00}},
		},
	}
	actual := vm.FetchInt64()
	require.Equal(t, int64(127), actual)
	require.Equal(t, uint64(2), vm.ActiveContext.PC)
}

func TestVirtualMachine_FetchFloat32(t *testing.T) {
	vm := &VirtualMachine{
		ActiveContext: &NativeFunctionContext{
			PC:       2,
			Function: &NativeFunction{Body: []byte{0x00, 0x00, 0x40, 0xe1, 0x47, 0x40}},
		},
	}
	actual := vm.FetchFloat32()
	require.Equal(t, float32(3.1231232), actual)
	require.Equal(t, uint64(5), vm.ActiveContext.PC)
}

func TestVirtualMachine_FetchFloat64(t *testing.T) {
	vm := &VirtualMachine{
		ActiveContext: &NativeFunctionContext{
			Function: &NativeFunction{Body: []byte{
				0x5e, 0xc4, 0xd8, 0xf9, 0x27, 0xfc, 0x08, 0x40,
			}},
		},
	}
	actual := vm.FetchFloat64()
	require.Equal(t, 3.1231231231, actual)
	require.Equal(t, uint64(7), vm.ActiveContext.PC)
}
