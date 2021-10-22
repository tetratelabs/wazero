package naivevm

func getGlobal(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	index := vm.FetchUint32()
	g := vm.activeFrame.f.ModuleInstance.Globals[index]
	vm.operands.push(g.Val)
	vm.activeFrame.pc++
}

func setGlobal(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	index := vm.FetchUint32()
	g := vm.activeFrame.f.ModuleInstance.Globals[index]
	g.Val = vm.operands.pop()
	vm.activeFrame.pc++
}
