package wasm

func getGlobal(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	index := vm.FetchUint32()
	addr := vm.ActiveFrame.F.ModuleInstance.GlobalsAddrs[index]
	vm.Operands.Push(vm.Store.Globals[addr].Val)
	vm.ActiveFrame.PC++
}

func setGlobal(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	index := vm.FetchUint32()
	addr := vm.ActiveFrame.F.ModuleInstance.GlobalsAddrs[index]
	vm.Store.Globals[addr].Val = vm.Operands.Pop()
	vm.ActiveFrame.PC++
}
