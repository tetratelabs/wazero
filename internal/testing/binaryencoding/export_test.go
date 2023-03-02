package binaryencoding

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestEncodeExport(t *testing.T) {
	tests := []struct {
		name     string
		input    *wasm.Export
		expected []byte
	}{
		{
			name: "func no name, index 0",
			input: &wasm.Export{ // e.g. (export "" (func 0)))
				Type:  wasm.ExternTypeFunc,
				Name:  "",
				Index: 0,
			},
			expected: []byte{wasm.ExternTypeFunc, 0x00, 0x00},
		},
		{
			name: "func name, func index 0",
			input: &wasm.Export{ // e.g. (export "pi" (func 0))
				Type:  wasm.ExternTypeFunc,
				Name:  "pi",
				Index: 0,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExternTypeFunc,
				0x00,
			},
		},
		{
			name: "func name, index 10",
			input: &wasm.Export{ // e.g. (export "pi" (func 10))
				Type:  wasm.ExternTypeFunc,
				Name:  "pi",
				Index: 10,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExternTypeFunc,
				0x0a,
			},
		},
		{
			name: "global no name, index 0",
			input: &wasm.Export{ // e.g. (export "" (global 0)))
				Type:  wasm.ExternTypeGlobal,
				Name:  "",
				Index: 0,
			},
			expected: []byte{0x00, wasm.ExternTypeGlobal, 0x00},
		},
		{
			name: "global name, global index 0",
			input: &wasm.Export{ // e.g. (export "pi" (global 0))
				Type:  wasm.ExternTypeGlobal,
				Name:  "pi",
				Index: 0,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExternTypeGlobal,
				0x00,
			},
		},
		{
			name: "global name, index 10",
			input: &wasm.Export{ // e.g. (export "pi" (global 10))
				Type:  wasm.ExternTypeGlobal,
				Name:  "pi",
				Index: 10,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExternTypeGlobal,
				0x0a,
			},
		},
		{
			name: "memory no name, index 0",
			input: &wasm.Export{ // e.g. (export "" (memory 0)))
				Type:  wasm.ExternTypeMemory,
				Name:  "",
				Index: 0,
			},
			expected: []byte{0x00, wasm.ExternTypeMemory, 0x00},
		},
		{
			name: "memory name, memory index 0",
			input: &wasm.Export{ // e.g. (export "mem" (memory 0))
				Type:  wasm.ExternTypeMemory,
				Name:  "mem",
				Index: 0,
			},
			expected: []byte{
				0x03, 'm', 'e', 'm',
				wasm.ExternTypeMemory,
				0x00,
			},
		},
		{
			name: "memory name, index 10",
			input: &wasm.Export{ // e.g. (export "mem" (memory 10))
				Type:  wasm.ExternTypeMemory,
				Name:  "mem",
				Index: 10,
			},
			expected: []byte{
				0x03, 'm', 'e', 'm',
				wasm.ExternTypeMemory,
				0x0a,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bytes := encodeExport(tc.input)
			require.Equal(t, tc.expected, bytes)
		})
	}
}
