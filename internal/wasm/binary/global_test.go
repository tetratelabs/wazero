package binary

import (
	"testing"

	"github.com/heeus/hwazero/internal/leb128"
	"github.com/heeus/hwazero/internal/testing/require"
	"github.com/heeus/hwazero/internal/wasm"
)

func TestEncodeGlobal(t *testing.T) {
	tests := []struct {
		name     string
		input    *wasm.Global
		expected []byte
	}{
		{
			name: "const",
			input: &wasm.Global{
				Type: &wasm.GlobalType{ValType: wasm.ValueTypeI32},
				Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(1)},
			},
			expected: []byte{
				wasm.ValueTypeI32, 0x00, // 0 == const
				wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
			},
		},
		{
			name: "var",
			input: &wasm.Global{
				Type: &wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
				Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI32Const, Data: leb128.EncodeInt32(1)},
			},
			expected: []byte{
				wasm.ValueTypeI32, 0x01, // 1 == var
				wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeGlobal(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
