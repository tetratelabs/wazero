package naivevm

import (
	"encoding/binary"
	"math"

	"github.com/mathetake/gasm/wasm"
)

func memoryBase(vm *naiveVirtualMachine) uint64 {
	vm.activeFrame.pc++
	_ = vm.FetchUint32() // ignore align
	vm.activeFrame.pc++
	return uint64(vm.FetchUint32()) + vm.operands.pop()
}

func (vm *naiveVirtualMachine) currentMemory() []byte {
	return vm.activeFrame.f.ModuleInstance.Memories[0].Memory
}

func i32Load(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(binary.LittleEndian.Uint32(vm.currentMemory()[base:])))
	vm.activeFrame.pc++
}

func i64Load(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(binary.LittleEndian.Uint64(vm.currentMemory()[base:]))
	vm.activeFrame.pc++
}

func f32Load(vm *naiveVirtualMachine) {
	i32Load(vm)
}

func f64Load(vm *naiveVirtualMachine) {
	i64Load(vm)
}

func i32Load8s(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(int8(vm.currentMemory()[base])))
	vm.activeFrame.pc++
}

func i32Load8u(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(uint8(vm.currentMemory()[base])))
	vm.activeFrame.pc++
}

func i32Load16s(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(int16(binary.LittleEndian.Uint16(vm.currentMemory()[base:]))))
	vm.activeFrame.pc++
}

func i32Load16u(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(binary.LittleEndian.Uint16(vm.currentMemory()[base:])))
	vm.activeFrame.pc++
}

func i64Load8s(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(int8(vm.currentMemory()[base])))
	vm.activeFrame.pc++
}

func i64Load8u(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(uint8(vm.currentMemory()[base])))
	vm.activeFrame.pc++
}

func i64Load16s(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(int16(binary.LittleEndian.Uint16(vm.currentMemory()[base:]))))
	vm.activeFrame.pc++
}

func i64Load16u(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(binary.LittleEndian.Uint16(vm.currentMemory()[base:])))
	vm.activeFrame.pc++
}

func i64Load32s(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(int32(binary.LittleEndian.Uint32(vm.currentMemory()[base:]))))
	vm.activeFrame.pc++
}

func i64Load32u(vm *naiveVirtualMachine) {
	base := memoryBase(vm)
	vm.operands.push(uint64(binary.LittleEndian.Uint32(vm.currentMemory()[base:])))
	vm.activeFrame.pc++
}

func i32Store(vm *naiveVirtualMachine) {
	val := vm.operands.pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.currentMemory()[base:], uint32(val))
	vm.activeFrame.pc++
}

func i64Store(vm *naiveVirtualMachine) {
	val := vm.operands.pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.currentMemory()[base:], val)
	vm.activeFrame.pc++
}

func f32Store(vm *naiveVirtualMachine) {
	val := vm.operands.pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.currentMemory()[base:], uint32(val))
	vm.activeFrame.pc++
}

func f64Store(vm *naiveVirtualMachine) {
	v := vm.operands.pop()
	base := memoryBase(vm)
	binary.LittleEndian.PutUint64(vm.currentMemory()[base:], v)
	vm.activeFrame.pc++
}

func i32Store8(vm *naiveVirtualMachine) {
	v := byte(vm.operands.pop())
	base := memoryBase(vm)
	vm.currentMemory()[base] = v
	vm.activeFrame.pc++
}

func i32Store16(vm *naiveVirtualMachine) {
	v := uint16(vm.operands.pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.currentMemory()[base:], v)
	vm.activeFrame.pc++
}

func i64Store8(vm *naiveVirtualMachine) {
	v := byte(vm.operands.pop())
	base := memoryBase(vm)
	vm.currentMemory()[base] = v
	vm.activeFrame.pc++
}

func i64Store16(vm *naiveVirtualMachine) {
	v := uint16(vm.operands.pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint16(vm.currentMemory()[base:], v)
	vm.activeFrame.pc++
}

func i64Store32(vm *naiveVirtualMachine) {
	v := uint32(vm.operands.pop())
	base := memoryBase(vm)
	binary.LittleEndian.PutUint32(vm.currentMemory()[base:], v)
	vm.activeFrame.pc++
}

func memorySize(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(int32(uint64(len(vm.currentMemory())) / wasm.PageSize)))
	vm.activeFrame.pc += 2
}

func memoryGrow(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	n := vm.operands.pop()

	memoryInst := vm.activeFrame.f.ModuleInstance.Memories[0]

	max := uint64(math.MaxUint32)
	if memoryInst.Max != nil {
		max = uint64(*memoryInst.Max) * wasm.PageSize
	}
	if uint64(n*wasm.PageSize+uint64(len(memoryInst.Memory))) > max {
		v := int32(-1)
		vm.operands.push(uint64(v))
		vm.activeFrame.pc++
		return
	}

	vm.operands.push(uint64(len(memoryInst.Memory)) / wasm.PageSize)
	memoryInst.Memory = append(memoryInst.Memory, make([]byte, n*wasm.PageSize)...)
	vm.activeFrame.pc++ // Skip reserved bytes.
}
