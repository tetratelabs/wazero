package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func buildvm() *VirtualMachine {
	return &VirtualMachine{
		OperandStack: NewVirtualMachineOperandStack(),
	}
}

func Test_i32eqz(t *testing.T) {
	vm := buildvm()
	vm.OperandStack.Push(uint64(0))
	i32eqz(vm)
	assert.Equal(t, uint32(1), uint32(vm.OperandStack.Pop()))
}

func Test_i32eq(t *testing.T) {
	vm := buildvm()
	vm.OperandStack.Push(uint64(3))
	vm.OperandStack.Push(uint64(3))
	i32eq(vm)
	assert.Equal(t, uint32(1), uint32(vm.OperandStack.Pop()))
}

func Test_i32ne(t *testing.T) {
	vm := buildvm()
	vm.OperandStack.Push(uint64(3))
	vm.OperandStack.Push(uint64(4))
	i32ne(vm)
	assert.Equal(t, uint32(1), uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(3))
	vm.OperandStack.Push(uint64(3))
	i32ne(vm)
	assert.Equal(t, uint32(0), uint32(vm.OperandStack.Pop()))
}
