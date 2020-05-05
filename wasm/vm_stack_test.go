package wasm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVirtualMachineOperandStack(t *testing.T) {
	s := NewVirtualMachineOperandStack()
	assert.Equal(t, initialStackHeight, len(s.Stack))

	var exp uint64 = 10
	s.Push(exp)
	assert.Equal(t, exp, s.Pop())

	// verify the length grows
	for i := 0; i < initialStackHeight+1; i++ {
		s.Push(uint64(i))
	}
	assert.True(t, len(s.Stack) > initialStackHeight)

	// verify the length is not shortened
	for i := 0; i < len(s.Stack); i++ {
		_ = s.Pop()
	}

	assert.True(t, len(s.Stack) > initialStackHeight)
}

func TestVirtualMachineLabelStack(t *testing.T) {
	s := NewVirtualMachineLabelStack()
	assert.Equal(t, initialStackHeight, len(s.Stack))

	exp := &Label{Arity: 100}
	s.Push(exp)
	assert.Equal(t, exp, s.Pop())

	// verify the length grows
	for i := 0; i < initialStackHeight+1; i++ {
		s.Push(&Label{})
	}
	assert.True(t, len(s.Stack) > initialStackHeight)

	// verify the length is not shortened
	for i := 0; i < len(s.Stack); i++ {
		_ = s.Pop()
	}

	assert.True(t, len(s.Stack) > initialStackHeight)
}
