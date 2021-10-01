package wasm

import (
	"encoding/binary"
)

func memoryBase(vm *VirtualMachine) uint64 {
	vm.ActiveContext.PC++
	_ = vm.FetchUint32() // ignore align
	vm.ActiveContext.PC++
	return uint64(vm.FetchUint32()) + vm.OperandStack.Pop()
}

func i32Load(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint32(vm.Memory[base:])))
	vm.ActiveContext.PC++
}

func i64Load(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(binary.LittleEndian.Uint64(vm.Memory[base:]))
	vm.ActiveContext.PC++
}

func f32Load(vm *VirtualMachine) {
	i32Load(vm)
}

func f64Load(vm *VirtualMachine) {
	i64Load(vm)
}

func i32Load8s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(vm.Memory[base]))
	vm.ActiveContext.PC++
}

func i32Load8u(vm *VirtualMachine) {
	i32Load8s(vm)
}

func i32Load16s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint16(vm.Memory[base:])))
	vm.ActiveContext.PC++
}

func i32Load16u(vm *VirtualMachine) {
	i32Load16s(vm)
}

func i64Load8s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(vm.Memory[base]))
	vm.ActiveContext.PC++
}

func i64Load8u(vm *VirtualMachine) {
	i64Load8s(vm)
}

func i64Load16s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint16(vm.Memory[base:])))
	vm.ActiveContext.PC++
}

func i64Load16u(vm *VirtualMachine) {
	i64Load16s(vm)
}

func i64Load32s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint32(vm.Memory[base:])))
	vm.ActiveContext.PC++
}

func i64Load32u(vm *VirtualMachine) {
	i64Load32s(vm)
}

func i32Store(vm *VirtualMachine) {
	val := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.Memory[base:], uint32(val))
	vm.ActiveContext.PC++
}

func i64Store(vm *VirtualMachine) {
	val := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.Memory[base:], val)
	vm.ActiveContext.PC++
}

func f32Store(vm *VirtualMachine) {
	val := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.Memory[base:], uint32(val))
	vm.ActiveContext.PC++
}

func f64Store(vm *VirtualMachine) {
	v := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.Memory[base:], v)
	vm.ActiveContext.PC++
}

func i32Store8(vm *VirtualMachine) {
	v := byte(vm.OperandStack.Pop())
	base := memoryBase(vm)
	vm.Memory[base] = v
	vm.ActiveContext.PC++
}

func i32Store16(vm *VirtualMachine) {
	v := uint16(vm.OperandStack.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.Memory[base:], v)
	vm.ActiveContext.PC++
}

func i64Store8(vm *VirtualMachine) {
	v := byte(vm.OperandStack.Pop())
	base := memoryBase(vm)
	vm.Memory[base] = v
	vm.ActiveContext.PC++
}

func i64Store16(vm *VirtualMachine) {
	v := uint16(vm.OperandStack.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.Memory[base:], v)
	vm.ActiveContext.PC++
}

func i64Store32(vm *VirtualMachine) {
	v := uint32(vm.OperandStack.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.Memory[base:], v)
	vm.ActiveContext.PC++
}

func memorySize(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	vm.OperandStack.Push(uint64(int32(len(vm.Memory) / vmPageSize)))
	vm.ActiveContext.PC++
}

func memoryGrow(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	n := uint32(vm.OperandStack.Pop())

	if vm.InnerModule.SecMemory[0].Max != nil &&
		uint64(n+uint32(len(vm.Memory)/vmPageSize)) > uint64(*(vm.InnerModule.SecMemory[0].Max)) {
		v := int32(-1)
		vm.OperandStack.Push(uint64(v))
		return
	}

	vm.OperandStack.Push(uint64(len(vm.Memory)) / vmPageSize)
	vm.Memory = append(vm.Memory, make([]byte, n*vmPageSize)...)
	vm.ActiveContext.PC++
}
