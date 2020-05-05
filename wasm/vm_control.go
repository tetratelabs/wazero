package wasm

import (
	"bytes"

	"github.com/mathetake/gasm/wasm/leb128"
)

func block(vm *VirtualMachine) {
	ctx := vm.ActiveContext
	block, ok := ctx.Function.Blocks[ctx.PC]
	if !ok {
		panic("block not initialized")
	}

	ctx.PC += block.BlockTypeBytes
	ctx.LabelStack.Push(&Label{
		Arity:          len(block.BlockType.ReturnTypes),
		ContinuationPC: block.EndAt,
		EndPC:          block.EndAt,
	})
}

func loop(vm *VirtualMachine) {
	ctx := vm.ActiveContext
	block, ok := ctx.Function.Blocks[ctx.PC]
	if !ok {
		panic("block not found")
	}
	ctx.PC += block.BlockTypeBytes
	ctx.LabelStack.Push(&Label{
		Arity:          len(block.BlockType.ReturnTypes),
		ContinuationPC: block.StartAt - 1,
		EndPC:          block.EndAt,
	})
}

func ifOp(vm *VirtualMachine) {
	ctx := vm.ActiveContext
	block, ok := ctx.Function.Blocks[vm.ActiveContext.PC]
	if !ok {
		panic("block not initialized")
	}
	ctx.PC += block.BlockTypeBytes

	if vm.OperandStack.Pop() == 0 {
		// enter else
		vm.ActiveContext.PC = block.ElseAt
	}

	ctx.LabelStack.Push(&Label{
		Arity:          len(block.BlockType.ReturnTypes),
		ContinuationPC: block.EndAt,
		EndPC:          block.EndAt,
	})
}

func elseOp(vm *VirtualMachine) {
	l := vm.ActiveContext.LabelStack.Pop()
	vm.ActiveContext.PC = l.EndPC
}

func end(vm *VirtualMachine) {
	if vm.ActiveContext.LabelStack.SP > -1 {
		_ = vm.ActiveContext.LabelStack.Pop()
	}
}

func br(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	brAt(vm, index)
}

func brIf(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	index := vm.FetchUint32()
	c := vm.OperandStack.Pop()
	if c != 0 {
		brAt(vm, index)
	}
}

func brAt(vm *VirtualMachine, index uint32) {
	var l *Label
	for i := uint32(0); i < index+1; i++ {
		l = vm.ActiveContext.LabelStack.Pop()
	}
	vm.ActiveContext.PC = l.ContinuationPC
}

func brTable(vm *VirtualMachine) {
	vm.ActiveContext.PC++
	r := bytes.NewBuffer(vm.ActiveContext.Function.Body[vm.ActiveContext.PC:])
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
	vm.ActiveContext.PC += n + num

	i := vm.OperandStack.Pop()
	if uint32(i) < nl {
		brAt(vm, lis[i])
	} else {
		brAt(vm, ln)
	}
}
