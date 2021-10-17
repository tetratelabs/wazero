package wasm

import (
	"math"
)

func i32Const(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	vm.Operands.Push(uint64(vm.FetchInt32()))
	vm.ActiveFrame.PC++
}

func i64Const(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	vm.Operands.Push(uint64(vm.FetchInt64()))
	vm.ActiveFrame.PC++
}

func f32Const(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	vm.Operands.Push(uint64(math.Float32bits(vm.FetchFloat32())))
	vm.ActiveFrame.PC++
}

func f64Const(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	vm.Operands.Push(math.Float64bits(vm.FetchFloat64()))
	vm.ActiveFrame.PC++
}
