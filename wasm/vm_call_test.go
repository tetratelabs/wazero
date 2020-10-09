package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type dummyFunc struct {
	cnt int
}

func (d *dummyFunc) Call(_ *VirtualMachine)      { d.cnt++ }
func (d *dummyFunc) FunctionType() *FunctionType { return &FunctionType{} }

func Test_call(t *testing.T) {
	df := &dummyFunc{}
	vm := &VirtualMachine{
		ActiveContext: &NativeFunctionContext{
			Function: &NativeFunction{
				Body: []byte{byte(OptCodeCall), 0x01},
			},
		},
		Functions: []VirtualMachineFunction{nil, df},
	}

	call(vm)
	assert.Equal(t, 1, df.cnt)
}

func Test_callIndirect(t *testing.T) {
	df := &dummyFunc{}
	vm := &VirtualMachine{
		ActiveContext: &NativeFunctionContext{
			Function: &NativeFunction{
				Body: []byte{byte(OptCodeCall), 0x01, 0x00},
			},
		},
		Functions: []VirtualMachineFunction{nil, df},
		InnerModule: &Module{
			SecTypes: []*FunctionType{nil, {}},
			IndexSpace: &ModuleIndexSpace{
				Table: [][]uint32{{0, 1}},
			},
		},
		OperandStack: NewVirtualMachineOperandStack(),
	}
	vm.OperandStack.Push(1)

	callIndirect(vm)
	assert.Equal(t, 1, df.cnt)
}
