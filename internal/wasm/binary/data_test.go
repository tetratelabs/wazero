package binary

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_decodeDataSegment(t *testing.T) {
	tests := []struct {
		in       []byte
		exp      *wasm.DataSegment
		features wasm.Features
		expErr   string
	}{
		{
			in: []byte{0xf,
				// Const expression.
				wasm.OpcodeI32Const, 0x1, wasm.OpcodeEnd,
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			features: wasm.FeatureBulkMemoryOperations,
			expErr:   "invalid data segment prefix: 0xf",
		},
		{
			in: []byte{0x0,
				// Const expression.
				wasm.OpcodeI32Const, 0x1, wasm.OpcodeEnd,
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			exp: &wasm.DataSegment{
				OffsetExpression: &wasm.ConstantExpression{
					Opcode: wasm.OpcodeI32Const,
					Data:   []byte{0x1},
				},
				Init: []byte{0xf, 0xf},
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{0x0,
				// Const expression.
				wasm.OpcodeI32Const, 0x1,
				0x2, 0xf, 0xf,
			},
			expErr:   "read offset expression: constant expression has been not terminated",
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{0x1, // Passive data segment without memory index and const expr.
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			exp: &wasm.DataSegment{
				OffsetExpression: nil,
				Init:             []byte{0xf, 0xf},
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{0x2,
				0x0, // Memory index.
				// Const expression.
				wasm.OpcodeI32Const, 0x1, wasm.OpcodeEnd,
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			exp: &wasm.DataSegment{
				OffsetExpression: &wasm.ConstantExpression{
					Opcode: wasm.OpcodeI32Const,
					Data:   []byte{0x1},
				},
				Init: []byte{0xf, 0xf},
			},
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{0x2,
				0x1, // Memory index.
				// Const expression.
				wasm.OpcodeI32Const, 0x1, wasm.OpcodeEnd,
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			expErr:   "memory index must be zero but was 1",
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{0x2,
				0x0, // Memory index.
				// Const expression.
				wasm.OpcodeI32Const, 0x1,
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			expErr:   "read offset expression: constant expression has been not terminated",
			features: wasm.FeatureBulkMemoryOperations,
		},
		{
			in: []byte{0x2,
				0x0, // Memory index.
				// Const expression.
				wasm.OpcodeI32Const, 0x1, wasm.OpcodeEnd,
				// Two initial data.
				0x2, 0xf, 0xf,
			},
			features: wasm.FeatureMutableGlobal,
			expErr:   "non-zero prefix for data segment is invalid as feature \"bulk-memory-operations\" is disabled",
		},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := decodeDataSegment(bytes.NewReader(tc.in), tc.features)
			if tc.expErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.exp, actual)
			} else {
				require.EqualError(t, err, tc.expErr)
			}
		})
	}
}
