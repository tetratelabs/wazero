package wasm

import (
	"math"
)

func i32Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(vm.FetchInt32()))
}

func i64Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(vm.FetchInt64()))
}

func f32Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(math.Float32bits(vm.FetchFloat32())))
}

func f64Const(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(math.Float64bits(vm.FetchFloat64()))
}
