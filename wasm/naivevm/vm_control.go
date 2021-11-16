package naivevm

import (
	"bytes"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

func block(vm *naiveVirtualMachine) {
	frame := vm.activeFrame
	block, ok := frame.f.Blocks[frame.pc]
	if !ok {
		panic("block not initialized")
	}

	frame.pc += block.BlockTypeBytes
	vm.activeFrame.labels.push(&label{
		arity:          len(block.BlockType.ReturnTypes),
		continuationPC: block.EndAt + 1,
		operandSP:      vm.operands.sp,
	})
	vm.activeFrame.pc++
}

func loop(vm *naiveVirtualMachine) {
	frame := vm.activeFrame
	block, ok := frame.f.Blocks[frame.pc]
	if !ok {
		panic("block not found")
	}
	frame.pc += block.BlockTypeBytes
	arity := len(block.BlockType.InputTypes)
	vm.activeFrame.labels.push(&label{
		arity:          arity,
		continuationPC: block.StartAt,
		operandSP:      vm.operands.sp - arity,
	})
	vm.activeFrame.pc++
}

func ifOp(vm *naiveVirtualMachine) {
	frame := vm.activeFrame
	block, ok := frame.f.Blocks[frame.pc]
	if !ok {
		panic("block not initialized")
	}
	frame.pc += block.BlockTypeBytes

	if vm.operands.pop() == 0 {
		frame.pc = block.ElseAt
	}

	arity := len(block.BlockType.ReturnTypes)
	vm.activeFrame.labels.push(&label{
		arity:          arity,
		continuationPC: block.EndAt + 1,
		operandSP:      vm.operands.sp - len(block.BlockType.InputTypes),
	})
	vm.activeFrame.pc++
}

func elseOp(vm *naiveVirtualMachine) {
	l := vm.activeFrame.labels.pop()
	vm.activeFrame.pc = l.continuationPC
}

func end(vm *naiveVirtualMachine) {
	_ = vm.activeFrame.labels.pop()
	vm.activeFrame.pc++
}

func returnOp(vm *naiveVirtualMachine) {
	vm.popFrame()
}

func br(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	index := vm.FetchUint32()
	brAt(vm, index)
}

func brIf(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	index := vm.FetchUint32()
	c := vm.operands.pop()
	if c != 0 {
		brAt(vm, index)
	} else {
		vm.activeFrame.pc++
	}
}

func brAt(vm *naiveVirtualMachine, index uint32) {
	var l *label
	for i := uint32(0); i < index+1; i++ {
		l = vm.activeFrame.labels.pop()
	}

	// TODO: can be optimized.
	values := make([]uint64, 0, l.arity)
	for i := 0; i < l.arity; i++ {
		values = append(values, vm.operands.pop())
	}
	vm.operands.sp = l.operandSP
	for _, v := range values {
		vm.operands.push(v)
	}
	vm.activeFrame.pc = l.continuationPC
}

func brTable(vm *naiveVirtualMachine) {
	vm.activeFrame.pc++
	r := bytes.NewBuffer(vm.activeFrame.f.Body[vm.activeFrame.pc:])
	nl, num, err := leb128.DecodeUint32(r)
	if err != nil {
		panic(err)
	}

	lis := make([]uint32, nl)
	for i := range lis {
		li, n, err := leb128.DecodeUint32(r)
		if err != nil {
			panic(err)
		}
		num += n
		lis[i] = li
	}

	ln, n, err := leb128.DecodeUint32(r)
	if err != nil {
		panic(err)
	}
	vm.activeFrame.pc += n + num

	i := vm.operands.pop()
	if uint32(i) < nl {
		brAt(vm, lis[i])
	} else {
		brAt(vm, ln)
	}
}
