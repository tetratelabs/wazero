package wasm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNumOps(t *testing.T) {

	var testTable = []struct {
		input [2]int                // numbers to push on the stack
		op    func(*VirtualMachine) // operation to apply
		desc  string                // string description of the operation
		want  uint32                // result should obtain
	}{
		{input: [2]int{0, 0}, op: i32eqz, desc: "i32eqz", want: 1},
		{input: [2]int{3, 3}, op: i32eq, desc: "i32eq", want: 1},
		{input: [2]int{3, 4}, op: i32ne, desc: "i32ne", want: 1},
		{input: [2]int{3, 3}, op: i32ne, desc: "i32ne", want: 0},
		{input: [2]int{-4, 1}, op: i32lts, desc: "i32lts", want: 1},
		{input: [2]int{4, -1}, op: i32lts, desc: "i32lts", want: 0},
		{input: [2]int{1, 4}, op: i32ltu, desc: "i32ltu", want: 1},
		{input: [2]int{4, 1}, op: i32ltu, desc: "i32ltu", want: 0},
		{input: [2]int{1, -4}, op: i32gts, desc: "i32gts", want: 1},
		{input: [2]int{-4, 1}, op: i32gts, desc: "i32gts", want: 0},
	}

	var vm VirtualMachine
	for id, tt := range testTable {
		vm = VirtualMachine{
			OperandStack: NewVirtualMachineOperandStack(),
		}

		vm.OperandStack.Push(uint64(tt.input[0]))
		vm.OperandStack.Push(uint64(tt.input[1]))
		tt.op(&vm)

		assert.Equal(t, tt.want, uint32(vm.OperandStack.Pop()),
			fmt.Sprintf("test #%d : %d %s %d should eq %d", id, tt.input[0], tt.desc, tt.input[1], tt.want))
	}

}
