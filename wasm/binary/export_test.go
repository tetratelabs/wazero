package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestEncodeExport(t *testing.T) {
	tests := []struct {
		name     string
		input    *wasm.Export
		expected []byte
	}{
		{
			name: "func no name, index 0",
			input: &wasm.Export{ // Ex. (export "" "" (func 0)))
				Kind:  wasm.ExportKindFunc,
				Name:  "",
				Index: 0,
			},
			expected: []byte{wasm.ExportKindFunc, 0x00, 0x00},
		},
		{
			name: "func name, func index 0",
			input: &wasm.Export{ // Ex. (export "pi" (func 0))
				Kind:  wasm.ExportKindFunc,
				Name:  "pi",
				Index: 0,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExportKindFunc,
				0x00,
			},
		},
		{
			name: "func name, index 10",
			input: &wasm.Export{ // Ex. (export "pi" (func 10))
				Kind:  wasm.ExportKindFunc,
				Name:  "pi",
				Index: 10,
			},
			expected: []byte{
				0x02, 'p', 'i',
				wasm.ExportKindFunc,
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
