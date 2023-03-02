package binaryencoding

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var addLocalZeroLocalTwo = []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 2, wasm.OpcodeI32Add, wasm.OpcodeEnd}

func TestEncodeCode(t *testing.T) {
	addLocalZeroLocalOne := []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}
	tests := []struct {
		name     string
		input    *wasm.Code
		expected []byte
	}{
		{
			name: "smallest function body",
			input: &wasm.Code{ // e.g. (func)
				Body: []byte{wasm.OpcodeEnd},
			},
			expected: []byte{
				0x02,           // 2 bytes to encode locals and the body
				0x00,           // no local blocks
				wasm.OpcodeEnd, // Body
			},
		},
		{
			name: "params and instructions", // local.get index is params, then locals
			input: &wasm.Code{ // e.g. (func (type 3) local.get 0 local.get 1 i32.add)
				Body: addLocalZeroLocalOne,
			},
			expected: append([]byte{
				0x07, // 7 bytes to encode locals and the body
				0x00, // no local blocks
			},
				addLocalZeroLocalOne..., // Body
			),
		},
		{
			name: "locals and instructions",
			input: &wasm.Code{ // e.g. (func (result i32) (local i32, i32) local.get 0 local.get 1 i32.add)
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
				Body:       addLocalZeroLocalOne,
			},
			expected: append([]byte{
				0x09,                    // 9 bytes to encode locals and the body
				0x01,                    // 1 local block
				0x02, wasm.ValueTypeI32, // local block 1
			},
				addLocalZeroLocalOne..., // Body
			),
		},
		{
			name: "mixed locals and instructions",
			input: &wasm.Code{ // e.g. (func (result i32) (local i32) (local i64) (local i32) local.get 0 local.get 2 i32.add)
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeI32},
				Body:       addLocalZeroLocalTwo,
			},
			expected: append([]byte{
				0x0d,                    // 13 bytes to encode locals and the body
				0x03,                    // 3 local blocks
				0x01, wasm.ValueTypeI32, // local block 1
				0x01, wasm.ValueTypeI64, // local block 2
				0x01, wasm.ValueTypeI32, // local block 3
			},
				addLocalZeroLocalTwo..., // Body
			),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeCode(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}

func BenchmarkEncodeCode(b *testing.B) {
	input := &wasm.Code{ // e.g. (func (result i32) (local i32) (local i64) (local i32) local.get 0 local.get 2 i32.add)
		LocalTypes: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeI32},
		Body:       addLocalZeroLocalTwo,
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if bytes := encodeCode(input); len(bytes) == 0 {
			b.Fatal("didn't encode anything")
		}
	}
}
