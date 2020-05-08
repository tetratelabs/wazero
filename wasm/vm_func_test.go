package wasm

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostFunction_Call(t *testing.T) {
	var cnt int64
	f := func(in int64) (int32, int64, float32, float64) {
		cnt += in
		return 1, 2, 3, 4
	}
	hf := &HostFunction{
		function: reflect.ValueOf(f),
		Signature: &FunctionType{
			InputTypes:  []ValueType{ValueTypeI64},
			ReturnTypes: []ValueType{ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64},
		},
	}

	vm := &VirtualMachine{OperandStack: NewVirtualMachineOperandStack()}
	vm.OperandStack.Push(10)

	hf.Call(vm)
	assert.Equal(t, 3, vm.OperandStack.SP)
	assert.Equal(t, int64(10), cnt)

	// f64
	assert.Equal(t, 4.0, math.Float64frombits(vm.OperandStack.Pop()))
	assert.Equal(t, float32(3.0), float32(math.Float64frombits(vm.OperandStack.Pop())))
	assert.Equal(t, int64(2), int64(vm.OperandStack.Pop()))
	assert.Equal(t, int32(1), int32(vm.OperandStack.Pop()))
}

func TestNativeFunction_Call(t *testing.T) {
	n := &NativeFunction{
		Signature: &FunctionType{},
		Body: []byte{
			byte(OptCodeI64Const), 0x05, byte(OptCodeReturn),
		},
	}
	vm := &VirtualMachine{
		OperandStack: NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{
			PC: 1000,
		},
	}
	n.Call(vm)
	assert.Equal(t, uint64(0x05), vm.OperandStack.Pop())
	assert.Equal(t, uint64(1000), vm.ActiveContext.PC)
}

func TestVirtualMachine_execNativeFunction(t *testing.T) {
	n := &NativeFunction{
		Signature: &FunctionType{},
		Body: []byte{
			byte(OptCodeI64Const), 0x05,
			byte(OptCodeI64Const), 0x01,
			byte(OptCodeReturn),
		},
	}
	vm := &VirtualMachine{
		OperandStack: NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{
			Function: n,
		},
	}

	vm.execNativeFunction()
	assert.Equal(t, uint64(4), vm.ActiveContext.PC)
	assert.Equal(t, uint64(0x01), vm.OperandStack.Pop())
	assert.Equal(t, uint64(0x05), vm.OperandStack.Pop())
}
