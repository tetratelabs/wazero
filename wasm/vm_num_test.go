package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_i32eqz(t *testing.T) {
	vm := &VirtualMachine{
		OperandStack:  NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{},
	}
	var testTable = []struct {
		input int
		want  uint64
	}{
		{input: 0, want: 1},
		{input: 1, want: 0},
	}
	for _, tt := range testTable {
		vm.OperandStack.Push(uint64(tt.input))
		i32eqz(vm)
		require.Equal(t, tt.want, vm.OperandStack.Pop())
	}
}

func Test_i32ne(t *testing.T) {
	vm := &VirtualMachine{
		OperandStack:  NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{},
	}
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{3, 4}, want: 1},
		{input: [2]int{3, 3}, want: 0},
	}
	for _, tt := range testTable {
		vm.OperandStack.Push(uint64(tt.input[0]))
		vm.OperandStack.Push(uint64(tt.input[1]))
		i32ne(vm)
		require.Equal(t, tt.want, vm.OperandStack.Pop())
	}
}

func Test_i32lts(t *testing.T) {
	vm := &VirtualMachine{
		OperandStack:  NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{},
	}
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{-4, 1}, want: 1},
		{input: [2]int{4, -1}, want: 0},
	}
	for _, tt := range testTable {
		vm.OperandStack.Push(uint64(tt.input[0]))
		vm.OperandStack.Push(uint64(tt.input[1]))
		i32lts(vm)
		require.Equal(t, tt.want, vm.OperandStack.Pop())
	}
}

func Test_i32ltu(t *testing.T) {
	vm := &VirtualMachine{
		OperandStack:  NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{},
	}
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{1, 4}, want: 1},
		{input: [2]int{4, 1}, want: 0},
	}
	for _, tt := range testTable {
		vm.OperandStack.Push(uint64(tt.input[0]))
		vm.OperandStack.Push(uint64(tt.input[1]))
		i32ltu(vm)
		require.Equal(t, tt.want, vm.OperandStack.Pop())
	}
}

func Test_i32gts(t *testing.T) {
	vm := &VirtualMachine{
		OperandStack:  NewVirtualMachineOperandStack(),
		ActiveContext: &NativeFunctionContext{},
	}
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{1, -4}, want: 1},
		{input: [2]int{-4, 1}, want: 0},
	}
	for _, tt := range testTable {
		vm.OperandStack.Push(uint64(tt.input[0]))
		vm.OperandStack.Push(uint64(tt.input[1]))
		i32gts(vm)
		require.Equal(t, tt.want, vm.OperandStack.Pop())
	}
}
