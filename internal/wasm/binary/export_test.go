package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func TestEncodeExport(t *testing.T) {
	tests := []struct {
		name     string
		input    *wasm.Export
		expected []byte
	}{
		{
			name: "func no name, index 0",
			input: &wasm.Export{ // Ex. (export "" (func 0)))
				Kind:  wasm.ExternalKindFunc,
				Name:  "",
				Index: 0,
			},
			expected: []byte{wasm.ExternalKindFunc, 0x00, 0x00},
		},
		{
			name: "func name, func index 0",
			input: &wasm.Export{ // Ex. (export "pi" (func 0))
				Kind:  wasm.ExternalKindFunc,
				Name:  "pi",
				Index: 0,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExternalKindFunc,
				0x00,
			},
		},
		{
			name: "func name, index 10",
			input: &wasm.Export{ // Ex. (export "pi" (func 10))
				Kind:  wasm.ExternalKindFunc,
				Name:  "pi",
				Index: 10,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExternalKindFunc,
				0x0a,
			},
		},
		{
			name: "memory no name, index 0",
			input: &wasm.Export{ // Ex. (export "" (memory 0)))
				Kind:  wasm.ExternalKindMemory,
				Name:  "",
				Index: 0,
			},
			expected: []byte{0x00, wasm.ExternalKindMemory, 0x00},
		},
		{
			name: "memory name, memory index 0",
			input: &wasm.Export{ // Ex. (export "mem" (memory 0))
				Kind:  wasm.ExternalKindMemory,
				Name:  "mem",
				Index: 0,
			},
			expected: []byte{
				0x03, 'm', 'e', 'm',
				wasm.ExternalKindMemory,
				0x00,
			},
		},
		{
			name: "memory name, index 10",
			input: &wasm.Export{ // Ex. (export "mem" (memory 10))
				Kind:  wasm.ExternalKindMemory,
				Name:  "mem",
				Index: 10,
			},
			expected: []byte{
				0x03, 'm', 'e', 'm',
				wasm.ExternalKindMemory,
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
