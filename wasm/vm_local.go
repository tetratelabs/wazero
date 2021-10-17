package wasm

func getLocal(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	id := vm.FetchUint32()
	vm.Operands.Push(vm.ActiveFrame.Locals[id])
	vm.ActiveFrame.PC++
}

func setLocal(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	id := vm.FetchUint32()
	v := vm.Operands.Pop()
	vm.ActiveFrame.Locals[id] = v
	vm.ActiveFrame.PC++
}

func teeLocal(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	id := vm.FetchUint32()
	v := vm.Operands.Peek()
	vm.ActiveFrame.Locals[id] = v
	vm.ActiveFrame.PC++
}
