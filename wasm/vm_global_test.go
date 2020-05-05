package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getGlobal(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeGlobalGet), 0x05},
		},
	}

	exp := uint64(1)
	globals := []uint64{0, 0, 0, 0, 0, exp}

	vm := &VirtualMachine{
		ActiveContext: ctx,
		OperandStack:  NewVirtualMachineOperandStack(),
		Globals:       globals,
	}
	getGlobal(vm)
	assert.Equal(t, exp, vm.OperandStack.Pop())
	assert.Equal(t, -1, vm.OperandStack.SP)
}

func Test_setGlobal(t *testing.T) {
	ctx := &NativeFunctionContext{
		Function: &NativeFunction{
			Body: []byte{byte(OptCodeGlobalSet), 0x05},
		},
	}

	exp := uint64(100)
	st := NewVirtualMachineOperandStack()
	st.Push(exp)

	vm := &VirtualMachine{ActiveContext: ctx, OperandStack: st, Globals: []uint64{0, 0, 0, 0, 0, 0}}
	setGlobal(vm)
	assert.Equal(t, exp, vm.Globals[5])
	assert.Equal(t, -1, vm.OperandStack.SP)
}
