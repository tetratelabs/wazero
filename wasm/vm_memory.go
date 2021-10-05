package wasm

import (
	"encoding/binary"
	"math"
)

func memoryBase(vm *VirtualMachine) uint64 {
	vm.ActiveContext.PC++
	_ = vm.FetchUint32() // ignore align
	vm.ActiveContext.PC++
	return uint64(vm.FetchUint32()) + vm.OperandStack.Pop()
}

func (vm *VirtualMachine) CurrentMemory() []byte {
	memoryAddr := vm.ActiveContext.Function.ModuleInstance.MemoryAddrs[0]
	return vm.Store.Memories[memoryAddr].Memory
}

func i32Load(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint32(vm.CurrentMemory()[base:])))
	vm.ActiveContext.PC++
}

func i64Load(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(binary.LittleEndian.Uint64(vm.CurrentMemory()[base:]))
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
	vm.OperandStack.Push(uint64(int8(vm.CurrentMemory()[base])))
	vm.ActiveContext.PC++
}

func i32Load8u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(uint8(vm.CurrentMemory()[base])))
	vm.ActiveContext.PC++
}

func i32Load16s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(int16(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:]))))
	vm.ActiveContext.PC++
}

func i32Load16u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:])))
	vm.ActiveContext.PC++
}

func i64Load8s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(int8(vm.CurrentMemory()[base])))
	vm.ActiveContext.PC++
}

func i64Load8u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(uint8(vm.CurrentMemory()[base])))
	vm.ActiveContext.PC++
}

func i64Load16s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(int16(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:]))))
	vm.ActiveContext.PC++
}

func i64Load16u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:])))
	vm.ActiveContext.PC++
}

func i64Load32s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(int32(binary.LittleEndian.Uint32(vm.CurrentMemory()[base:]))))
	vm.ActiveContext.PC++
}

func i64Load32u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.OperandStack.Push(uint64(binary.LittleEndian.Uint32(vm.CurrentMemory()[base:])))
	vm.ActiveContext.PC++
}

func i32Store(vm *VirtualMachine) {
	val := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[base:], uint32(val))
	vm.ActiveContext.PC++
}

func i64Store(vm *VirtualMachine) {
	val := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.CurrentMemory()[base:], val)
	vm.ActiveContext.PC++
}

func f32Store(vm *VirtualMachine) {
	val := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[base:], uint32(val))
	vm.ActiveContext.PC++
}

func f64Store(vm *VirtualMachine) {
	v := vm.OperandStack.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.CurrentMemory()[base:], v)
	vm.ActiveContext.PC++
}

func i32Store8(vm *VirtualMachine) {
	v := byte(vm.OperandStack.Pop())
	base := memoryBase(vm)
	vm.CurrentMemory()[base] = v
	vm.ActiveContext.PC++
}

func i32Store16(vm *VirtualMachine) {
	v := uint16(vm.OperandStack.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.CurrentMemory()[base:], v)
	vm.ActiveContext.PC++
}

func i64Store8(vm *VirtualMachine) {
	v := byte(vm.OperandStack.Pop())
	base := memoryBase(vm)
	vm.CurrentMemory()[base] = v
	vm.ActiveContext.PC++
}

func i64Store16(vm *VirtualMachine) {
	v := uint16(vm.OperandStack.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.CurrentMemory()[base:], v)
	vm.ActiveContext.PC++
}

func i64Store32(vm *VirtualMachine) {
	v := uint32(vm.OperandStack.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[base:], v)
	vm.ActiveContext.PC++
}

func memorySize(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(int32(uint64(len(vm.CurrentMemory())) / PageSize)))
	vm.ActiveContext.PC += 2
}

func memoryGrow(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	n := vm.OperandStack.Pop()

	memoryAddr := vm.ActiveContext.Function.ModuleInstance.MemoryAddrs[0]
	memoryInst := vm.Store.Memories[memoryAddr]

	max := uint64(math.MaxUint32)
	if memoryInst.Max != nil {
		max = uint64(*memoryInst.Max) * PageSize
	}
	if uint64(n*PageSize+uint64(len(memoryInst.Memory))) > max {
		v := int32(-1)
		vm.OperandStack.Push(uint64(v))
		vm.ActiveContext.PC++
		return
	}

	vm.OperandStack.Push(uint64(len(memoryInst.Memory)) / PageSize)
	memoryInst.Memory = append(memoryInst.Memory, make([]byte, n*PageSize)...)
	vm.ActiveContext.PC++
}
