package wasm

func getGlobal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	vm.OperandStack.Push(vm.Globals[id])
	vm.ActiveContext.PC++
}

func setGlobal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	vm.Globals[id] = vm.OperandStack.Pop()
	vm.ActiveContext.PC++
}
