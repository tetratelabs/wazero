package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getLocal(t *testing.T) {
	exp := uint64(100)
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeLocalGet), 0x05},
		},
		Locals: []uint64{0, 0, 0, 0, 0, exp},
	}

	vm := &VirtualMachine{ActiveContext: ctx, OperandStack: NewVirtualMachineOperandStack()}
	getLocal(vm)
	assert.Equal(t, exp, vm.OperandStack.Pop())
	assert.Equal(t, -1, vm.OperandStack.SP)
}

func Test_setLocal(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeLocalSet), 0x05},
		},
		Locals: make([]uint64, 100),
	}

	exp := uint64(100)
	st := NewVirtualMachineOperandStack()
	st.Push(exp)

	vm := &VirtualMachine{ActiveContext: ctx, OperandStack: st}
	setLocal(vm)
	assert.Equal(t, exp, vm.ActiveContext.Locals[5])
	assert.Equal(t, -1, vm.OperandStack.SP)
}

func Test_teeLocal(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeLocalTee), 0x05},
		},
		Locals: make([]uint64, 100),
	}

	exp := uint64(100)
	st := NewVirtualMachineOperandStack()
	st.Push(exp)

	vm := &VirtualMachine{ActiveContext: ctx, OperandStack: st}
	teeLocal(vm)
	assert.Equal(t, exp, vm.ActiveContext.Locals[5])
	assert.Equal(t, exp, vm.OperandStack.Pop())
}
