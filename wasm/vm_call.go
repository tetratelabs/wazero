package wasm

func call(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	vm.Functions[index].Call(vm)
}

func callIndirect(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	expType := vm.InnerModule.SecTypes[index]

	tableIndex := vm.OperandStack.Pop()
	// note: mvp limits the size of table index space to 1
	if tableIndex >= uint64(len(vm.InnerModule.IndexSpace.Table[0])) {
		panic("table index out of range")
	}

	te := vm.InnerModule.IndexSpace.Table[0][tableIndex]
	if te == nil {
		panic("table entry not initialized")
	}

	f := vm.Functions[*te]
	ft := f.FunctionType()
	if !hasSameSignature(ft.InputTypes, expType.InputTypes) ||
		!hasSameSignature(ft.ReturnTypes, expType.ReturnTypes) {
		panic("function signature mismatch")
	}
	f.Call(vm)

	vm.ActiveContext.PC++ // skip 0x00
}
