package binary

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
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
			exp: wasm.NewConstantExpressionFromOpcode(wasm.OpcodeRefFunc, []byte{0x80, 0}),
		},
		{
			in: []byte{
				wasm.OpcodeRefFunc,
				0x80, 0x80, 0x80, 0x4f, // 165675008 in varint encoding.
				wasm.OpcodeEnd,
			},
			exp: wasm.NewConstantExpressionFromOpcode(wasm.OpcodeRefFunc, []byte{0x80, 0x80, 0x80, 0x4f}),
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				wasm.RefTypeFuncref.Kind(),
				wasm.OpcodeEnd,
			},
			exp: wasm.NewConstantExpressionFromOpcode(wasm.OpcodeRefNull, []byte{wasm.RefTypeFuncref.Kind()}),
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				wasm.RefTypeExternref.Kind(),
				wasm.OpcodeEnd,
			},
			exp: wasm.NewConstantExpressionFromOpcode(wasm.OpcodeRefNull, []byte{wasm.RefTypeExternref.Kind()}),
		},
		{
			in: []byte{
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeEnd,
			},
			exp: wasm.NewConstantExpressionFromOpcode(wasm.OpcodeVecV128Const, []byte{
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
			}),
		},
		{
			in: []byte{
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			},
			exp: wasm.ConstantExpression{
				Data: []byte{
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeI32Add,
					wasm.OpcodeEnd,
				},
			},
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var actual wasm.ConstantExpression
			err := decodeConstantExpression(bytes.NewReader(tc.in),
				api.CoreFeatureBulkMemoryOperations|api.CoreFeatureSIMD|experimental.CoreFeaturesExtendedConst, &actual)
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
			expectedErr: "read const expression opcode: EOF",
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
			expectedErr: "read const expression opcode: EOF",
			features:    api.CoreFeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				0x80, // LEB128 continuation bit set with no following byte
			},
			expectedErr: "invalid type for ref.null: 0x80",
			features:    api.CoreFeatureBulkMemoryOperations,
		},
		{
			in: []byte{
				wasm.OpcodeRefNull,
				wasm.RefTypeExternref.Kind(),
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
		{
			in: []byte{
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeI32Add,
				wasm.OpcodeEnd,
			},
			expectedErr: "i32.add is not supported in a constant expression as feature \"extended-const\" is disabled",
			features:    api.CoreFeaturesV2,
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
