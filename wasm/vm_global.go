package wasm

func getGlobal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	addr := vm.ActiveContext.Function.ModuleInstance.GlobalsAddrs[index]
	vm.OperandStack.Push(vm.Store.Globals[addr].Val)
	vm.ActiveContext.PC++
}

func setGlobal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	addr := vm.ActiveContext.Function.ModuleInstance.GlobalsAddrs[index]
	// TODO: Check mutatability.
	vm.Store.Globals[addr].Val = vm.OperandStack.Pop()
	vm.ActiveContext.PC++
}
