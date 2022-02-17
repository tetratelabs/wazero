package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasm"
)

var addLocalZeroLocalTwo = []byte{internalwasm.OpcodeLocalGet, 0, internalwasm.OpcodeLocalGet, 2, internalwasm.OpcodeI32Add, internalwasm.OpcodeEnd}

func TestEncodeCode(t *testing.T) {
	addLocalZeroLocalOne := []byte{internalwasm.OpcodeLocalGet, 0, internalwasm.OpcodeLocalGet, 1, internalwasm.OpcodeI32Add, internalwasm.OpcodeEnd}
	tests := []struct {
		name     string
		input    *internalwasm.Code
		expected []byte
	}{
		{
			name: "smallest function body",
			input: &internalwasm.Code{ // Ex. (func)
				Body: []byte{internalwasm.OpcodeEnd},
			},
			expected: []byte{
				0x02,                   // 2 bytes to encode locals and the body
				0x00,                   // no local blocks
				internalwasm.OpcodeEnd, // Body
			},
		},
		{
			name: "params and instructions", // local.get index space is params, then locals
			input: &internalwasm.Code{ // Ex. (func (type 3) local.get 0 local.get 1 i32.add)
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
			input: &internalwasm.Code{ // Ex. (func (result i32) (local i32, i32) local.get 0 local.get 1 i32.add)
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
			input: &internalwasm.Code{ // Ex. (func (result i32) (local i32) (local i64) (local i32) local.get 0 local.get 2 i32.add)
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
	input := &internalwasm.Code{ // Ex. (func (result i32) (local i32) (local i64) (local i32) local.get 0 local.get 2 i32.add)
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
