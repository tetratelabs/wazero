package naivevm

import (
	"math"
)

func i32Const(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	vm.operands.push(uint64(vm.FetchInt32()))
	vm.activeFrame.pc++
}

func i64Const(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	vm.operands.push(uint64(vm.FetchInt64()))
	vm.activeFrame.pc++
}

func f32Const(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	vm.operands.push(uint64(math.Float32bits(vm.FetchFloat32())))
	vm.activeFrame.pc++
}

func f64Const(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	vm.operands.push(math.Float64bits(vm.FetchFloat64()))
	vm.activeFrame.pc++
}
