package wasm

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_i32Const(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeI32Const), 0x05},
		},
	}

	vm := &VirtualMachine{
		ActiveContext: ctx,
		OperandStack:  NewVirtualMachineOperandStack(),
	}
	i32Const(vm)
	assert.Equal(t, uint32(0x05), uint32(vm.OperandStack.Pop()))
	assert.Equal(t, -1, vm.OperandStack.SP)
}

func Test_i64Const(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeI64Const), 0x05},
		},
	}

	vm := &VirtualMachine{
		ActiveContext: ctx,
		OperandStack:  NewVirtualMachineOperandStack(),
	}
	i64Const(vm)
	assert.Equal(t, uint32(0x05), uint32(vm.OperandStack.Pop()))
	assert.Equal(t, -1, vm.OperandStack.SP)
}

func Test_f32Const(t *testing.T) {

	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeF32Const), 0x00, 0x00, 0x80, 0x3f},
		},
	}

	vm := &VirtualMachine{
		ActiveContext: ctx,
		OperandStack:  NewVirtualMachineOperandStack(),
	}
	f32Const(vm)
	assert.Equal(t, float32(1.0), math.Float32frombits(uint32(vm.OperandStack.Pop())))
	assert.Equal(t, -1, vm.OperandStack.SP)
}

func Test_f64Const(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeF64Const), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f},
		},
	}

	vm := &VirtualMachine{
		ActiveContext: ctx,
		OperandStack:  NewVirtualMachineOperandStack(),
	}
	f64Const(vm)
	assert.Equal(t, 1.0, math.Float64frombits(vm.OperandStack.Pop()))
	assert.Equal(t, -1, vm.OperandStack.SP)
}
