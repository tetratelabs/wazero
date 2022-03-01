package internalwasm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateFunction_valueStackLimit(t *testing.T) {
	const max = 100
	const valuesNum = max + 1

	// Build a function which has max+1 const instructions.
	var body []byte
	for i := 0; i < valuesNum; i++ {
		body = append(body, OpcodeI32Const, 1)
	}

	// Drop all the consts so that if the max is higher, this function body would be sound.
	for i := 0; i < valuesNum; i++ {
		body = append(body, OpcodeDrop)
	}

	// Plus all functions must end with End opcode.
	body = append(body, OpcodeEnd)

	t.Run("not exceed", func(t *testing.T) {
		err := validateFunction(&FunctionType{}, body, nil, nil, nil, nil, nil, nil, max+1)
		require.NoError(t, err)
	})
	t.Run("exceed", func(t *testing.T) {
		err := validateFunction(&FunctionType{}, body, nil, nil, nil, nil, nil, nil, max)
		require.Error(t, err)
		expMsg := fmt.Sprintf("function may have %d stack values, which exceeds limit %d", valuesNum, max)
		require.Equal(t, expMsg, err.Error())
	})
}
