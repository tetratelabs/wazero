package wasm

import "fmt"

func call(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	addr := vm.ActiveContext.Function.ModuleInstance.FunctionAddrs[index]
	vm.Store.Functions[addr].Call(vm)
	vm.ActiveContext.PC++
}

func callIndirect(vm *VirtualMachine) {
	currentModuleInst := vm.ActiveContext.Function.ModuleInstance

	vm.ActiveContext.PC++
	typeIndex := vm.FetchUint32()
	expType := currentModuleInst.Types[typeIndex]

	// note: mvp limits the size of table index space to 1
	const tableIndex = 0
	vm.ActiveContext.PC++ // skip 0x00 (table index)

	tableAddr := currentModuleInst.TableAddrs[tableIndex]
	tableInst := vm.Store.Tables[tableAddr]
	index := vm.OperandStack.Pop()
	if index >= uint64(len(tableInst.Table)) {
		panic("table index out of range")
	}

	functinAddr := tableInst.Table[index]
	if functinAddr == nil {
		panic("table entry not initialized")
	}

	f := vm.Store.Functions[*functinAddr]
	ft := f.FunctionType()
	if !hasSameSignature(ft.InputTypes, expType.InputTypes) ||
		!hasSameSignature(ft.ReturnTypes, expType.ReturnTypes) {
		panic(fmt.Sprintf("function signature mismatch (%#x, %#x) != (%#x, %#x)",
			ft.InputTypes, ft.ReturnTypes, expType.InputTypes, expType.ReturnTypes))
	}
	f.Call(vm)

	vm.ActiveContext.PC++
}
