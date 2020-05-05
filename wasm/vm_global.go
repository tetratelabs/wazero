package wasm

func getGlobal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	vm.OperandStack.Push(vm.Globals[id])
}

func setGlobal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	vm.Globals[id] = vm.OperandStack.Pop()
}
