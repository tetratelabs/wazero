package wasm

import (
	"math"
)

func i32Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(vm.FetchInt32()))
	vm.ActiveContext.PC++
}

func i64Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(vm.FetchInt64()))
	vm.ActiveContext.PC++
}

func f32Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(math.Float32bits(vm.FetchFloat32())))
	vm.ActiveContext.PC++
}

func f64Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(math.Float64bits(vm.FetchFloat64()))
	vm.ActiveContext.PC++
}
