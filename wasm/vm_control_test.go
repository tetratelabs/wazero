package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_block(t *testing.T) {
	ctx := &NativeFunctionContext{
		PC: 1,
		Function: &NativeFunction{
			Blocks: map[uint64]*NativeFunctionBlock{
				1: {
					StartAt:        1,
					EndAt:          100,
					BlockTypeBytes: 3,
					BlockType:      &FunctionType{ReturnTypes: []ValueType{ValueTypeI32}},
				},
			},
		},
		LabelStack: NewVirtualMachineLabelStack(),
	}
	block(&VirtualMachine{ActiveContext: ctx})
	assert.Equal(t, &Label{
		Arity:          1,
		ContinuationPC: 100,
		EndPC:          100,
	}, ctx.LabelStack.Stack[ctx.LabelStack.SP])
	assert.Equal(t, uint64(4), ctx.PC)
}

func Test_loop(t *testing.T) {
	ctx := &NativeFunctionContext{
		PC: 1,
		Function: &NativeFunction{
			Blocks: map[uint64]*NativeFunctionBlock{
				1: {
					StartAt:        1,
					EndAt:          100,
					BlockTypeBytes: 3,
					BlockType:      &FunctionType{ReturnTypes: []ValueType{ValueTypeI32}},
				},
			},
		},
		LabelStack: NewVirtualMachineLabelStack(),
	}
	loop(&VirtualMachine{ActiveContext: ctx})
	assert.Equal(t, &Label{
		Arity:          1,
		ContinuationPC: 0,
		EndPC:          100,
	}, ctx.LabelStack.Stack[ctx.LabelStack.SP])
	assert.Equal(t, uint64(4), ctx.PC)
}

func Test_ifOp(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		ctx := &NativeFunctionContext{
			PC: 1,
			Function: &NativeFunction{
				Blocks: map[uint64]*NativeFunctionBlock{
					1: {
						StartAt:        1,
						EndAt:          100,
						BlockTypeBytes: 3,
						BlockType:      &FunctionType{ReturnTypes: []ValueType{ValueTypeI32}},
					},
				},
			},
			LabelStack: NewVirtualMachineLabelStack(),
		}
		vm := &VirtualMachine{ActiveContext: ctx, OperandStack: NewVirtualMachineOperandStack()}
		vm.OperandStack.Push(1)
		ifOp(vm)
		assert.Equal(t, &Label{
			Arity:          1,
			ContinuationPC: 100,
			EndPC:          100,
		}, ctx.LabelStack.Stack[ctx.LabelStack.SP])
		assert.Equal(t, uint64(4), ctx.PC)
	})
	t.Run("false", func(t *testing.T) {
		ctx := &NativeFunctionContext{
			PC: 1,
			Function: &NativeFunction{
				Blocks: map[uint64]*NativeFunctionBlock{
					1: {
						StartAt:        1,
						ElseAt:         50,
						EndAt:          100,
						BlockTypeBytes: 3,
						BlockType:      &FunctionType{ReturnTypes: []ValueType{ValueTypeI32}},
					},
				},
			},
			LabelStack: NewVirtualMachineLabelStack(),
		}
		vm := &VirtualMachine{ActiveContext: ctx, OperandStack: NewVirtualMachineOperandStack()}
		vm.OperandStack.Push(0)
		ifOp(vm)
		assert.Equal(t, &Label{
			Arity:          1,
			ContinuationPC: 100,
			EndPC:          100,
		}, ctx.LabelStack.Stack[ctx.LabelStack.SP])
		assert.Equal(t, uint64(50), ctx.PC)
	})
}

func Test_elseOp(t *testing.T) {
	ctx := &NativeFunctionContext{LabelStack: NewVirtualMachineLabelStack()}
	ctx.LabelStack.Push(&Label{EndPC: 100000})
	elseOp(&VirtualMachine{ActiveContext: ctx})
	assert.Equal(t, uint64(100000), ctx.PC)
}

func Test_end(t *testing.T) {
	ctx := &NativeFunctionContext{LabelStack: NewVirtualMachineLabelStack()}
	ctx.LabelStack.Push(&Label{EndPC: 100000})
	end(&VirtualMachine{ActiveContext: ctx})
	assert.Equal(t, -1, ctx.LabelStack.SP)
}

func Test_br(t *testing.T) {
	ctx := &NativeFunctionContext{
		LabelStack: NewVirtualMachineLabelStack(),
		Function:   &NativeFunction{Body: []byte{0x00, 0x01}},
	}
	vm := &VirtualMachine{ActiveContext: ctx, OperandStack: NewVirtualMachineOperandStack()}
	ctx.LabelStack.Push(&Label{ContinuationPC: 5})
	ctx.LabelStack.Push(&Label{})
	br(vm)
	assert.Equal(t, uint64(5), ctx.PC)
}

func Test_brIf(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		ctx := &NativeFunctionContext{
			LabelStack: NewVirtualMachineLabelStack(),
			Function:   &NativeFunction{Body: []byte{0x00, 0x01}},
		}
		vm := &VirtualMachine{ActiveContext: ctx, OperandStack: NewVirtualMachineOperandStack()}
		vm.OperandStack.Push(1)
		ctx.LabelStack.Push(&Label{ContinuationPC: 5})
		ctx.LabelStack.Push(&Label{})
		brIf(vm)
		assert.Equal(t, uint64(5), ctx.PC)
	})

	t.Run("false", func(t *testing.T) {
		ctx := &NativeFunctionContext{
			LabelStack: NewVirtualMachineLabelStack(),
			Function:   &NativeFunction{Body: []byte{0x00, 0x01}},
		}
		vm := &VirtualMachine{ActiveContext: ctx, OperandStack: NewVirtualMachineOperandStack()}
		vm.OperandStack.Push(0)
		brIf(vm)
		assert.Equal(t, uint64(1), ctx.PC)
	})
}

func Test_brAt(t *testing.T) {

}
