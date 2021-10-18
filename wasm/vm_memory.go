package wasm

import (
	"encoding/binary"
	"math"
)

func memoryBase(vm *VirtualMachine) uint64 {
	vm.ActiveFrame.PC++
	_ = vm.FetchUint32() // ignore align
	vm.ActiveFrame.PC++
	return uint64(vm.FetchUint32()) + vm.Operands.Pop()
}

func (vm *VirtualMachine) CurrentMemory() []byte {
	memoryAddr := vm.ActiveFrame.F.ModuleInstance.MemoryAddrs[0]
	return vm.Store.Memories[memoryAddr].Memory
}

func i32Load(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(binary.LittleEndian.Uint32(vm.CurrentMemory()[base:])))
	vm.ActiveFrame.PC++
}

func i64Load(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(binary.LittleEndian.Uint64(vm.CurrentMemory()[base:]))
	vm.ActiveFrame.PC++
}

func f32Load(vm *VirtualMachine) {
	i32Load(vm)
}

func f64Load(vm *VirtualMachine) {
	i64Load(vm)
}

func i32Load8s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(int8(vm.CurrentMemory()[base])))
	vm.ActiveFrame.PC++
}

func i32Load8u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(uint8(vm.CurrentMemory()[base])))
	vm.ActiveFrame.PC++
}

func i32Load16s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(int16(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:]))))
	vm.ActiveFrame.PC++
}

func i32Load16u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:])))
	vm.ActiveFrame.PC++
}

func i64Load8s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(int8(vm.CurrentMemory()[base])))
	vm.ActiveFrame.PC++
}

func i64Load8u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(uint8(vm.CurrentMemory()[base])))
	vm.ActiveFrame.PC++
}

func i64Load16s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(int16(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:]))))
	vm.ActiveFrame.PC++
}

func i64Load16u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(binary.LittleEndian.Uint16(vm.CurrentMemory()[base:])))
	vm.ActiveFrame.PC++
}

func i64Load32s(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(int32(binary.LittleEndian.Uint32(vm.CurrentMemory()[base:]))))
	vm.ActiveFrame.PC++
}

func i64Load32u(vm *VirtualMachine) {
	base := memoryBase(vm)
	vm.Operands.Push(uint64(binary.LittleEndian.Uint32(vm.CurrentMemory()[base:])))
	vm.ActiveFrame.PC++
}

func i32Store(vm *VirtualMachine) {
	val := vm.Operands.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[base:], uint32(val))
	vm.ActiveFrame.PC++
}

func i64Store(vm *VirtualMachine) {
	val := vm.Operands.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.CurrentMemory()[base:], val)
	vm.ActiveFrame.PC++
}

func f32Store(vm *VirtualMachine) {
	val := vm.Operands.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[base:], uint32(val))
	vm.ActiveFrame.PC++
}

func f64Store(vm *VirtualMachine) {
	v := vm.Operands.Pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.CurrentMemory()[base:], v)
	vm.ActiveFrame.PC++
}

func i32Store8(vm *VirtualMachine) {
	v := byte(vm.Operands.Pop())
	base := memoryBase(vm)
	vm.CurrentMemory()[base] = v
	vm.ActiveFrame.PC++
}

func i32Store16(vm *VirtualMachine) {
	v := uint16(vm.Operands.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.CurrentMemory()[base:], v)
	vm.ActiveFrame.PC++
}

func i64Store8(vm *VirtualMachine) {
	v := byte(vm.Operands.Pop())
	base := memoryBase(vm)
	vm.CurrentMemory()[base] = v
	vm.ActiveFrame.PC++
}

func i64Store16(vm *VirtualMachine) {
	v := uint16(vm.Operands.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.CurrentMemory()[base:], v)
	vm.ActiveFrame.PC++
}

func i64Store32(vm *VirtualMachine) {
	v := uint32(vm.Operands.Pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[base:], v)
	vm.ActiveFrame.PC++
}

func memorySize(vm *VirtualMachine) {
	vm.Operands.Push(uint64(int32(uint64(len(vm.CurrentMemory())) / pageSize)))
	vm.ActiveFrame.PC += 2
}

func memoryGrow(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	n := vm.Operands.Pop()

	memoryAddr := vm.ActiveFrame.F.ModuleInstance.MemoryAddrs[0]
	memoryInst := vm.Store.Memories[memoryAddr]

	max := uint64(math.MaxUint32)
	if memoryInst.Max != nil {
		max = uint64(*memoryInst.Max) * pageSize
	}
	if uint64(n*pageSize+uint64(len(memoryInst.Memory))) > max {
		v := int32(-1)
		vm.Operands.Push(uint64(v))
		vm.ActiveFrame.PC++
		return
	}

	vm.Operands.Push(uint64(len(memoryInst.Memory)) / pageSize)
	memoryInst.Memory = append(memoryInst.Memory, make([]byte, n*pageSize)...)
	vm.ActiveFrame.PC++ // Skip reserved bytes.
}
