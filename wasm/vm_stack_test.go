package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVirtualMachineOperandStack(t *testing.T) {
	s := NewVirtualMachineOperandStack()
	assert.Equal(t, initialOperandStackHeight, len(s.Stack))

	var exp uint64 = 10
	s.Push(exp)
	assert.Equal(t, exp, s.Pop())

	// verify the length grows
	for i := 0; i < initialOperandStackHeight+1; i++ {
		s.Push(uint64(i))
	}
	assert.True(t, len(s.Stack) > initialOperandStackHeight)

	// verify the length is not shortened
	for i := 0; i < len(s.Stack); i++ {
		_ = s.Pop()
	}

	assert.True(t, len(s.Stack) > initialOperandStackHeight)
}

func TestVirtualMachineLabelStack(t *testing.T) {
	s := NewVirtualMachineLabelStack()
	assert.Equal(t, initialLabelStackHeight, len(s.Stack))

	exp := &Label{Arity: 100}
	s.Push(exp)
	assert.Equal(t, exp, s.Pop())

	// verify the length grows
	for i := 0; i < initialLabelStackHeight+1; i++ {
		s.Push(&Label{})
	}
	assert.True(t, len(s.Stack) > initialLabelStackHeight)

	// verify the length is not shortened
	for i := 0; i < len(s.Stack); i++ {
		_ = s.Pop()
	}

	assert.True(t, len(s.Stack) > initialLabelStackHeight)
}
