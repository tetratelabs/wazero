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
		err := validateFunction(Features20191205, &FunctionType{}, body, nil, nil, nil, nil, nil, nil, max+1)
		require.NoError(t, err)
	})
	t.Run("exceed", func(t *testing.T) {
		err := validateFunction(Features20191205, &FunctionType{}, body, nil, nil, nil, nil, nil, nil, max)
		require.Error(t, err)
		expMsg := fmt.Sprintf("function may have %d stack values, which exceeds limit %d", valuesNum, max)
		require.Equal(t, expMsg, err.Error())
	})
}

func TestValidateFunction_SignExtensionOps(t *testing.T) {
	const maxStackHeight = 100 // arbitrary
	tests := []struct {
		input                Opcode
		expectedErrOnDisable string
	}{
		{
			input:                OpcodeI32Extend8S,
			expectedErrOnDisable: "i32.extend8_s invalid as feature sign-extension-ops is disabled",
		},
		{
			input:                OpcodeI32Extend16S,
			expectedErrOnDisable: "i32.extend16_s invalid as feature sign-extension-ops is disabled",
		},
		{
			input:                OpcodeI64Extend8S,
			expectedErrOnDisable: "i64.extend8_s invalid as feature sign-extension-ops is disabled",
		},
		{
			input:                OpcodeI64Extend16S,
			expectedErrOnDisable: "i64.extend16_s invalid as feature sign-extension-ops is disabled",
		},
		{
			input:                OpcodeI64Extend32S,
			expectedErrOnDisable: "i64.extend32_s invalid as feature sign-extension-ops is disabled",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(InstructionName(tc.input), func(t *testing.T) {
			t.Run("disabled", func(t *testing.T) {
				err := validateFunction(Features20191205, &FunctionType{}, []byte{tc.input}, nil, nil, nil, nil, nil, nil, maxStackHeight)
				require.EqualError(t, err, tc.expectedErrOnDisable)
			})
			t.Run("enabled", func(t *testing.T) {
				is32bit := tc.input == OpcodeI32Extend8S || tc.input == OpcodeI32Extend16S
				var body []byte
				if is32bit {
					body = append(body, OpcodeI32Const)
				} else {
					body = append(body, OpcodeI64Const)
				}
				body = append(body, tc.input, 123, OpcodeDrop, OpcodeEnd)
				err := validateFunction(FeatureSignExtensionOps, &FunctionType{}, body, nil, nil, nil, nil, nil, nil, maxStackHeight)
				require.NoError(t, err)
			})
		})
	}
}
