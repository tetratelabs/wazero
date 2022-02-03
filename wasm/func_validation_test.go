package wasm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateFunction_valueStackLimit(t *testing.T) {
	const max = 100
	const valuesNum = max + 1

	// Build a function which has max+1 const instructions.
	f := &FunctionInstance{Body: []byte{}, FunctionType: &TypeInstance{Type: &FunctionType{}}}
	for i := 0; i < valuesNum; i++ {
		f.Body = append(f.Body, OpcodeI32Const, 1)
	}

	// Drop all the consts so that if the max is higher, this function body would be sound.
	for i := 0; i < valuesNum; i++ {
		f.Body = append(f.Body, OpcodeDrop)
	}

	// Plus all functions must end with End opcode.
	f.Body = append(f.Body, OpcodeEnd)

	t.Run("not exceed", func(t *testing.T) {
		err := validateFunctionInstance(f, nil, nil, nil, nil, nil, max+1)
		require.NoError(t, err)
	})
	t.Run("exceed", func(t *testing.T) {
		err := validateFunctionInstance(f, nil, nil, nil, nil, nil, max)
		require.Error(t, err)
		expMsg := fmt.Sprintf("function too large: potentially could have %d values on the stack with the limit %d", valuesNum, max)
		require.Equal(t, expMsg, err.Error())
	})
}
