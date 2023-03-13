package binary

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestDecodeConstantExpression(t *testing.T) {
	tests := []struct {
		in  []byte
		exp wasm.ConstantExpression
	}{
		{
			in: []byte{
				wasm.OpcodeRefFunc,
				0x80, 0, // Multi byte zero.
				wasm.OpcodeEnd,
			},
			exp: wasm.ConstantExpression{
				Opcode: wasm.OpcodeRefFunc,
				Data:   []byte{0x80, 0},
			},
		},
		{
			in: []byte{
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
			},
			exp: wasm.ConstantExpression{
				Opcode: wasm.OpcodeRefFunc,
				Data:   []byte{0x80, 0x80, 0x80, 0x4f},
			},
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				wasm.RefTypeFuncref,
				wasm.OpcodeEnd,
			},
			exp: wasm.ConstantExpression{
				Opcode: wasm.OpcodeRefNull,
				Data: []byte{
					wasm.RefTypeFuncref,
				},
			},
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				wasm.RefTypeExternref,
				wasm.OpcodeEnd,
			},
			exp: wasm.ConstantExpression{
				Opcode: wasm.OpcodeRefNull,
				Data: []byte{
					wasm.RefTypeExternref,
				},
			},
		},
		{
			in: []byte{
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeEnd,
			},
			exp: wasm.ConstantExpression{
				Opcode: wasm.OpcodeVecV128Const,
				Data: []byte{
					1, 1, 1, 1, 1, 1, 1, 1,
					1, 1, 1, 1, 1, 1, 1, 1,
				},
			},
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var actual wasm.ConstantExpression
			err := decodeConstantExpression(bytes.NewReader(tc.in),
				api.CoreFeatureBulkMemoryOperations|api.CoreFeatureSIMD, &actual)
			require.NoError(t, err)
			require.Equal(t, tc.exp, actual)
		})
	}
}

func TestDecodeConstantExpression_errors(t *testing.T) {
	tests := []struct {
		in          []byte
		expectedErr string
		features    api.CoreFeatures
	}{
		{
			in: []byte{
				wasm.OpcodeRefFunc,
				0,
			},
			expectedErr: "look for end opcode: EOF",
			features:    api.CoreFeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
			},
			expectedErr: "read reference type for ref.null: EOF",
			features:    api.CoreFeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				0xff,
				wasm.OpcodeEnd,
			},
			expectedErr: "invalid type for ref.null: 0xff",
			features:    api.CoreFeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				wasm.RefTypeExternref,
				wasm.OpcodeEnd,
			},
			expectedErr: "ref.null is not supported as feature \"bulk-memory-operations\" is disabled",
			features:    api.CoreFeaturesV1,
		},
		{
			in: []byte{
				wasm.OpcodeRefFunc,
				0x80, 0,
				wasm.OpcodeEnd,
			},
			expectedErr: "ref.func is not supported as feature \"bulk-memory-operations\" is disabled",
			features:    api.CoreFeaturesV1,
		},
		{
			in: []byte{
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeEnd,
			},
			expectedErr: "vector instructions are not supported as feature \"simd\" is disabled",
			features:    api.CoreFeaturesV1,
		},
		{
			in: []byte{
				wasm.OpcodeVecPrefix,
			},
			expectedErr: "read vector instruction opcode suffix: EOF",
			features:    api.CoreFeatureSIMD,
		},
		{
			in: []byte{
				wasm.OpcodeVecPrefix,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeEnd,
			},
			expectedErr: "invalid vector opcode for const expression: 0x1",
			features:    api.CoreFeatureSIMD,
		},
		{
			in: []byte{
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
			},
			expectedErr: "read vector const instruction immediates: needs 16 bytes but was 8 bytes",
			features:    api.CoreFeatureSIMD,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.expectedErr, func(t *testing.T) {
			var actual wasm.ConstantExpression
			err := decodeConstantExpression(bytes.NewReader(tc.in), tc.features, &actual)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}
