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
		err := validateFunction(&FunctionType{}, body, nil, nil, nil, nil, nil, nil, max+1, Features(0))
		require.NoError(t, err)
	})
	t.Run("exceed", func(t *testing.T) {
		err := validateFunction(&FunctionType{}, body, nil, nil, nil, nil, nil, nil, max, Features(0))
		require.Error(t, err)
		expMsg := fmt.Sprintf("function may have %d stack values, which exceeds limit %d", valuesNum, max)
		require.Equal(t, expMsg, err.Error())
	})
}

func TestValidateFunction_SignExtensionOps(t *testing.T) {
	// TODO: actually support, guarded by FeatureSignExtensionOps flag which defaults to false #66
	tests := []struct {
		input       Opcode
		expectedErr string
	}{
		{
			input:       OpcodeI32Extend8S,
			expectedErr: "i32.extend8_s invalid as sign-extension-ops is not yet supported. See #66",
		},
		{
			input:       OpcodeI32Extend16S,
			expectedErr: "i32.extend16_s invalid as sign-extension-ops is not yet supported. See #66",
		},
		{
			input:       OpcodeI64Extend8S,
			expectedErr: "i64.extend8_s invalid as sign-extension-ops is not yet supported. See #66",
		},
		{
			input:       OpcodeI64Extend16S,
			expectedErr: "i64.extend16_s invalid as sign-extension-ops is not yet supported. See #66",
		},
		{
			input:       OpcodeI64Extend32S,
			expectedErr: "i64.extend32_s invalid as sign-extension-ops is not yet supported. See #66",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(InstructionName(tc.input), func(t *testing.T) {
			err := validateFunction(&FunctionType{}, []byte{tc.input}, nil, nil, nil, nil, nil, nil, 0, Features(0))
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
