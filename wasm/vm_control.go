package wasm

import (
	"bytes"

	"github.com/mathetake/gasm/wasm/leb128"
)

func block(vm *VirtualMachine) {
	frame := vm.ActiveFrame
	block, ok := frame.F.Blocks[frame.PC]
	if !ok {
		panic("block not initialized")
	}

	frame.PC += block.BlockTypeBytes
	vm.ActiveFrame.Labels.Push(&Label{
		Arity:          len(block.BlockType.ReturnTypes),
		ContinuationPC: block.EndAt + 1,
		OperandSP:      vm.Operands.SP,
	})
	vm.ActiveFrame.PC++
}

func loop(vm *VirtualMachine) {
	frame := vm.ActiveFrame
	block, ok := frame.F.Blocks[frame.PC]
	if !ok {
		panic("block not found")
	}
	frame.PC += block.BlockTypeBytes
	arity := len(block.BlockType.InputTypes)
	vm.ActiveFrame.Labels.Push(&Label{
		Arity:          arity,
		ContinuationPC: block.StartAt,
		OperandSP:      vm.Operands.SP - arity,
	})
	vm.ActiveFrame.PC++
}

func ifOp(vm *VirtualMachine) {
	frame := vm.ActiveFrame
	block, ok := frame.F.Blocks[frame.PC]
	if !ok {
		panic("block not initialized")
	}
	frame.PC += block.BlockTypeBytes

	if vm.Operands.Pop() == 0 {
		frame.PC = block.ElseAt
	}

	arity := len(block.BlockType.ReturnTypes)
	vm.ActiveFrame.Labels.Push(&Label{
		Arity:          arity,
		ContinuationPC: block.EndAt + 1,
		OperandSP:      vm.Operands.SP - len(block.BlockType.InputTypes),
	})
	vm.ActiveFrame.PC++
}

func elseOp(vm *VirtualMachine) {
	l := vm.ActiveFrame.Labels.Pop()
	vm.ActiveFrame.PC = l.ContinuationPC
}

func end(vm *VirtualMachine) {
	_ = vm.ActiveFrame.Labels.Pop()
	vm.ActiveFrame.PC++
}

func returnOp(vm *VirtualMachine) {
	vm.Frames.Pop()
	vm.ActiveFrame = vm.Frames.Peek()
}

func br(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	index := vm.FetchUint32()
	brAt(vm, index)
}

func brIf(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	index := vm.FetchUint32()
	c := vm.Operands.Pop()
	if c != 0 {
		brAt(vm, index)
	} else {
		vm.ActiveFrame.PC++
	}
}

func brAt(vm *VirtualMachine, index uint32) {
	var l *Label
	for i := uint32(0); i < index+1; i++ {
		l = vm.ActiveFrame.Labels.Pop()
	}

	// TODO: can be optimized.
	values := make([]uint64, 0, l.Arity)
	for i := 0; i < l.Arity; i++ {
		values = append(values, vm.Operands.Pop())
	}
	vm.Operands.SP = l.OperandSP
	for _, v := range values {
		vm.Operands.Push(v)
	}
	vm.ActiveFrame.PC = l.ContinuationPC
}

func brTable(vm *VirtualMachine) {
	vm.ActiveFrame.PC++
	r := bytes.NewBuffer(vm.ActiveFrame.F.Body[vm.ActiveFrame.PC:])
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
	vm.ActiveFrame.PC += n + num

	i := vm.Operands.Pop()
	if uint32(i) < nl {
		brAt(vm, lis[i])
	} else {
		brAt(vm, ln)
	}
}
