package naivevm

func getLocal(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	id := vm.FetchUint32()
	vm.operands.push(vm.activeFrame.locals[id])
	vm.activeFrame.pc++
}

func setLocal(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	id := vm.FetchUint32()
	v := vm.operands.pop()
	vm.activeFrame.locals[id] = v
	vm.activeFrame.pc++
}

func teeLocal(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	id := vm.FetchUint32()
	v := vm.operands.peek()
	vm.activeFrame.locals[id] = v
	vm.activeFrame.pc++
}
