package wasm

func getLocal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	vm.OperandStack.Push(vm.ActiveContext.Locals[id])
	vm.ActiveContext.PC++
}

func setLocal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	v := vm.OperandStack.Pop()
	vm.ActiveContext.Locals[id] = v
	vm.ActiveContext.PC++
}

func teeLocal(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	id := vm.FetchUint32()
	v := vm.OperandStack.Peek()
	vm.ActiveContext.Locals[id] = v
	vm.ActiveContext.PC++
}
