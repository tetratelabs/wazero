package binaryencoding

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestEncodeImport(t *testing.T) {
	ptrOfUint32 := func(v uint32) *uint32 {
		return &v
	}

	tests := []struct {
		name     string
		input    *wasm.Import
		expected []byte
	}{
		{
			name: "func no module, no name, type index 0",
			input: &wasm.Import{ // e.g. (import "" "" (func (type 0)))
				Type:     wasm.ExternTypeFunc,
				Module:   "",
				Name:     "",
				DescFunc: 0,
			},
			expected: []byte{wasm.ExternTypeFunc, 0x00, 0x00, 0x00},
		},
		{
			name: "func module, no name, type index 0",
			input: &wasm.Import{ // e.g. (import "$test" "" (func (type 0)))
				Type:     wasm.ExternTypeFunc,
				Module:   "test",
				Name:     "",
				DescFunc: 0,
			},
			expected: []byte{
				0x04, 't', 'e', 's', 't',
				0x00,
				wasm.ExternTypeFunc,
				0x00,
			},
		},
		{
			name: "func module, name, type index 0",
			input: &wasm.Import{ // e.g. (import "$math" "$pi" (func (type 0)))
				Type:     wasm.ExternTypeFunc,
				Module:   "math",
				Name:     "pi",
				DescFunc: 0,
			},
			expected: []byte{
				0x04, 'm', 'a', 't', 'h',
				0x02, 'p', 'i',
				wasm.ExternTypeFunc,
				0x00,
			},
		},
		{
			name: "func module, name, type index 10",
			input: &wasm.Import{ // e.g. (import "$math" "$pi" (func (type 10)))
				Type:     wasm.ExternTypeFunc,
				Module:   "math",
				Name:     "pi",
				DescFunc: 10,
			},
			expected: []byte{
				0x04, 'm', 'a', 't', 'h',
				0x02, 'p', 'i',
				wasm.ExternTypeFunc,
				0x0a,
			},
		},
		{
			name: "global const",
			input: &wasm.Import{
				Type:       wasm.ExternTypeGlobal,
				Module:     "math",
				Name:       "pi",
				DescGlobal: wasm.GlobalType{ValType: wasm.ValueTypeF64},
			},
			expected: []byte{
				0x04, 'm', 'a', 't', 'h',
				0x02, 'p', 'i',
				wasm.ExternTypeGlobal,
				wasm.ValueTypeF64, 0x00, // 0 == const
			},
		},
		{
			name: "global var",
			input: &wasm.Import{
				Type:       wasm.ExternTypeGlobal,
				Module:     "math",
				Name:       "pi",
				DescGlobal: wasm.GlobalType{ValType: wasm.ValueTypeF64, Mutable: true},
			},
			expected: []byte{
				0x04, 'm', 'a', 't', 'h',
				0x02, 'p', 'i',
				wasm.ExternTypeGlobal,
				wasm.ValueTypeF64, 0x01, // 1 == var
			},
		},
		{
			name: "table",
			input: &wasm.Import{
				Type:      wasm.ExternTypeTable,
				Module:    "my",
				Name:      "table",
				DescTable: wasm.Table{Min: 1, Max: ptrOfUint32(2)},
			},
			expected: []byte{
				0x02, 'm', 'y',
				0x05, 't', 'a', 'b', 'l', 'e',
				wasm.ExternTypeTable,
				wasm.RefTypeFuncref,
				0x1, 0x1, 0x2, // Limit with max.
			},
		},
		{
			name: "memory",
			input: &wasm.Import{
				Type:    wasm.ExternTypeMemory,
				Module:  "my",
				Name:    "memory",
				DescMem: &wasm.Memory{Min: 1, Max: 2, IsMaxEncoded: true},
			},
			expected: []byte{
				0x02, 'm', 'y',
				0x06, 'm', 'e', 'm', 'o', 'r', 'y',
				wasm.ExternTypeMemory,
				0x1, 0x1, 0x2, // Limit with max.
			},
		},
		{
			name: "memory - defaultt max",
			input: &wasm.Import{
				Type:    wasm.ExternTypeMemory,
				Module:  "my",
				Name:    "memory",
				DescMem: &wasm.Memory{Min: 1, Max: wasm.MemoryLimitPages, IsMaxEncoded: false},
			},
			expected: []byte{
				0x02, 'm', 'y',
				0x06, 'm', 'e', 'm', 'o', 'r', 'y',
				wasm.ExternTypeMemory,
				0x0, 0x1, // Limit without max.
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := EncodeImport(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
